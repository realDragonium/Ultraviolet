package proxy

import (
	"crypto/ecdsa"
	"errors"
	"net"
	"time"

	"github.com/pires/go-proxyproto"
	"github.com/realDragonium/Ultraviolet/config"
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
			return nil, NewMcAnswerStatus(worker.defaultStatus, 0), false
		} else {
			return nil, NewMcAnswerClose(), false
		}
	}
	return worker.servers[serverId].connReqCh, McAnswerBasic{}, true
}

func NewPrivateWorker(serverId int, cfg config.WorkerServerConfig) PrivateWorker {
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
	handshakePacket := mc.ServerBoundHandshake{
		ProtocolVersion: cfg.ValidProtocol,
		ServerAddress:   "Ultraviolet",
		ServerPort:      25565,
		NextState:       1,
	}.Marshal()

	return PrivateWorker{
		proxyCh:           make(chan ProxyAction),
		rateLimit:         cfg.RateLimit,
		rateCooldown:      cfg.RateLimitDuration,
		stateCooldown:     cfg.StateUpdateCooldown,
		statusCache:       cfg.CacheStatus,
		statusCooldown:    cfg.CacheUpdateCooldown,
		offlineStatus:     cfg.OfflineStatus,
		stateUpdateCh:     make(chan ServerState),
		disconnectPacket:  cfg.DisconnectPacket,
		serverConnFactory: createConnFeature,
		statusHandshake:   handshakePacket,
		useOldRealIP:      cfg.OldRealIp,
		useNewRealIP:      cfg.NewRealIP,
		realIPKey:         cfg.RealIPKey,
	}
}

type PrivateWorker struct {
	activeConns int
	proxyCh     chan ProxyAction
	gatewayCh   chan gatewayRequest
	reqCh       chan McRequest

	rateCounter   int
	rateStartTime time.Time
	rateLimit     int
	rateCooldown  time.Duration

	state         ServerState
	stateCooldown time.Duration
	stateUpdateCh chan ServerState

	offlineStatus   mc.Packet
	cachedStatus    mc.Packet
	statusCache     bool
	statusCooldown  time.Duration
	statusCacheTime time.Time
	statusLatency   time.Duration
	statusHandshake mc.Packet

	useOldRealIP      bool
	useNewRealIP      bool
	realIPKey         *ecdsa.PrivateKey
	serverConnFactory func(net.Addr) func() (net.Conn, error)
	disconnectPacket  mc.Packet
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
		case request := <-worker.gatewayCh:
			request.ch <- worker.activeConns > 0
		}
	}
}

func (worker *PrivateWorker) proxyRequest(proxyAction ProxyAction) {
	switch proxyAction {
	case PROXY_OPEN:
		worker.activeConns++
	case PROXY_CLOSE:
		worker.activeConns--
	}
}

func (worker *PrivateWorker) HandleRequest(request McRequest) McAnswer {
	if worker.state == UNKNOWN {
		worker.updateServerState()
	}
	if worker.state == OFFLINE {
		if request.Type == STATUS {
			return NewMcAnswerStatus(worker.offlineStatus, 0)
		} else if request.Type == LOGIN {
			return NewMcAnswerDisonncet(worker.disconnectPacket)
		}
	}
	if request.Type == STATUS && worker.statusCache {
		if time.Since(worker.statusCacheTime) >= worker.statusCooldown {
			worker.updateCacheStatus()
		}
		return NewMcAnswerStatus(worker.cachedStatus, worker.statusLatency)
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
			return NewMcAnswerClose()
		}
	}
	if request.Type == LOGIN {
		var upgrader func(hs *mc.ServerBoundHandshake) bool
		if worker.useOldRealIP {
			upgrader = func(hs *mc.ServerBoundHandshake) bool {
				hs.UpgradeToOldRealIP(request.Addr.String())
				return true
			}
		}
		if worker.useNewRealIP {
			upgrader = func(hs *mc.ServerBoundHandshake) bool {
				hs.UpgradeToNewRealIP(request.Addr.String(), worker.realIPKey)
				return true
			}
		}
		return NewMcAnswerRealIPProxy(worker.proxyCh, connFunc, upgrader)
	}
	return NewMcAnswerProxy(worker.proxyCh, connFunc)
}

func (worker *PrivateWorker) updateServerState() {
	connFunc := worker.serverConnFactory(&net.IPAddr{})
	_, err := connFunc()
	if err != nil {
		worker.state = OFFLINE
	} else {
		worker.state = ONLINE
	}
	go func(sleepTime time.Duration, updateCh chan ServerState) {
		time.Sleep(sleepTime)
		updateCh <- UNKNOWN
	}(worker.stateCooldown, worker.stateUpdateCh)
}

func (worker *PrivateWorker) updateCacheStatus() {
	connFunc := worker.serverConnFactory(&net.IPAddr{})
	conn, err := connFunc()
	go func(sleepTime time.Duration, updateCh chan ServerState) {
		time.Sleep(sleepTime)
		updateCh <- UNKNOWN
	}(worker.stateCooldown, worker.stateUpdateCh)
	if err != nil {
		worker.state = OFFLINE
		return
	} else {
		worker.state = ONLINE
	}
	mcConn := NewMcConn(conn)
	mcConn.WritePacket(worker.statusHandshake)
	mcConn.WritePacket(mc.ServerBoundRequest{}.Marshal())
	worker.cachedStatus, _ = mcConn.ReadPacket()
	beginTime := time.Now()
	mcConn.WritePacket(mc.NewServerBoundPing().Marshal())
	mcConn.ReadPacket()
	worker.statusLatency = time.Since(beginTime)
	conn.Close()
	worker.statusCacheTime = time.Now()
}
