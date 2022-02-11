package worker

import (
	"net"

	"github.com/pires/go-proxyproto"
	"github.com/prometheus/client_golang/prometheus"
	"github.com/prometheus/client_golang/prometheus/promauto"
	ultraviolet "github.com/realDragonium/Ultraviolet"
	"github.com/realDragonium/Ultraviolet/config"
	"github.com/realDragonium/Ultraviolet/mc"
	"github.com/realDragonium/Ultraviolet/module"
)

type CheckOpenConns struct {
	Ch chan bool
}

var (
	playersConnected = promauto.NewGaugeVec(prometheus.GaugeOpts{
		Namespace: "ultraviolet",
		Name:      "player_connected",
		Help:      "The total number of connected players",
	}, []string{"host"})
)

type BackendFactoryFunc func(config.BackendWorkerConfig) Backend

type Backend interface {
	ReqCh() chan<- BackendRequest
	HasActiveConn() bool
	Update(cfg BackendConfig) error
	Close()
}

func NewBackendConfig(cfg config.BackendWorkerConfig) BackendConfig {
	var connCreator module.ConnectionCreator
	var hsModifier module.HandshakeModifier
	var rateLimiter module.ConnectionLimiter
	var statusCache module.StatusCache
	var serverState module.StateAgent

	dialer := net.Dialer{
		Timeout: cfg.DialTimeout,
		LocalAddr: &net.TCPAddr{
			IP: net.ParseIP(cfg.ProxyBind),
		},
	}
	connCreator = module.BasicConnCreator(cfg.ProxyTo, dialer)

	if cfg.RateLimit > 0 {
		unverifyCooldown := 10 * cfg.RateLimitDuration
		rateLimiter = module.NewBotFilterConnLimiter(cfg.RateLimit, cfg.RateLimitDuration, unverifyCooldown, cfg.RateBanListCooldown, cfg.RateDisconPk)
	} else {
		rateLimiter = module.AlwaysAllowConnection{}
	}

	if cfg.CacheStatus {
		statusCache = module.NewStatusCache(cfg.ValidProtocol, cfg.CacheUpdateCooldown, connCreator)
	}

	switch cfg.StateOption {
	case config.ALWAYS_ONLINE:
		serverState = module.AlwaysOnlineState{}
	case config.ALWAYS_OFFLINE:
		serverState = module.AlwaysOfflineState{}
	case config.CACHE:
		fallthrough
	default:
		serverState = module.NewMcServerState(cfg.StateUpdateCooldown, connCreator)
	}

	if cfg.OldRealIp {
		hsModifier = module.NewRealIP2_4()
	} else if cfg.NewRealIP {
		hsModifier = module.NewRealIP2_5(cfg.RealIPKey)
	}

	return BackendConfig{
		Name:                cfg.Name,
		UpdateProxyProtocol: true,
		SendProxyProtocol:   cfg.SendProxyProtocol,

		DisconnectPacket:    cfg.DisconnectPacket,
		OfflineStatusPacket: cfg.OfflineStatus,

		ConnCreator: connCreator,
		HsModifier:  hsModifier,
		ConnLimiter: rateLimiter,
		ServerState: serverState,
		StatusCache: statusCache,
	}
}

type BackendConfig struct {
	Name                string
	UpdateProxyProtocol bool
	SendProxyProtocol   bool
	DisconnectPacket    mc.Packet
	OfflineStatusPacket mc.Packet

	HsModifier  module.HandshakeModifier
	ConnCreator module.ConnectionCreator
	ConnLimiter module.ConnectionLimiter
	ServerState module.StateAgent
	StatusCache module.StatusCache
}

var BackendFactory BackendFactoryFunc = func(cfg config.BackendWorkerConfig) Backend {
	backendwrk := NewBackendWorker(cfg)
	backendwrk.Run()
	return &backendwrk
}

func NewBackendWorker(cfgServer config.BackendWorkerConfig) BackendWorker {
	cfg := NewBackendConfig(cfgServer)
	cfg.UpdateProxyProtocol = true
	wrk := NewEmptyBackendWorker()
	wrk.UpdateSameGoroutine(cfg)
	return wrk
}

func NewEmptyBackendWorker() BackendWorker {
	return BackendWorker{
		proxyCh:     make(chan ProxyAction, 10),
		reqCh:       make(chan BackendRequest, 5),
		connCheckCh: make(chan CheckOpenConns),
		updateCh:    make(chan BackendConfig),
		closeCh:     make(chan struct{}),
	}
}

type BackendWorker struct {
	activeConns int
	proxyCh     chan ProxyAction
	reqCh       chan BackendRequest
	connCheckCh chan CheckOpenConns
	updateCh    chan BackendConfig
	closeCh     chan struct{}

	Name                string
	SendProxyProtocol   bool
	OfflineStatusPacket mc.Packet
	DisconnectPacket    mc.Packet

	HsModifier  module.HandshakeModifier
	ConnCreator module.ConnectionCreator
	ConnLimiter module.ConnectionLimiter
	ServerState module.StateAgent
	StatusCache module.StatusCache
}

func (w *BackendWorker) Run() {
	go func(worker BackendWorker) {
		worker.Work()
	}(*w)
}

func (w *BackendWorker) ReqCh() chan<- BackendRequest {
	return w.reqCh
}

func (w *BackendWorker) HasActiveConn() bool {
	ch := make(chan bool)
	checker := CheckOpenConns{
		Ch: ch,
	}
	w.connCheckCh <- checker
	ans := <-ch
	return ans
}

func (w *BackendWorker) Update(cfg BackendConfig) error {
	w.updateCh <- cfg
	return nil
}

func (w *BackendWorker) Close() {
	w.closeCh <- struct{}{}
}

//TODO: Need different name for this...?
func (w *BackendWorker) UpdateSameGoroutine(wCfg BackendConfig) {
	if wCfg.Name != "" {
		playersConnected.Delete(prometheus.Labels{"host": w.Name})
		w.Name = wCfg.Name
		playersConnected.WithLabelValues(w.Name).Add(float64(w.activeConns))
	}
	if wCfg.UpdateProxyProtocol {
		w.SendProxyProtocol = wCfg.SendProxyProtocol
	}
	if len(wCfg.DisconnectPacket.Data) > 0 {
		w.DisconnectPacket = wCfg.DisconnectPacket
	}
	if len(wCfg.OfflineStatusPacket.Data) > 0 {
		w.OfflineStatusPacket = wCfg.OfflineStatusPacket
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
			ans.ServerName = worker.Name
			req.Ch <- ans
		case proxyAction := <-worker.proxyCh:
			worker.proxyRequest(proxyAction)
		case connCheck := <-worker.connCheckCh:
			connCheck.Ch <- worker.activeConns > 0
		case cfg := <-worker.updateCh:
			worker.UpdateSameGoroutine(cfg)
		case <-worker.closeCh:
			close(worker.reqCh)
			return
		}
	}
}

func (worker *BackendWorker) proxyRequest(proxyAction ProxyAction) {
	switch proxyAction {
	case ProxyOpen:
		worker.activeConns++
		playersConnected.WithLabelValues(worker.Name).Inc()
	case ProxyClose:
		worker.activeConns--
		playersConnected.WithLabelValues(worker.Name).Dec()
	}
}

func (worker *BackendWorker) HandleRequest(req BackendRequest) BackendAnswer {
	if worker.ServerState != nil && worker.ServerState.State() == ultraviolet.Offline {
		switch req.ReqData.Type {
		case mc.Status:
			return NewStatusAnswer(worker.OfflineStatusPacket)
		case mc.Login:
			return NewDisconnectAnswer(worker.DisconnectPacket)
		}
	}

	if worker.StatusCache != nil && req.ReqData.Type == mc.Status {
		ans, err := worker.StatusCache.Status()
		if err != nil {
			return NewStatusAnswer(worker.OfflineStatusPacket)
		}
		return NewStatusAnswer(ans)
	}

	if worker.ConnLimiter != nil {
		if ok, _ := worker.ConnLimiter.Allow(req.ReqData); !ok {
			return BackendAnswer{}
		}
	}
	connFunc := worker.ConnCreator.Conn()
	if worker.SendProxyProtocol {
		connFunc = func() (net.Conn, error) {
			addr := req.ReqData.Addr
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
		worker.HsModifier.Modify(&req.ReqData.Handshake, req.ReqData.Addr.String())
	}

	hsPk := req.ReqData.Handshake.Marshal()
	var secondPacket mc.Packet
	switch req.ReqData.Type {
	case mc.Status:
		secondPacket = mc.ServerBoundRequest{}.Marshal()
	case mc.Login:
		secondPacket = mc.ServerLoginStart{Name: mc.String(req.ReqData.Username)}.Marshal()
	}
	return NewProxyAnswer(hsPk, secondPacket, worker.proxyCh, connFunc)
}
