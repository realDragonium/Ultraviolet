package server

import (
	"crypto/ecdsa"
	"errors"
	"io"
	"log"
	"net"
	"time"

	"github.com/pires/go-proxyproto"
	"github.com/realDragonium/Ultraviolet/config"
	"github.com/realDragonium/Ultraviolet/mc"
	"github.com/realDragonium/Ultraviolet/old_proxy"
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

type BackendRequest struct {
	Type      mc.HandshakeState
	Handshake mc.ServerBoundHandshake
	Addr      net.Addr
	Client    net.Conn
	Ch        chan ProcessAnswer
}

type ProcessAnswer struct {
	serverConnFunc func() (net.Conn, error)
	action         BackendAction
	proxyCh        chan ProxyAction
	latency        time.Duration

	firstPacket  []byte
	secondPacket []byte
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
	handshakePacket := mc.ServerBoundHandshake{
		ProtocolVersion: cfg.ValidProtocol,
		ServerAddress:   "Ultraviolet",
		ServerPort:      25565,
		NextState:       1,
	}.Marshal()
	hsByte, _ := handshakePacket.Marshal()

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
		stateUpdateCh:     make(chan ServerState,1),
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
			// worker.ProcessResponse(req, ans)
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
			firstPacket: worker.offlineStatus,
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
	hsBytes, _ := hsPk.Marshal()
	return ProcessAnswer{
		serverConnFunc: connFunc,
		firstPacket:    hsBytes,
		action:         PROXY,
		proxyCh:        worker.proxyCh,
	}
}

func (worker *BackendWorker) ProcessResponse(req BackendRequest, ans ProcessAnswer) {
	switch ans.action {
	case PROXY:
		proxy := ProxySomething{
			client:         req.Client,
			proxyCh:        worker.proxyCh,
			serverConnFunc: ans.serverConnFunc,
			hs:             ans.firstPacket,
			otherPacket:    ans.secondPacket,
		}
		go proxy.ProxyBlindy()
	case DISCONNECT:
		req.Client.Write(ans.Response())
		req.Client.Close()
	case SEND_STATUS:
		req.Client.Write(ans.Response())
		readBuf := make([]byte, 128)
		_, err := req.Client.Read(readBuf)
		if err != nil {
			req.Client.Close()
			return
		}
		if ans.Latency() != 0 {
			time.Sleep(ans.Latency())
		}
		req.Client.Write(readBuf[:])
		req.Client.Close()
	case CLOSE:
		req.Client.Close()
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
	mcConn := old_proxy.NewMcConn(conn)
	conn.Write(worker.statusHandshake)
	mcConn.WritePacket(mc.ServerBoundRequest{}.Marshal())
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

type ProxySomething struct {
	client         net.Conn
	serverConnFunc func() (net.Conn, error)
	proxyCh        chan ProxyAction
	hs             []byte
	otherPacket    []byte
	requestType    mc.HandshakeState
}

func (proxy ProxySomething) ProxyBlindy() {
	sConn, err := proxy.serverConnFunc()
	if err != nil {
		log.Printf("Err when creating server connection: %v", err)
		proxy.client.Close()
		return
	}
	sConn.Write(proxy.hs)
	switch proxy.requestType {
	case mc.LOGIN:
		sConn.Write(proxy.otherPacket)
		go func(client, server net.Conn, proxyCh chan ProxyAction) {
			ProxyLogin(client, server, proxyCh)
		}(proxy.client, sConn, proxy.proxyCh)
	case mc.STATUS:
		// For some unknown reason if we dont send this here
		//  its goes wrong with proxying status requests
		sConn.Write(proxy.otherPacket)
		go func(client, server net.Conn, proxyCh chan ProxyAction) {
			ProxyStatus(client, server, proxyCh)
		}(proxy.client, sConn, proxy.proxyCh)
	}

}

func ProxyLogin(client, server net.Conn, proxyCh chan ProxyAction) {
	proxyCh <- PROXY_OPEN
	// Close behavior doesnt seem to work that well
	go func() {
		io.Copy(server, client)
		client.Close()
	}()
	io.Copy(client, server)
	server.Close()
	proxyCh <- PROXY_CLOSE
}

func ProxyStatus(client, server net.Conn, proxyCh chan ProxyAction) {
	proxyCh <- PROXY_OPEN
	go func() {
		pipe(server, client)
		client.Close()
	}()
	pipe(client, server)
	server.Close()
	proxyCh <- PROXY_CLOSE
}

func pipe(c1, c2 net.Conn) {
	buffer := make([]byte, 0xffff)
	for {
		n, err := c1.Read(buffer)
		if err != nil {
			return
		}
		_, err = c2.Write(buffer[:n])
		if err != nil {
			return
		}
	}
}
