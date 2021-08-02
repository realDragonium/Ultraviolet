package server

import (
	"crypto/ecdsa"
	"errors"
	"log"
	"net"
	"time"

	"github.com/pires/go-proxyproto"
	"github.com/realDragonium/Ultraviolet/config"
	"github.com/realDragonium/Ultraviolet/mc"
)

var (
	ErrOverConnRateLimit = errors.New("too many request within rate limit time frame")
	ErrNotValidHandshake = errors.New("not a valid handshake state")
)

type BackendAction byte

const (
	PROXY BackendAction = iota
	DISCONNECT
	SEND_STATUS
	CLOSE
	ERROR
)

func (state BackendAction) String() string {
	var text string
	switch state {
	case PROXY:
		text = "Proxy"
	case DISCONNECT:
		text = "Disconnect"
	case SEND_STATUS:
		text = "Send Status"
	case CLOSE:
		text = "Close"
	case ERROR:
		text = "Error"
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

	firstPacket  []byte
	secondPacket []byte
}

func NewDisconnectAnswer(p []byte) ProcessAnswer {
	return ProcessAnswer{
		action:      DISCONNECT,
		firstPacket: p,
	}
}

func NewStatusAnswer(p []byte) ProcessAnswer {
	return ProcessAnswer{
		action:      SEND_STATUS,
		firstPacket: p,
	}
}

func NewStatusLatencyAnswer(p []byte, latency time.Duration) ProcessAnswer {
	return ProcessAnswer{
		action:      SEND_STATUS,
		firstPacket: p,
		latency:     latency,
	}
}

func NewProxyAnswer(p1, p2 []byte, proxyCh chan ProxyAction, connFunc func() (net.Conn, error)) ProcessAnswer {
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
func (ans ProcessAnswer) Response() []byte {
	return ans.firstPacket
}
func (ans ProcessAnswer) Response2() []byte {
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

func StartBackendWorker(serverCfg config.ServerConfig) (chan BackendRequest, error) {
	workerServerCfg, err := config.FileToWorkerConfig2(serverCfg)
	if err != nil {
		return nil, err
	}
	serverWorker := NewBackendWorker(0, workerServerCfg)

	go serverWorker.Work()
	return serverWorker.ReqCh, nil
}

func NewBackendWorker(serverId int, cfg config.WorkerServerConfig2) BackendWorker {
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
	handshake := mc.ServerBoundHandshake{
		ProtocolVersion: cfg.ValidProtocol,
		ServerAddress:   "Ultraviolet",
		ServerPort:      25565,
		NextState:       1,
	}
	handshakePacket := handshake.Marshal()
	hsByte := handshakePacket.Marshal()

	return BackendWorker{
		ReqCh:             make(chan BackendRequest, 25),
		proxyCh:           make(chan ProxyAction, 10),
		rateLimit:         cfg.RateLimit,
		rateLimitStatus:   cfg.RateLimitStatus,
		rateCooldown:      cfg.RateLimitDuration,
		stateCooldown:     cfg.StateUpdateCooldown,
		statusCache:       cfg.CacheStatus,
		statusCooldown:    cfg.CacheUpdateCooldown,
		offlineStatus:     cfg.OfflineStatus,
		stateUpdateCh:     make(chan ServerState, 1),
		disconnectMsg:     cfg.DisconnectPacket,
		serverConnFactory: createConnFeature,
		statusHandshake:   hsByte,
		useOldRealIP:      cfg.OldRealIp,
		useNewRealIP:      cfg.NewRealIP,
		realIPKey:         cfg.RealIPKey,
	}
}

type BackendWorker struct {
	activeConns int
	proxyCh     chan ProxyAction
	ReqCh       chan BackendRequest

	rateCounter     int
	rateStartTime   time.Time
	rateLimit       int
	rateLimitStatus bool
	rateCooldown    time.Duration

	state         ServerState
	stateCooldown time.Duration
	stateUpdateCh chan ServerState

	offlineStatus   []byte
	cachedStatus    []byte
	statusCache     bool
	statusCooldown  time.Duration
	statusCacheTime time.Time
	statusLatency   time.Duration
	statusHandshake []byte

	useOldRealIP      bool
	useNewRealIP      bool
	realIPKey         *ecdsa.PrivateKey
	serverConnFactory func(net.Addr) func() (net.Conn, error)
	disconnectMsg     []byte
}

func (worker *BackendWorker) Work() {
	for {
		select {
		case state := <-worker.stateUpdateCh:
			worker.state = state
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
	case PROXY_CLOSE:
		worker.activeConns--
	}
}

func (worker *BackendWorker) HandleRequest(req BackendRequest) ProcessAnswer {
	if worker.state == UNKNOWN {
		worker.updateServerState()
	}
	if worker.state == OFFLINE {
		if req.Type == mc.STATUS {
			return ProcessAnswer{
				firstPacket: worker.offlineStatus,
				action:      SEND_STATUS,
			}
		} else if req.Type == mc.LOGIN {
			return ProcessAnswer{
				action:      DISCONNECT,
				firstPacket: worker.disconnectMsg,
			}
		}
	}
	if req.Type == mc.STATUS && worker.statusCache {
		if time.Since(worker.statusCacheTime) >= worker.statusCooldown {
			worker.updateCacheStatus()
		}
		return ProcessAnswer{
			firstPacket: worker.cachedStatus,
			action:      SEND_STATUS,
			latency:     worker.statusLatency,
		}
	}
	var connFunc func() (net.Conn, error)
	if worker.rateLimit == 0 || (!worker.rateLimitStatus && req.Type == mc.STATUS) {
		connFunc = worker.serverConnFactory(req.Addr)
	} else {
		if time.Since(worker.rateStartTime) >= worker.rateCooldown {
			worker.rateCounter = 0
			worker.rateStartTime = time.Now()
		}
		if worker.rateCounter < worker.rateLimit {
			worker.rateCounter++
			connFunc = worker.serverConnFactory(req.Addr)
		} else {
			return ProcessAnswer{
				action: CLOSE,
			}
		}
	}
	if req.Type == mc.LOGIN {
		if worker.useOldRealIP {
			req.Handshake.UpgradeToOldRealIP(req.Addr.String())
		}
		if worker.useNewRealIP {
			req.Handshake.UpgradeToNewRealIP(req.Addr.String(), worker.realIPKey)
		}
	}
	hsPk := req.Handshake.Marshal()
	hsBytes := hsPk.Marshal()
	return ProcessAnswer{
		serverConnFunc: connFunc,
		firstPacket:    hsBytes,
		action:         PROXY,
		proxyCh:        worker.proxyCh,
	}
}

func (worker *BackendWorker) updateServerState() {
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

func (worker *BackendWorker) updateCacheStatus() {
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
	mcConn := mc.NewMcConn(conn)
	conn.Write(worker.statusHandshake)
	mcConn.WritePacket(mc.ServerBoundRequestPacket())
	cachedStatus := make([]byte, 0xffffff)
	conn.Read(cachedStatus)
	worker.cachedStatus = cachedStatus
	beginTime := time.Now()
	mcConn.WritePacket(mc.NewServerBoundPing().Marshal())
	mcConn.ReadPacket()
	worker.statusLatency = time.Since(beginTime) / 2
	conn.Close()
	worker.statusCacheTime = time.Now()
}

//////////////////////////
//////////////////////////
//////////////////////////
//////////////////////////
//////////////////////////

func NewBasicBackendWorker(serverId int, cfg config.WorkerServerConfig2) BasicBackendWorker {
	var connCreator ConnectionCreator
	var hsModifier HandshakeModifier
	var rateLimiter ConnectionLimiter
	var statusCache statusCache
	dialer := net.Dialer{
		Timeout: cfg.DialTimeout,
		LocalAddr: &net.TCPAddr{
			IP: net.ParseIP(cfg.ProxyBind),
		},
	}

	connCreator = BasicConnCreator(cfg.ProxyTo, dialer)
	if cfg.RateLimit > 0 {
		rateLimiter = NewRateLimiter(cfg.RateLimit, cfg.RateLimitDuration)
	}
	if cfg.CacheStatus {
		statusCache = NewStatusCache(cfg.ValidProtocol, cfg.CacheUpdateCooldown, connCreator)
	}
	serverState := McServerState{
		cooldown:    cfg.StateUpdateCooldown,
		connCreator: connCreator,
	}

	if cfg.OldRealIp {
		hsModifier = RealIPv2_4{}
	} else if cfg.NewRealIP {
		hsModifier = RealIPv2_5{realIPKey: cfg.RealIPKey}
	}

	return BasicBackendWorker{
		ReqCh:   make(chan BackendRequest, 25),
		proxyCh: make(chan ProxyAction, 10),

		sendProxyProtocol: cfg.SendProxyProtocol,
		rateLimit:         cfg.RateLimit > 0,
		rateLimitStatus:   cfg.RateLimitStatus,

		offlineDisconnectMessage: cfg.DisconnectPacket,
		offlineStatus:            cfg.OfflineStatus,

		cacheStatus: cfg.CacheStatus,

		connCreator: connCreator,
		hsModifier:  hsModifier,
		rateLimiter: rateLimiter,
		state:       serverState,
		statusCache: statusCache,
	}
}

type BasicBackendWorker struct {
	activeConns int
	proxyCh     chan ProxyAction
	ReqCh       chan BackendRequest

	sendProxyProtocol bool
	cacheStatus       bool
	rateLimit         bool
	rateLimitStatus   bool

	offlineDisconnectMessage []byte
	offlineStatus            []byte

	hsModifier  HandshakeModifier
	connCreator ConnectionCreator
	rateLimiter ConnectionLimiter
	state       McServerState
	statusCache statusCache
}

func (worker *BasicBackendWorker) Work() {
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

func (worker *BasicBackendWorker) proxyRequest(proxyAction ProxyAction) {
	switch proxyAction {
	case PROXY_OPEN:
		worker.activeConns++
	case PROXY_CLOSE:
		worker.activeConns--
	}
}

func (worker *BasicBackendWorker) HandleRequest(req BackendRequest) ProcessAnswer {
	if worker.state.serverState() == OFFLINE {
		switch req.Type {
		case mc.STATUS:
			return NewStatusAnswer(worker.offlineStatus)
		case mc.LOGIN:
			return NewDisconnectAnswer(worker.offlineDisconnectMessage)
		}
	}

	if worker.cacheStatus && req.Type == mc.STATUS {
		ans, err := worker.statusCache.Status()
		if err != nil && !errors.Is(err, ErrStatusPing) {
			log.Println(err)
			return NewStatusAnswer(worker.offlineStatus)
		}
		return ans
	}

	connFunc := worker.connCreator.Conn()
	if worker.rateLimit && !worker.rateLimiter.Allow() {
		if req.Type == mc.LOGIN || (worker.rateLimitStatus && req.Type == mc.STATUS) {
			return NewCloseAnswer()
		}
	}

	if worker.sendProxyProtocol {
		connFunc = func() (net.Conn, error) {
			addr := req.Addr
			serverConn, err := worker.connCreator.Conn()()
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

	if worker.hsModifier != nil {
		worker.hsModifier.Modify(&req.Handshake, req.Addr.String())
	}

	hsPk := req.Handshake.Marshal()
	hsBytes := hsPk.Marshal()
	var secondPacket mc.Packet
	switch req.Type {
	case mc.STATUS:
		secondPacket = mc.ServerBoundRequest{}.Marshal()
	case mc.LOGIN:
		secondPacket = mc.ServerLoginStart{Name: mc.String(req.Username)}.Marshal()
	}
	secondPkBytes := secondPacket.Marshal()
	return NewProxyAnswer(hsBytes, secondPkBytes, worker.proxyCh, connFunc)
}

type HandshakeModifier interface {
	Modify(hs *mc.ServerBoundHandshake, addr string)
}

type RealIPv2_4 struct{}

func (rip RealIPv2_4) Modify(hs *mc.ServerBoundHandshake, addr string) {
	hs.UpgradeToOldRealIP(addr)
}

type RealIPv2_5 struct {
	realIPKey *ecdsa.PrivateKey
}

func (rip RealIPv2_5) Modify(hs *mc.ServerBoundHandshake, addr string) {
	hs.UpgradeToNewRealIP(addr, rip.realIPKey)
}

type ConnectionCreator interface {
	Conn() func() (net.Conn, error)
}

type ConnectionCreatorFunc func() (net.Conn, error)

func (creator ConnectionCreatorFunc) Conn() func() (net.Conn, error) {
	return creator
}

type ConnectionLimiter interface {
	Allow() bool
}

func NewRateLimiter(ratelimit int, cooldown time.Duration) ConnectionLimiter {
	return &ratelimiter{
		rateLimit:    ratelimit,
		rateCooldown: cooldown,
	}
}

type ratelimiter struct {
	rateCounter   int
	rateStartTime time.Time
	rateLimit     int
	rateCooldown  time.Duration
}

func (r *ratelimiter) Allow() bool {
	var answer bool
	if time.Since(r.rateStartTime) >= r.rateCooldown {
		r.rateCounter = 0
		r.rateStartTime = time.Now()
	}
	if r.rateCounter < r.rateLimit {
		r.rateCounter++
		answer = true
	}
	return answer
}

func BasicConnCreator(proxyTo string, dialer net.Dialer) ConnectionCreatorFunc {
	return func() (net.Conn, error) {
		return dialer.Dial("tcp", proxyTo)
	}
}

type McServerState struct {
	state       ServerState
	cooldown    time.Duration
	startTime   time.Time
	connCreator ConnectionCreator
}

func (server *McServerState) serverState() ServerState {
	if server.state != UNKNOWN && time.Since(server.startTime) <= server.cooldown {
		return server.state
	}
	server.startTime = time.Now()
	connFunc := server.connCreator.Conn()
	conn, err := connFunc()
	if err != nil {
		server.state = OFFLINE
	} else {
		server.state = ONLINE
		conn.Close()
	}
	return server.state
}

func NewStatusCache(protocol int, cooldown time.Duration, connCreator ConnectionCreator) statusCache {
	handshakePacket := mc.ServerBoundHandshake{
		ProtocolVersion: protocol,
		ServerAddress:   "Ultraviolet",
		ServerPort:      25565,
		NextState:       1,
	}.Marshal()
	hsByte := handshakePacket.Marshal()

	return statusCache{
		connCreator: connCreator,
		cooldown:    cooldown,
		handshake:   hsByte,
	}
}

type statusCache struct {
	connCreator ConnectionCreator

	status    ProcessAnswer
	cooldown  time.Duration
	cacheTime time.Time
	handshake []byte
}

var ErrStatusPing = errors.New("something went wrong while pinging")

func (cache *statusCache) Status() (ProcessAnswer, error) {
	if time.Since(cache.cacheTime) < cache.cooldown {
		return cache.status, nil
	}
	var answer ProcessAnswer
	connFunc := cache.connCreator.Conn()
	conn, err := connFunc()
	if err != nil {
		return answer, err
	}
	cacheBuffer := make([]byte, 0xffffff)
	mcConn := mc.NewMcConn(conn)
	if _, err := conn.Write(cache.handshake); err != nil {
		return answer, err
	}
	if err := mcConn.WritePacket(mc.ServerBoundRequest{}.Marshal()); err != nil {
		return answer, err
	}
	n, err := conn.Read(cacheBuffer)
	if err != nil {
		return answer, err
	}
	beginTime := time.Now()
	statusBytes := cacheBuffer[:n]
	var latency time.Duration = 0
	if err := mcConn.WritePacket(mc.NewServerBoundPing().Marshal()); err != nil {
		cache.status = NewStatusLatencyAnswer(statusBytes, latency)
		return cache.status, ErrStatusPing
	}
	if _, err := mcConn.ReadPacket(); err != nil {
		cache.status = NewStatusLatencyAnswer(statusBytes, latency)
		return cache.status, ErrStatusPing
	}
	conn.Close()
	latency = time.Since(beginTime) / 2
	answer = NewStatusLatencyAnswer(statusBytes, latency)
	cache.status = answer
	return answer, nil
}
