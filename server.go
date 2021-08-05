package ultraviolet

import (
	"errors"
	"net"
	"time"

	"github.com/pires/go-proxyproto"
	"github.com/prometheus/client_golang/prometheus"
	"github.com/prometheus/client_golang/prometheus/promauto"
	"github.com/realDragonium/Ultraviolet/config"
	"github.com/realDragonium/Ultraviolet/mc"
)

var (
	ErrOverConnRateLimit = errors.New("too many request within rate limit time frame")
	ErrNotValidHandshake = errors.New("not a valid handshake state")
	playersConnected     = promauto.NewGaugeVec(prometheus.GaugeOpts{
		Name: "ultraviolet_connected",
		Help: "The total number of connected players",
	}, []string{"host"})
)

type BackendAction byte

const (
	ERROR BackendAction = iota
	PROXY
	DISCONNECT
	SEND_STATUS
	CLOSE
)

func (state BackendAction) String() string {
	var text string
	switch state {
	case PROXY:
		text = "proxy"
	case DISCONNECT:
		text = "disconnect"
	case SEND_STATUS:
		text = "send_status"
	case CLOSE:
		text = "close"
	case ERROR:
		text = "error"
	}
	return text
}

type ProxyAction int8

const (
	PROXY_OPEN ProxyAction = iota
	PROXY_CLOSE
)

func (action ProxyAction) String() string {
	var text string
	switch action {
	case PROXY_CLOSE:
		text = "Proxy Close"
	case PROXY_OPEN:
		text = "Proxy Open"
	}
	return text
}

type ServerState byte

const (
	UNKNOWN ServerState = iota
	ONLINE
	OFFLINE
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
	}
	return text
}

type BackendRequest struct {
	Type       mc.HandshakeState
	Handshake  mc.ServerBoundHandshake
	ServerAddr string
	Addr       net.Addr
	Username   string
	Ch         chan ProcessAnswer
}

type ProcessAnswer struct {
	serverConnFunc func() (net.Conn, error)
	action         BackendAction
	proxyCh        chan ProxyAction
	latency        time.Duration

	firstPacket  mc.Packet
	secondPacket mc.Packet
}

func NewDisconnectAnswer(p mc.Packet) ProcessAnswer {
	return ProcessAnswer{
		action:      DISCONNECT,
		firstPacket: p,
	}
}

func NewStatusAnswer(p mc.Packet) ProcessAnswer {
	return ProcessAnswer{
		action:      SEND_STATUS,
		firstPacket: p,
	}
}

func NewStatusLatencyAnswer(p mc.Packet, latency time.Duration) ProcessAnswer {
	return ProcessAnswer{
		action:      SEND_STATUS,
		firstPacket: p,
		latency:     latency,
	}
}

func NewProxyAnswer(p1, p2 mc.Packet, proxyCh chan ProxyAction, connFunc func() (net.Conn, error)) ProcessAnswer {
	return ProcessAnswer{
		action:         PROXY,
		serverConnFunc: connFunc,
		firstPacket:    p1,
		secondPacket:   p2,
		proxyCh:        proxyCh,
	}
}

func NewCloseAnswer() ProcessAnswer {
	return ProcessAnswer{
		action: CLOSE,
	}
}

func (ans ProcessAnswer) ServerConn() (net.Conn, error) {
	return ans.serverConnFunc()
}
func (ans ProcessAnswer) Response() mc.Packet {
	return ans.firstPacket
}
func (ans ProcessAnswer) Response2() mc.Packet {
	return ans.secondPacket
}
func (ans ProcessAnswer) ProxyCh() chan ProxyAction {
	return ans.proxyCh
}
func (ans ProcessAnswer) Latency() time.Duration {
	return ans.latency
}
func (ans ProcessAnswer) Action() BackendAction {
	return ans.action
}

func NewBackendWorker(serverId int, cfg config.WorkerServerConfig) BackendWorker {
	var connCreator ConnectionCreator
	var hsModifier HandshakeModifier
	var rateLimiter ConnectionLimiter
	var statusCache StatusCache
	dialer := net.Dialer{
		Timeout: cfg.DialTimeout,
		LocalAddr: &net.TCPAddr{
			IP: net.ParseIP(cfg.ProxyBind),
		},
	}

	connCreator = BasicConnCreator(cfg.ProxyTo, dialer)
	if cfg.RateLimit > 0 {
		rateLimiter = NewAbsConnLimiter(cfg.RateLimit, cfg.RateLimitDuration, cfg.RateLimitStatus)
	} else {
		rateLimiter = AlwaysAllowConnection{}
	}
	if cfg.CacheStatus {
		statusCache = NewStatusCache(cfg.ValidProtocol, cfg.CacheUpdateCooldown, connCreator)
	}
	serverState := McServerState{
		cooldown:    cfg.StateUpdateCooldown,
		connCreator: connCreator,
	}

	if cfg.OldRealIp {
		hsModifier = realIPv2_4{}
	} else if cfg.NewRealIP {
		hsModifier = realIPv2_5{realIPKey: cfg.RealIPKey}
	}

	return BackendWorker{
		identifyingName: cfg.Name,
		ReqCh:           make(chan BackendRequest, 5),
		proxyCh:         make(chan ProxyAction, 10),

		sendProxyProtocol: cfg.SendProxyProtocol,

		offlineDisconnectMessage: cfg.DisconnectPacket,
		offlineStatus:            cfg.OfflineStatus,

		ConnCreator: connCreator,
		HsModifier:  hsModifier,
		ConnLimiter: rateLimiter,
		ServerState: &serverState,
		StatusCache: statusCache,
	}
}

type BackendWorker struct {
	identifyingName string
	activeConns     int
	proxyCh         chan ProxyAction
	ReqCh           chan BackendRequest

	sendProxyProtocol bool

	offlineDisconnectMessage mc.Packet
	offlineStatus            mc.Packet

	HsModifier  HandshakeModifier
	ConnCreator ConnectionCreator
	ConnLimiter ConnectionLimiter
	ServerState StateAgent
	StatusCache StatusCache
}

func (worker *BackendWorker) Work() {
	for {
		select {
		case req := <-worker.ReqCh:
			ans := worker.HandleRequest(req)
			req.Ch <- ans
		case proxyAction := <-worker.proxyCh:
			worker.proxyRequest(proxyAction)
		}
	}
}

func (worker *BackendWorker) proxyRequest(proxyAction ProxyAction) {
	switch proxyAction {
	case PROXY_OPEN:
		worker.activeConns++
		playersConnected.WithLabelValues(worker.identifyingName).Inc()
	case PROXY_CLOSE:
		worker.activeConns--
		playersConnected.WithLabelValues(worker.identifyingName).Dec()
	}
}

func (worker *BackendWorker) HandleRequest(req BackendRequest) ProcessAnswer {
	if worker.ServerState != nil && worker.ServerState.State() == OFFLINE {
		switch req.Type {
		case mc.STATUS:
			return NewStatusAnswer(worker.offlineStatus)
		case mc.LOGIN:
			return NewDisconnectAnswer(worker.offlineDisconnectMessage)
		}
	}
	if worker.StatusCache != nil && req.Type == mc.STATUS {
		ans, err := worker.StatusCache.Status()
		if err != nil {
			return NewStatusAnswer(worker.offlineStatus)
		}
		return ans
	}

	if worker.ConnLimiter != nil {
		if ans, ok := worker.ConnLimiter.Allow(req); !ok {
			return ans
		}
	}
	connFunc := worker.ConnCreator.Conn()
	if worker.sendProxyProtocol {
		connFunc = func() (net.Conn, error) {
			addr := req.Addr
			serverConn, err := worker.ConnCreator.Conn()()
			if err != nil {
				return serverConn, err
			}
			header := &proxyproto.Header{
				Version:           2,
				Command:           proxyproto.PROXY,
				TransportProtocol: proxyproto.TCPv4,
				SourceAddr:        addr,
				DestinationAddr:   serverConn.RemoteAddr(),
			}
			_, err = header.WriteTo(serverConn)
			if err != nil {
				return serverConn, err
			}
			return serverConn, nil
		}
	}

	if worker.HsModifier != nil {
		worker.HsModifier.Modify(&req.Handshake, req.Addr.String())
	}

	hsPk := req.Handshake.Marshal()
	var secondPacket mc.Packet
	switch req.Type {
	case mc.STATUS:
		secondPacket = mc.ServerBoundRequest{}.Marshal()
	case mc.LOGIN:
		secondPacket = mc.ServerLoginStart{Name: mc.String(req.Username)}.Marshal()
	}
	return NewProxyAnswer(hsPk, secondPacket, worker.proxyCh, connFunc)
}
