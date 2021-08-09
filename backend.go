package ultraviolet

import (
	"errors"
	"log"
	"net"
	"time"

	"github.com/pires/go-proxyproto"
	"github.com/prometheus/client_golang/prometheus"
	"github.com/prometheus/client_golang/prometheus/promauto"
	"github.com/realDragonium/Ultraviolet/config"
	"github.com/realDragonium/Ultraviolet/mc"
)

var (
	ErrNotValidHandshake = errors.New("not a valid handshake state")
	playersConnected     = promauto.NewGaugeVec(prometheus.GaugeOpts{
		Name: "ultraviolet_connected",
		Help: "The total number of connected players",
	}, []string{"host"})
)

type BackendFactoryFunc func(config.ServerConfig) (Backend, error)

type Backend interface {
	ReqCh() chan<- BackendRequest
	HasActiveConn() bool
	Update(cfg BackendWorkerConfig)
	Close()
}

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

func NewBackendWorkerConfig(cfg config.WorkerServerConfig) BackendWorkerConfig {
	var connCreator ConnectionCreator
	var hsModifier HandshakeModifier
	var rateLimiter ConnectionLimiter
	var statusCache StatusCache
	var serverState StateAgent

	dialer := net.Dialer{
		Timeout: cfg.DialTimeout,
		LocalAddr: &net.TCPAddr{
			IP: net.ParseIP(cfg.ProxyBind),
		},
	}
	connCreator = BasicConnCreator(cfg.ProxyTo, dialer)

	if cfg.RateLimit > 0 {
		rateLimiter = NewBotFilterConnLimiter(cfg.RateLimit, cfg.RateLimitDuration, cfg.RateBanListCooldown, cfg.RateDisconPk)
	} else {
		rateLimiter = AlwaysAllowConnection{}
	}

	if cfg.CacheStatus {
		statusCache = NewStatusCache(cfg.ValidProtocol, cfg.CacheUpdateCooldown, connCreator)
	}

	switch cfg.StateOption {
	case config.ALWAYS_ONLINE:
		serverState = AlwaysOnlineState{}
	case config.ALWAYS_OFFLINE:
		serverState = AlwaysOfflineState{}
	case config.CACHE:
		fallthrough
	default:
		serverState = NewMcServerState(cfg.StateUpdateCooldown, connCreator)
	}

	if cfg.OldRealIp {
		hsModifier = realIPv2_4{}
	} else if cfg.NewRealIP {
		hsModifier = realIPv2_5{realIPKey: cfg.RealIPKey}
	}

	return BackendWorkerConfig{
		Name:                cfg.Name,
		UpdateProxyProtocol: true,
		SendProxyProtocol:   cfg.SendProxyProtocol,

		OfflineDisconnectMessage: cfg.DisconnectPacket,
		OfflineStatus:            cfg.OfflineStatus,

		ConnCreator: connCreator,
		HsModifier:  hsModifier,
		ConnLimiter: rateLimiter,
		ServerState: serverState,
		StatusCache: statusCache,
	}
}

type BackendWorkerConfig struct {
	Name                     string
	UpdateProxyProtocol      bool
	SendProxyProtocol        bool
	OfflineDisconnectMessage mc.Packet
	OfflineStatus            mc.Packet

	HsModifier  HandshakeModifier
	ConnCreator ConnectionCreator
	ConnLimiter ConnectionLimiter
	ServerState StateAgent
	StatusCache StatusCache
}

var BackendFactory BackendFactoryFunc = func(sc config.ServerConfig) (Backend, error) {
	workerConfig, _ := config.FileToWorkerConfig(sc)
	backendWorker := NewBackendWorker(workerConfig)
	go func(worker BackendWorker) {
		worker.Work()
	}(backendWorker)
	return &backendWorker, nil
}

func NewBackendWorker(cfgServer config.WorkerServerConfig) BackendWorker {
	cfg := NewBackendWorkerConfig(cfgServer)
	cfg.UpdateProxyProtocol = true
	worker := NewEmptyBackendWorker()
	worker.UpdateSameGoroutine(cfg)
	return worker
}

func NewEmptyBackendWorker() BackendWorker {
	return BackendWorker{
		proxyCh:     make(chan ProxyAction, 10),
		reqCh:       make(chan BackendRequest, 5),
		connCheckCh: make(chan CheckOpenConns),
		updateCh:    make(chan BackendWorkerConfig),
		closeCh:     make(chan struct{}),
	}
}

type BackendWorker struct {
	activeConns int
	proxyCh     chan ProxyAction
	reqCh       chan BackendRequest
	connCheckCh chan CheckOpenConns
	updateCh    chan BackendWorkerConfig
	closeCh     chan struct{}

	Name                     string
	SendProxyProtocol        bool
	OfflineStatus            mc.Packet
	OfflineDisconnectMessage mc.Packet

	HsModifier  HandshakeModifier
	ConnCreator ConnectionCreator
	ConnLimiter ConnectionLimiter
	ServerState StateAgent
	StatusCache StatusCache
}

func (w *BackendWorker) ReqCh() chan<- BackendRequest {
	return w.reqCh
}

func (w *BackendWorker) HasActiveConn() bool {
	log.Println("going to check")
	ch := make(chan bool)
	checker := CheckOpenConns{
		Ch: ch,
	}
	log.Println(w.connCheckCh)
	w.connCheckCh <- checker
	log.Println("sent request")
	ans := <-ch
	log.Println(ans)
	return ans
}

func (w *BackendWorker) Update(cfg BackendWorkerConfig) {
	w.updateCh <- cfg
}

func (w *BackendWorker) Close() {
	w.closeCh <- struct{}{}
}

//TODO: Need different name for this
func (w *BackendWorker) UpdateSameGoroutine(wCfg BackendWorkerConfig) {
	if wCfg.Name != "" {
		playersConnected.Delete(prometheus.Labels{"host": w.Name})
		w.Name = wCfg.Name
		playersConnected.WithLabelValues(w.Name).Add(float64(w.activeConns))
	}
	if wCfg.UpdateProxyProtocol {
		w.SendProxyProtocol = wCfg.SendProxyProtocol
	}
	if len(wCfg.OfflineDisconnectMessage.Data) > 0 {
		w.OfflineDisconnectMessage = wCfg.OfflineDisconnectMessage
	}
	if len(wCfg.OfflineStatus.Data) > 0 {
		w.OfflineStatus = wCfg.OfflineStatus
	}
	if wCfg.HsModifier != nil {
		w.HsModifier = wCfg.HsModifier
	}
	if wCfg.ConnCreator != nil {
		w.ConnCreator = wCfg.ConnCreator
	}
	if wCfg.ConnLimiter != nil {
		w.ConnLimiter = wCfg.ConnLimiter
	}
	if wCfg.ServerState != nil {
		w.ServerState = wCfg.ServerState
	}
	if wCfg.StatusCache != nil {
		w.StatusCache = wCfg.StatusCache
	}
}

func (worker *BackendWorker) Work() {
	for {
		select {
		case req := <-worker.reqCh:
			ans := worker.HandleRequest(req)
			req.Ch <- ans
		case proxyAction := <-worker.proxyCh:
			log.Println("received proxy start notification")
			worker.proxyRequest(proxyAction)
		case connCheck := <-worker.connCheckCh:
			log.Println(worker.activeConns > 0)
			connCheck.Ch <- worker.activeConns > 0
		case cfg := <-worker.updateCh:
			worker.UpdateSameGoroutine(cfg)
		case <-worker.closeCh:
			return
		}
	}
}

func (worker *BackendWorker) proxyRequest(proxyAction ProxyAction) {
	switch proxyAction {
	case PROXY_OPEN:
		worker.activeConns++
		playersConnected.WithLabelValues(worker.Name).Inc()
	case PROXY_CLOSE:
		worker.activeConns--
		playersConnected.WithLabelValues(worker.Name).Dec()
	}
}

func (worker *BackendWorker) HandleRequest(req BackendRequest) ProcessAnswer {
	if worker.ServerState != nil && worker.ServerState.State() == OFFLINE {
		switch req.Type {
		case mc.STATUS:
			return NewStatusAnswer(worker.OfflineStatus)
		case mc.LOGIN:
			log.Println(worker.OfflineDisconnectMessage)
			return NewDisconnectAnswer(worker.OfflineDisconnectMessage)
		}
	}
	if worker.StatusCache != nil && req.Type == mc.STATUS {
		ans, err := worker.StatusCache.Status()
		if err != nil {
			return NewStatusAnswer(worker.OfflineStatus)
		}
		return ans
	}

	if worker.ConnLimiter != nil {
		if ans, ok := worker.ConnLimiter.Allow(req); !ok {
			return ans
		}
	}
	connFunc := worker.ConnCreator.Conn()
	if worker.SendProxyProtocol {
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
