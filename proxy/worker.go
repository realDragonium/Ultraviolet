package proxy

import (
	"errors"
	"net"
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

func (state ServerState) String() string {
	var text string
	switch state {
	case UNKNOWN:
		text = "Unknown"
	case ONLINE:
		text = "Online"
	case OFFLINE:
		text = "Offline"
	case UPDATE:
		text = "Update"
	}
	return text
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

type ServerWorkerData struct {
	connReqCh chan McRequest
}

func NewPublicWorker(servers map[int]ServerWorkerData, serverDict map[string]int, status mc.Packet, reqCh chan McRequest) PublicWorker {
	return PublicWorker{
		reqCh:         reqCh,
		defaultStatus: status,
		serverDict:    serverDict,
		servers:       servers,
	}
}

type PublicWorker struct {
	reqCh         chan McRequest
	defaultStatus mc.Packet

	serverDict map[string]int
	servers    map[int]ServerWorkerData
}

func (worker *PublicWorker) Work() {
	for {
		request := <-worker.reqCh
		ch, ans, ok := worker.ProcessMCRequest(request)
		if !ok {
			request.Ch <- ans
			continue
		}
		ch <- request
	}
}

func (worker *PublicWorker) ProcessMCRequest(request McRequest) (chan McRequest, McAnswer, bool) {
	serverId, ok := worker.serverDict[request.ServerAddr]
	if !ok {
		//Unknown server address
		if request.Type == STATUS {
			return nil, McAnswer{
				Action:   SEND_STATUS,
				StatusPk: worker.defaultStatus,
			}, false
		} else {
			return nil, McAnswer{
				Action: CLOSE,
			}, false
		}
	}
	return worker.servers[serverId].connReqCh, McAnswer{}, true
}

func NewPrivateWorker(serverId int, cfg WorkerServerConfig) PrivateWorker {
	dialer := net.Dialer{
		Timeout: cfg.DialTimeout,
		LocalAddr: &net.TCPAddr{
			IP: net.ParseIP(cfg.ProxyBind),
		},
	}
	proxyTo := cfg.ProxyTo
	createConnFeature := func(addr net.Addr) func() (net.Conn, error) {
		return func() (net.Conn, error) {
			serverConn, err := dialer.Dial("tcp", proxyTo)
			if err != nil {
				return serverConn, err
			}

			if cfg.SendProxyProtocol {
				header := &proxyproto.Header{
					Version:           2,
					Command:           proxyproto.PROXY,
					TransportProtocol: proxyproto.TCPv4,
					SourceAddr:        addr,
					DestinationAddr:   serverConn.RemoteAddr(),
				}
				header.WriteTo(serverConn)
			}
			return serverConn, nil
		}
	}

	return PrivateWorker{
		serverId:      serverId,
		proxyCh:       make(chan ProxyAction),
		stateUpdateCh: make(chan ServerState),

		rateLimit:    cfg.RateLimit,
		rateCooldown: cfg.RateLimitDuration,

		state:         UNKNOWN,
		stateCooldown: cfg.StateUpdateCooldown,

		offlineStatus:    cfg.OfflineStatus,
		disconnectPacket: cfg.DisconnectPacket,

		serverConnFactory: createConnFeature,
	}
}

type PrivateWorker struct {
	serverId          int
	activeConnections int
	proxyCh           chan ProxyAction
	reqCh             chan McRequest

	rateCounter   int
	rateStartTime time.Time
	rateLimit     int
	rateCooldown  time.Duration

	state         ServerState
	stateCooldown time.Duration
	stateUpdateCh chan ServerState

	serverConnFactory func(net.Addr) func() (net.Conn, error)

	offlineStatus    mc.Packet
	disconnectPacket mc.Packet
}

func (worker *PrivateWorker) Work() {
	for {
		select {
		case state := <-worker.stateUpdateCh:
			worker.state = state
		case request := <-worker.reqCh:
			answer := worker.HandleRequest(request)
			request.Ch <- answer
		case proxyAction := <-worker.proxyCh:
			worker.proxyRequest(proxyAction)
		}
	}
}

func (worker *PrivateWorker) proxyRequest(proxyAction ProxyAction) {
	switch proxyAction {
	case PROXY_OPEN:
		worker.activeConnections++
	case PROXY_CLOSE:
		worker.activeConnections--
	}
}

func (worker *PrivateWorker) HandleRequest(request McRequest) McAnswer {
	if worker.state == UNKNOWN {
		connFunc := worker.serverConnFactory(&net.IPAddr{})
		_, err := connFunc()
		if err != nil {
			worker.state = OFFLINE
		} else {
			worker.state = ONLINE
		}
		go func(serverId int, sleepTime time.Duration, updateCh chan ServerState) {
			time.Sleep(sleepTime)
			updateCh <- UNKNOWN
		}(worker.serverId, worker.stateCooldown, worker.stateUpdateCh)
	}
	if worker.state == OFFLINE {
		if request.Type == STATUS {
			return McAnswer{
				Action:   SEND_STATUS,
				StatusPk: worker.offlineStatus,
			}
		} else if request.Type == LOGIN {
			return McAnswer{
				Action:        DISCONNECT,
				DisconMessage: worker.disconnectPacket,
			}
		}
	}
	var connFunc func() (net.Conn, error)
	if worker.rateLimit == 0 {
		connFunc = worker.serverConnFactory(request.Addr)
	} else {
		if time.Since(worker.rateStartTime) >= worker.rateCooldown {
			worker.rateCounter = 0
			worker.rateStartTime = time.Now()
		}
		if worker.rateCounter < worker.rateLimit {
			worker.rateCounter++
			connFunc = worker.serverConnFactory(request.Addr)
		} else {
			return McAnswer{
				Action: CLOSE,
			}
		}
	}
	return McAnswer{
		Action:         PROXY,
		ProxyCh:        worker.proxyCh,
		ServerConnFunc: connFunc,
	}
}
