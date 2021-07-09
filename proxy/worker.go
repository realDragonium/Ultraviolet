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
	ErrOverConnRateLimit = errors.New("too many request within rate limit time frame")
)

type ServerState byte

const (
	UNKNOWN ServerState = iota
	ONLINE
	OFFLINE
	UPDATE
)

type connectionConfig struct {
	ProxyTo     string
	ProxyBind   string
	DialTimeout time.Duration
}

func newServerConn(cfg connectionConfig) (net.Conn, error) {
	dialer := net.Dialer{
		Timeout: cfg.DialTimeout,
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
	DialTimeout       time.Duration
	SendProxyProtocol bool

	RateLimit         int
	RateLimitDuration time.Duration
}

func RunBasicWorkers(amount int, cfg WorkerConfig, statusCh chan StatusRequest, connCh chan ConnRequest) {
	worker := NewBasicWorker(cfg, statusCh, connCh)
	for i := 0; i < amount; i++ {
		go func(worker BasicWorker) {
			worker.Work()
		}(worker)
	}
}

func NewBasicWorker(cfg WorkerConfig, statusCh chan StatusRequest, connCh chan ConnRequest) BasicWorker {
	return BasicWorker{
		reqCh:         cfg.ReqCh,
		defaultStatus: cfg.DefaultStatus,
		servers:       cfg.Servers,
		ProxyCh:       cfg.ProxyCh,

		statusCh: statusCh,
		connCh:   connCh,
	}
}

type BasicWorker struct {
	reqCh         chan McRequest
	defaultStatus mc.Packet
	servers       map[string]WorkerServerConfig
	ProxyCh       chan ProxyAction

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
		stateAnswerCh := make(chan StatusAnswer)
		w.statusCh <- StatusRequest{
			ServerId: request.ServerAddr,
			Type:     STATE_REQUEST,
			AnswerCh: stateAnswerCh,
		}
		answer := <-stateAnswerCh

		if answer.State == OFFLINE {
			// This need to be modified later when online status cache is being added
			if request.Type == STATUS {
				statusAnswerCh := make(chan StatusAnswer)
				w.statusCh <- StatusRequest{
					ServerId: request.ServerAddr,
					Type:     STATUS_REQUEST,
					AnswerCh: statusAnswerCh,
				}
				answer := <-statusAnswerCh
				request.Ch <- McAnswer{
					Action:   SEND_STATUS,
					StatusPk: answer.Pk,
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

func RunConnWorkers(amount int, reqCh chan ConnRequest, updateCh chan StatusRequest, proxies map[string]WorkerServerConfig) {
	worker := NewConnWorker(reqCh, updateCh, proxies)
	for i := 0; i < amount; i++ {
		go func(worker ConnWorker) {
			worker.Work()
		}(worker)
	}
}

func NewConnWorker(reqCh chan ConnRequest, updateCh chan StatusRequest, proxies map[string]WorkerServerConfig) ConnWorker {
	servers := make(map[string]*ConnectionData)
	for id, proxy := range proxies {
		dialTimeout := proxy.DialTimeout
		if dialTimeout == 0 {
			dialTimeout = time.Second
		}
		servers[id] = &ConnectionData{
			mu: sync.Mutex{},
			cfg: connectionConfig{
				ProxyTo:     proxy.ProxyTo,
				ProxyBind:   proxy.ProxyBind,
				DialTimeout: dialTimeout,
			},
			connLimit:         proxy.RateLimit,
			connLimitDuration: proxy.RateLimitDuration,
		}
	}

	return ConnWorker{
		reqCh:   reqCh,
		servers: &servers,
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
	servers *map[string]*ConnectionData
}

func (w ConnWorker) Work() {
	var connFunc func() (net.Conn, error)
	var data *ConnectionData
	var request ConnRequest
	for {
		request = <-w.reqCh
		data = (*w.servers)[request.serverId]
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

func RunStatusWorkers(amount int, reqCh chan StatusRequest, connCh chan ConnRequest, proxies map[string]WorkerServerConfig) {
	stateWorker := NewStatusWorker(reqCh, connCh, proxies)
	for i := 0; i < amount; i++ {
		go func(worker StatusWorker) {
			stateWorker.Work()
		}(stateWorker)
	}
}

func NewStatusWorker(reqStatusCh chan StatusRequest, connCh chan ConnRequest, proxies map[string]WorkerServerConfig) StatusWorker {
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
		reqConnCh:  connCh,
		reqCh:      reqStatusCh,
		serverData: servers,
	}
}

type StatusReqType byte

const (
	STATE_UPDATE StatusReqType = iota + 1
	STATE_REQUEST
	STATUS_REQUEST
)

type StatusRequest struct {
	ServerId string
	State    ServerState
	Type     StatusReqType
	AnswerCh chan StatusAnswer
}

type StatusAnswer struct {
	Pk    mc.Packet
	State ServerState
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
	reqConnCh  chan ConnRequest
	reqCh      chan StatusRequest
	serverData map[string]*ServerData
}

func (w *StatusWorker) Work() {
	for {
		request := <-w.reqCh
		data := w.serverData[request.ServerId]
		if request.Type == STATE_UPDATE {
			data.State = request.State
			w.serverData[request.ServerId] = data
			continue
		}
		if data.State == UNKNOWN {
			connAnswerCh := make(chan func() (net.Conn, error))
			w.reqConnCh <- ConnRequest{
				serverId: request.ServerId,
				answerCh: connAnswerCh,
			}
			connFunc := <-connAnswerCh
			_, err := connFunc()
			if err != nil {
				data.State = OFFLINE
			} else {
				data.State = ONLINE
			}
			w.serverData[request.ServerId] = data
			go func(serverId string, sleepTime time.Duration, updateCh chan StatusRequest) {
				time.Sleep(sleepTime)
				updateCh <- StatusRequest{
					ServerId: serverId,
					Type:     STATE_UPDATE,
					State:    UNKNOWN,
				}
			}(request.ServerId, data.StateUpdateCooldown, w.reqCh)
		}

		switch request.Type {
		case STATUS_REQUEST:
			var statusPk mc.Packet
			switch data.State {
			case ONLINE:
				statusPk = data.OnlineStatus
			case OFFLINE:
				statusPk = data.OfflineStatus
			}
			request.AnswerCh <- StatusAnswer{
				Pk: statusPk,
			}
		case STATE_REQUEST:
			request.AnswerCh <- StatusAnswer{
				State: data.State,
			}

		}
	}
}
