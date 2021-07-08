package proxy

import (
	"errors"
	"log"
	"net"
	"sync"
	"time"

	"github.com/pires/go-proxyproto"
	"github.com/realDragonium/Ultraviolet/mc"
)

var (
	ErrCantWriteToServer     = errors.New("can't write to proxy target")
	ErrCantWriteToClient     = errors.New("can't write to client")
	ErrCantConnectWithServer = errors.New("cant connect with server")

	ErrOverConnRateLimit = errors.New("too many request within rate limit time frame")

	dialTimeout = 1000 * time.Millisecond
)

type ServerState byte

const (
	ONLINE ServerState = iota
	OFFLINE
)

type connectionConfig struct {
	ProxyTo           string
	ProxyBind         string
	SendProxyProtocol bool
}

func newServerConn(cfg connectionConfig) (net.Conn, error) {
	dialer := net.Dialer{
		Timeout: dialTimeout,
		LocalAddr: &net.TCPAddr{
			// TODO: Add something for invalid ProxyBind ips
			IP: net.ParseIP(cfg.ProxyBind),
		},
	}
	serverConn, err := dialer.Dial("tcp", cfg.ProxyTo)
	if err != nil {
		return serverConn, err
	}

	return serverConn, nil
}

// What should the worker know?
// - All domains and their target
// - Default Status
// - some way to cache motd status from a server  (probably separate worker)
// - Some anti bot thing (probably separate worker)
// - Some way to create connections with backend server (probably separate worker)
// -
func SomethingElse() {

}

func NewWorkerConfig(reqCh chan McRequest, servers map[string]WorkerServerConfig, defaultStatus mc.Packet) WorkerConfig {
	return WorkerConfig{
		DefaultStatus: defaultStatus,
		ReqCh:         reqCh,
		ProxyCh:       make(chan ProxyAction),
		Servers:       servers,
	}
}

type WorkerConfig struct {
	DefaultStatus mc.Packet
	ReqCh         chan McRequest
	ProxyCh       chan ProxyAction
	Servers       map[string]WorkerServerConfig
}

type WorkerServerConfig struct {
	State ServerState

	OfflineStatus    mc.Packet
	DisconnectPacket mc.Packet

	ProxyTo           string
	ProxyBind         string
	SendProxyProtocol bool

	RateLimit         int
	RateLimitDuration time.Duration
}

func NewWorker(cfg WorkerConfig) BasicWorker {
	stateCh := make(chan StateRequest)
	stateWorker := NewStateWorker(stateCh, cfg.Servers)
	go stateWorker.Work()

	statusCh := make(chan StatusRequest)
	statusWorker := NewStatusWorker(statusCh, cfg.Servers)
	go statusWorker.Work()

	connCh := make(chan ConnRequest)
	connWorker := NewConnWorker(connCh, cfg.Servers)
	go connWorker.Work()

	return BasicWorker{
		reqCh:         cfg.ReqCh,
		defaultStatus: cfg.DefaultStatus,
		servers:       cfg.Servers,
		ProxyCh:       cfg.ProxyCh,

		stateCh:  stateCh,
		statusCh: statusCh,
		connCh:   connCh,
	}
}

type BasicWorker struct {
	reqCh         chan McRequest
	defaultStatus mc.Packet
	servers       map[string]WorkerServerConfig
	ProxyCh       chan ProxyAction

	stateCh  chan StateRequest
	statusCh chan StatusRequest
	connCh   chan ConnRequest
}

func (w BasicWorker) Work() {
	// See or this can be 'efficienter' when the for loop calls the work method
	//  instead of the work method containing the for loop (something with allocation)
	for {
		request := <-w.reqCh
		cfg, ok := w.servers[request.ServerAddr]
		if !ok {
			//Unknown server address
			if request.Type == STATUS {
				request.Ch <- McAnswer{
					Action:   SEND_STATUS,
					StatusPk: w.defaultStatus,
				}
			} else {
				request.Ch <- McAnswer{
					Action: CLOSE,
				}
			}
			return
		}
		stateAnswerCh := make(chan StateServerData)
		w.stateCh <- StateRequest{
			serverId: request.ServerAddr,
			answerCh: stateAnswerCh,
		}
		stateData := <-stateAnswerCh

		if stateData.state == OFFLINE {
			// This need to be modified later when online status cache is being added
			if request.Type == STATUS {
				statusAnswerCh := make(chan mc.Packet)
				w.statusCh <- StatusRequest{
					serverId: request.ServerAddr,
					state:    stateData.state,
					answerCh: statusAnswerCh,
				}
				statusPk := <-statusAnswerCh
				request.Ch <- McAnswer{
					Action:   SEND_STATUS,
					StatusPk: statusPk,
				}
			} else if request.Type == LOGIN {
				request.Ch <- McAnswer{
					Action:        DISCONNECT,
					DisconMessage: cfg.DisconnectPacket,
				}
			}
			return
		}
		connAnswerCh := make(chan func() (net.Conn, error))
		w.connCh <- ConnRequest{
			clientAddr: request.Addr,
			serverId:   request.ServerAddr,
			answerCh:   connAnswerCh,
		}
		connFunc := <-connAnswerCh
		netConn, err := connFunc()
		if err != nil {
			log.Printf("error while creating connection to server: %v", err)
			request.Ch <- McAnswer{
				Action: CLOSE,
			}
			return
		}

		if cfg.SendProxyProtocol {
			header := &proxyproto.Header{
				Version:           2,
				Command:           proxyproto.PROXY,
				TransportProtocol: proxyproto.TCPv4,
				SourceAddr:        request.Addr,
				DestinationAddr:   netConn.RemoteAddr(),
			}
			header.WriteTo(netConn)
		}

		request.Ch <- McAnswer{
			Action:     PROXY,
			ProxyCh:    w.ProxyCh,
			ServerConn: NewMcConn(netConn),
		}
	}
}

func NewConnWorker(reqCh chan ConnRequest, proxies map[string]WorkerServerConfig) ConnWorker {
	servers := make(map[string]*ConnectionData)
	for id, proxy := range proxies {
		servers[id] = &ConnectionData{
			mu: sync.Mutex{},
			cfg: connectionConfig{
				ProxyTo:   proxy.ProxyTo,
				ProxyBind: proxy.ProxyBind,
			},
			connLimit:         proxy.RateLimit,
			connLimitDuration: proxy.RateLimitDuration,
		}

	}
	return ConnWorker{
		reqCh:   reqCh,
		servers: servers,
	}
}

type ConnRequest struct {
	answerCh   chan func() (net.Conn, error)
	clientAddr net.Addr
	serverId   string
}

type ConnectionData struct {
	cfg connectionConfig
	mu  sync.Mutex

	connCounter       int
	connLimit         int
	connLimitDuration time.Duration
	timeStamp         time.Time
}

type ConnWorker struct {
	reqCh   chan ConnRequest
	servers map[string]*ConnectionData
}

func (w ConnWorker) Work() {
	var connFunc func() (net.Conn, error)
	var data *ConnectionData
	var request ConnRequest
	for {
		request = <-w.reqCh
		data = w.servers[request.serverId]
		data.mu.Lock()
		if data.connLimit == 0 {
			connFunc = func() (net.Conn, error) {
				return newServerConn(data.cfg)
			}
		} else {
			if time.Since(data.timeStamp) >= data.connLimitDuration {
				data.connCounter = 0
				data.timeStamp = time.Now()
			}
			if data.connCounter < data.connLimit {
				data.connCounter++
				connFunc = func() (net.Conn, error) {
					return newServerConn(data.cfg)
				}
			} else {
				connFunc = func() (net.Conn, error) {
					return nil, ErrOverConnRateLimit
				}
			}
		}

		data.mu.Unlock()
		request.answerCh <- connFunc
	}
}

func NewStatusWorker(reqCh chan StatusRequest, proxies map[string]WorkerServerConfig) StatusWorker {
	servers := make(map[string]StatusServerData)
	for id, proxy := range proxies {
		servers[id] = StatusServerData{
			OfflineStatus: proxy.OfflineStatus,
			OnlineStatus:  mc.Packet{},
		}

	}
	return StatusWorker{
		reqCh:   reqCh,
		servers: servers,
	}
}

type StatusRequest struct {
	serverId string
	state    ServerState
	answerCh chan mc.Packet
}

type StatusServerData struct {
	OnlineStatus  mc.Packet
	OfflineStatus mc.Packet
}

// TODO:
// - add online status caching when doesnt proxy client requests to actual server
type StatusWorker struct {
	reqCh   chan StatusRequest
	servers map[string]StatusServerData
}

func (w StatusWorker) Work() {
	for {
		request := <-w.reqCh
		cfg := w.servers[request.serverId]
		var statusPk mc.Packet
		switch request.state {
		case ONLINE:
			statusPk = cfg.OnlineStatus
		case OFFLINE:
			statusPk = cfg.OfflineStatus
		}
		request.answerCh <- statusPk
	}
}

func NewStateWorker(reqCh chan StateRequest, proxies map[string]WorkerServerConfig) StateWorker {
	servers := make(map[string]StateServerData)
	for id, proxy := range proxies {
		servers[id] = StateServerData{
			state: proxy.State,
		}

	}
	return StateWorker{
		reqCh:   reqCh,
		servers: servers,
	}
}

type StateRequest struct {
	serverId string
	answerCh chan StateServerData
}

type StateServerData struct {
	state ServerState
}

// TODO:
// - automatically update state every x amount of time
// - check or state is still valid or expired before replying
type StateWorker struct {
	reqCh   chan StateRequest
	servers map[string]StateServerData
}

func (w StateWorker) Work() {
	for {
		request := <-w.reqCh
		request.answerCh <- w.servers[request.serverId]
	}
}
