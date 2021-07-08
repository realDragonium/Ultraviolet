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
	UNKNOWN ServerState = iota
	ONLINE
	OFFLINE
	UPDATE
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
		log.Println(err)
		return serverConn, err
	}

	return serverConn, nil
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
	State               ServerState
	StateUpdateCooldown time.Duration

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
	connCh := make(chan ConnRequest)
	statusCh := make(chan StatusRequest)
	stateUpdateCh := make(chan StateUpdate)

	connWorker := NewConnWorker(connCh, stateUpdateCh, cfg.Servers)
	go connWorker.Work()

	statusWorker := NewStatusWorker(statusCh, stateCh, stateUpdateCh, connCh, cfg.Servers)
	go statusWorker.Work()

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
			continue
		}
		stateAnswerCh := make(chan ServerState)
		w.stateCh <- StateRequest{
			serverId: request.ServerAddr,
			answerCh: stateAnswerCh,
		}
		state := <-stateAnswerCh

		if state == OFFLINE {
			// This need to be modified later when online status cache is being added
			if request.Type == STATUS {
				statusAnswerCh := make(chan mc.Packet)
				w.statusCh <- StatusRequest{
					serverId: request.ServerAddr,
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
			continue
		}
		connAnswerCh := make(chan func() (net.Conn, error))
		w.connCh <- ConnRequest{
			serverId: request.ServerAddr,
			answerCh: connAnswerCh,
		}
		connFunc := <-connAnswerCh
		netConn, err := connFunc()
		if err != nil {
			log.Printf("error while creating connection to server: %v", err)
			request.Ch <- McAnswer{
				Action: CLOSE,
			}
			continue
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

func NewConnWorker(reqCh chan ConnRequest, updateCh chan StateUpdate, proxies map[string]WorkerServerConfig) ConnWorker {
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
	answerCh chan func() (net.Conn, error)
	serverId string
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

func NewStatusWorker(reqCh chan StatusRequest, stateCh chan StateRequest, updateCh chan StateUpdate, connCh chan ConnRequest, proxies map[string]WorkerServerConfig) StatusWorker {
	servers := make(map[string]*ServerData)
	for id, proxy := range proxies {
		cooldown := proxy.StateUpdateCooldown
		if cooldown == 0 {
			cooldown = time.Second
		}
		servers[id] = &ServerData{
			State:               proxy.State,
			OfflineStatus:       proxy.OfflineStatus,
			OnlineStatus:        mc.Packet{},
			StateUpdateCooldown: cooldown,
		}
	}

	return StatusWorker{
		reqConnCh:     connCh,
		reqStateCh:    stateCh,
		reqStatusCh:   reqCh,
		updateStateCh: updateCh,
		serverData:    servers,
	}
}

type StateUpdate struct {
	serverId string
	state    ServerState
}

type StateRequest struct {
	serverId string
	answerCh chan ServerState
}

type StatusRequest struct {
	serverId string
	answerCh chan mc.Packet
}

type ServerData struct {
	State               ServerState
	StateUpdateCooldown time.Duration
	OnlineStatus        mc.Packet
	OfflineStatus       mc.Packet
}

// TODO:
// - automatically update state every x amount of time
// - check or state is still valid or expired before replying
// - add online status caching when doesnt proxy client requests to actual server
type StatusWorker struct {
	reqConnCh chan ConnRequest

	reqStatusCh   chan StatusRequest
	reqStateCh    chan StateRequest
	updateStateCh chan StateUpdate

	serverData map[string]*ServerData
}

func (w *StatusWorker) Work() {
	for {
		select {
		case request := <-w.reqStatusCh:
			data := w.serverData[request.serverId]
			if data.State == UNKNOWN {
				w.UpdateState(request.serverId)
				data = w.serverData[request.serverId]
			}
			var statusPk mc.Packet
			switch data.State {
			case ONLINE:
				statusPk = data.OnlineStatus
			case OFFLINE:
				statusPk = data.OfflineStatus
			}
			request.answerCh <- statusPk
		case request := <-w.reqStateCh:
			data := w.serverData[request.serverId]
			if data.State == UNKNOWN {
				w.UpdateState(request.serverId)
				data = w.serverData[request.serverId]
			}
			request.answerCh <- data.State
		case update := <-w.updateStateCh:
			data := w.serverData[update.serverId]
			if update.state == UPDATE {
				w.UpdateState(update.serverId)
				data = w.serverData[update.serverId]
			} else {
				data.State = update.state
			}
			w.serverData[update.serverId] = data
		}
	}
}

func (w *StatusWorker) UpdateState(serverId string) {
	data := w.serverData[serverId]
	connAnswerCh := make(chan func() (net.Conn, error))
	w.reqConnCh <- ConnRequest{
		serverId: serverId,
		answerCh: connAnswerCh,
	}
	connFunc := <-connAnswerCh
	_, err := connFunc()
	if err != nil {
		data.State = OFFLINE
	} else {
		data.State = ONLINE
	}
	w.serverData[serverId] = data
	go func(serverId string, sleepTime time.Duration, updateCh chan StateUpdate) {
		time.Sleep(sleepTime)
		updateCh <- StateUpdate{
			serverId: serverId,
			state:    UNKNOWN,
		}
	}(serverId, data.StateUpdateCooldown, w.updateStateCh)
}
