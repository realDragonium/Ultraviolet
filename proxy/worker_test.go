package proxy_test

import (
	"bytes"
	"errors"
	"fmt"
	"io"
	"log"
	"net"
	"strings"
	"sync"
	"testing"
	"time"

	"github.com/pires/go-proxyproto"
	"github.com/realDragonium/Ultraviolet/config"
	"github.com/realDragonium/Ultraviolet/mc"
	"github.com/realDragonium/Ultraviolet/proxy"
)

var LoginStatusTestCases = []struct {
	reqType         proxy.McRequestType
	denyAction      proxy.McAction
	unknownAction   proxy.McAction
	onlineAction    proxy.McAction
	offlineAction   proxy.McAction
	rateLimitAction proxy.McAction
}{
	{
		reqType:         proxy.STATUS,
		denyAction:      proxy.CLOSE,
		unknownAction:   proxy.SEND_STATUS,
		onlineAction:    proxy.PROXY,
		offlineAction:   proxy.SEND_STATUS,
		rateLimitAction: proxy.CLOSE,
	},
	{
		reqType:         proxy.LOGIN,
		denyAction:      proxy.DISCONNECT,
		unknownAction:   proxy.CLOSE,
		onlineAction:    proxy.PROXY,
		offlineAction:   proxy.DISCONNECT,
		rateLimitAction: proxy.CLOSE,
	},
}

var ErrNoResponse = errors.New("there was no response from worker")

var port *int16
var portLock sync.Mutex = sync.Mutex{}

// To make sure every test gets its own unique port
func testAddr() string {
	portLock.Lock()
	defer portLock.Unlock()
	if port == nil {
		port = new(int16)
		*port = 25500
	}
	addr := fmt.Sprintf("127.0.0.1:%d", *port)
	*port++
	return addr
}

type testLogger struct {
	t *testing.T
}

func (tLog *testLogger) Write(b []byte) (n int, err error) {
	tLog.t.Logf(string(b))
	return 0, nil
}

func unknownServerStatusPk() mc.Packet {
	return unknownServerStatus().Marshal()
}

func unknownServerStatus() mc.SimpleStatus {
	return mc.SimpleStatus{
		Name:        "Ultraviolet",
		Protocol:    0,
		Description: "No server found",
	}
}

func defaultOfflineStatusPacket() mc.Packet {
	return defaultOfflineStatus().Marshal()
}

func defaultOfflineStatus() mc.SimpleStatus {
	return mc.SimpleStatus{
		Name:        "Ultraviolet-ff",
		Protocol:    755,
		Description: "offline proxy being tested",
	}
}

//Test Help methods
func setupWorkers(cfg config.UltravioletConfig, serverCfgs ...config.ServerConfig) chan<- proxy.McRequest {
	reqCh := make(chan proxy.McRequest)
	gateway := proxy.NewGateway()
	gateway.StartWorkers(cfg, serverCfgs, reqCh)
	return reqCh
}

func simpleUltravioletConfig(logOutput io.Writer) config.UltravioletConfig {
	log.SetPrefix("log-text: ")
	log.SetFlags(0)
	return config.UltravioletConfig{
		DefaultStatus:   unknownServerStatus(),
		NumberOfWorkers: 1,
		LogOutput:       logOutput,
	}
}

func sendRequest_TestTimeout(t *testing.T, reqCh chan<- proxy.McRequest, req proxy.McRequest) proxy.McAnswer {
	t.Helper()
	answerCh := make(chan proxy.McAnswer)
	req.Ch = answerCh
	reqCh <- req
	select {
	case answer := <-answerCh:
		t.Log("worker has successfully responded")
		return answer
	case <-time.After(defaultChTimeout):
		t.Fatal("timed out")
	}
	return proxy.McAnswerBasic{}
}

func sendRequest_IgnoreResult(reqCh chan<- proxy.McRequest, req proxy.McRequest) {
	answerCh := make(chan proxy.McAnswer)
	req.Ch = answerCh
	reqCh <- req
	go func() {
		<-answerCh
	}()
}

func testCloseConnection(t *testing.T, conn net.Conn) {
	if _, err := conn.Write([]byte{0}); err != nil {
		t.Errorf("Got an unexpected error: %v", err)
	}
}

func samePk(expected, received mc.Packet) bool {
	sameID := expected.ID == received.ID
	sameData := bytes.Equal(expected.Data, received.Data)

	return sameID && sameData
}

func netAddrToIp(addr net.Addr) string {
	return strings.Split(addr.String(), ":")[0]
}

func acceptAllConnsListener(t *testing.T, addr string) {
	connCh, _ := createListener(t, addr)
	go func() {
		for {
			<-connCh
		}
	}()
}

func createListener(t *testing.T, addr string) (<-chan net.Conn, <-chan error) {
	connCh := make(chan net.Conn)
	errorCh := make(chan error)
	listener, err := net.Listen("tcp", addr)
	if err != nil {
		t.Fatal(err)
	}
	go func() {
		for {
			conn, err := listener.Accept()
			if err != nil {
				errorCh <- err
			}
			connCh <- conn
		}
	}()
	return connCh, errorCh
}

type createWorker func(t *testing.T, serverCfg ...config.ServerConfig) chan<- proxy.McRequest

// Actual Tests
func TestPublicAndPrivateWorkers(t *testing.T) {
	setupWorker := func(tLocal *testing.T, serverCfgs ...config.ServerConfig) chan<- proxy.McRequest {
		logOutput := &testLogger{t: tLocal}
		reqCh := setupWorkers(simpleUltravioletConfig(logOutput), serverCfgs...)
		return reqCh
	}
	runAllWorkerTests(t, setupWorker)
}

func runAllWorkerTests(t *testing.T, newWorker createWorker) {
	t.Run("Can receive request", func(t *testing.T) {
		testWorker_CanReceiveRequest(t, newWorker)
	})
	t.Run("unkown address", func(t *testing.T) {
		testUnknownAddr(t, newWorker)
	})
	t.Run("known address - offline server", func(t *testing.T) {
		testKnownAddr_OfflineServer(t, newWorker)
	})
	t.Run("known address - online server", func(t *testing.T) {
		testKnownAddr_OnlineServer(t, newWorker)
	})
	t.Run("known address - RealIP proxy", func(t *testing.T) {
		testKnownAddr_OldRealIPProxy(t, newWorker)
	})
	t.Run("proxy bind", func(t *testing.T) {
		testProxyBind(t, newWorker)
	})
	t.Run("proxy protocol", func(t *testing.T) {
		testProxyProtocol(t, newWorker)
	})
	t.Run("rate limit - too many requests will trigger it", func(t *testing.T) {
		testProxy_ManyRequestsWillRateLimit(t, newWorker)
	})
	t.Run("rate limit - allow new connections after ratelimit cooldown", func(t *testing.T) {
		testProxy_WillAllowNewConn_AfterDurationEnded(t, newWorker)
	})
	t.Run("rate limit - too many requests will trigger it", func(t *testing.T) {
		testProxy_ManyRequestsWillNotRateLimitStatus(t, newWorker)
	})
	t.Run("server state - doesnt call backend again before cooldown is over", func(t *testing.T) {
		testServerState_DoesntCallBeforeCooldownIsOver(t, newWorker)
	})
	t.Run("server state - call backend again when cooldown is over", func(t *testing.T) {
		testServerState_ShouldCallAgainOutOfCooldown(t, newWorker)
	})
	t.Run("status cache - doesnt call backend again before cooldown is over", func(t *testing.T) {
		testStatusCache_DoesntCallBeforeCooldownIsOver(t, newWorker)
	})
	t.Run("status cache - call backend again when cooldown is over", func(t *testing.T) {
		testStatusCache_ShouldCallAgainOutOfCooldown(t, newWorker)
	})
}

func testWorker_CanReceiveRequest(t *testing.T, newWorker createWorker) {
	serverCfg := config.ServerConfig{}
	reqCh := newWorker(t, serverCfg)
	select {
	case reqCh <- proxy.McRequest{}:
		t.Log("worker has successfully received request")
	case <-time.After(defaultChTimeout):
		t.Error("timed out")
	}
}

func testUnknownAddr(t *testing.T, newWorker createWorker) {
	for _, tc := range LoginStatusTestCases {
		t.Run(fmt.Sprintf("reqType-%v", tc.reqType), func(t *testing.T) {
			serverCfg := config.ServerConfig{}
			req := proxy.McRequest{
				Type:       tc.reqType,
				ServerAddr: "some weird server address",
			}
			reqCh := newWorker(t, serverCfg)
			answer := sendRequest_TestTimeout(t, reqCh, req)
			if answer.Action() != tc.unknownAction {
				t.Errorf("expected: %v \ngot: %v", tc.unknownAction, answer.Action())
			}
			if tc.reqType == proxy.STATUS {
				defaultStatusPk := unknownServerStatusPk()
				if !samePk(defaultStatusPk, answer.FirstPacket()) {
					defaultStatus, _ := mc.UnmarshalClientBoundResponse(defaultStatusPk)
					receivedStatus, _ := mc.UnmarshalClientBoundResponse(answer.FirstPacket())
					t.Errorf("expected: %v \ngot: %v", defaultStatus, receivedStatus)
				}
			}
		})
	}
}

func testKnownAddr_OfflineServer(t *testing.T, newWorker createWorker) {
	for _, tc := range LoginStatusTestCases {
		t.Run(fmt.Sprintf("reqType-%v", tc.reqType), func(t *testing.T) {
			serverAddr := "ultraviolet"
			disconnectMessage := "Some disconnect message right here"
			disconPacket := mc.ClientBoundDisconnect{
				Reason: mc.Chat(disconnectMessage),
			}.Marshal()
			serverCfg := config.ServerConfig{
				Domains:           []string{serverAddr},
				OfflineStatus:     defaultOfflineStatus(),
				DisconnectMessage: disconnectMessage,
			}
			req := proxy.McRequest{
				Type:       tc.reqType,
				ServerAddr: serverAddr,
			}
			offlineStatusPk := defaultOfflineStatusPacket()
			reqCh := newWorker(t, serverCfg)
			answer := sendRequest_TestTimeout(t, reqCh, req)
			if answer.Action() != tc.offlineAction {
				t.Errorf("expected: %v \ngot: %v", tc.offlineAction, answer.Action())
			}
			if tc.reqType == proxy.STATUS {
				if !samePk(offlineStatusPk, answer.FirstPacket()) {
					offlineStatus, _ := mc.UnmarshalClientBoundResponse(offlineStatusPk)
					receivedStatus, _ := mc.UnmarshalClientBoundResponse(answer.FirstPacket())
					t.Errorf("expected: %v \ngot: %v", offlineStatus, receivedStatus)
				}
			} else if tc.reqType == proxy.LOGIN {
				if !samePk(disconPacket, answer.FirstPacket()) {
					expected, _ := mc.UnmarshalClientDisconnect(disconPacket)
					received, _ := mc.UnmarshalClientDisconnect(answer.FirstPacket())
					t.Errorf("expected: %v \ngot: %v", expected, received)
				}
			}
		})
	}
}

func testKnownAddr_OnlineServer(t *testing.T, newWorker createWorker) {
	for _, tc := range LoginStatusTestCases {
		t.Run(fmt.Sprintf("reqType-%v", tc.reqType), func(t *testing.T) {
			serverAddr := "ultraviolet"
			targetAddr := testAddr()
			serverCfg := config.ServerConfig{
				Domains: []string{serverAddr},
				ProxyTo: targetAddr,
			}
			req := proxy.McRequest{
				Type:       tc.reqType,
				ServerAddr: serverAddr,
			}
			createListener(t, targetAddr)
			reqCh := newWorker(t, serverCfg)
			answer := sendRequest_TestTimeout(t, reqCh, req)
			if answer.Action() != tc.onlineAction {
				t.Fatalf("expected: %v \ngot: %v", tc.onlineAction, answer.Action())
			}
			serverConn, _ := answer.ServerConn()
			testCloseConnection(t, serverConn)
			if answer.ProxyCh() == nil {
				t.Error("No proxy channel provided")
			}
		})
	}
}

// Add new RealIP proxy
func testKnownAddr_OldRealIPProxy(t *testing.T, newWorker createWorker) {
	serverAddr := "ultraviolet"
	targetAddr := testAddr()
	serverCfg := config.ServerConfig{
		Domains:   []string{serverAddr},
		ProxyTo:   targetAddr,
		OldRealIP: true,
	}
	req := proxy.McRequest{
		Type:       proxy.LOGIN,
		ServerAddr: serverAddr,
		Addr: &net.IPAddr{
			IP: net.ParseIP("1.1.1.1"),
		},
	}
	createListener(t, targetAddr)
	reqCh := newWorker(t, serverCfg)
	answer := sendRequest_TestTimeout(t, reqCh, req)
	if answer.Action() != proxy.PROXY {
		t.Fatalf("expected: %v \ngot: %v", proxy.PROXY, answer.Action())
	}
	hs := mc.ServerBoundHandshake{
		ServerAddress: "ultraviolet",
	}
	if !answer.UpgradeToRealIp(&hs) {
		t.Error("Should have updated to RealIP format")
	}
}

func testProxyBind(t *testing.T, newWorker createWorker) {
	for _, tc := range LoginStatusTestCases {
		t.Run(fmt.Sprintf("reqType-%v", tc.reqType), func(t *testing.T) {
			serverAddr := "ultraviolet"
			targetAddr := testAddr()
			proxyBind := "127.0.0.2"
			serverCfg := config.ServerConfig{
				Domains:   []string{serverAddr},
				ProxyTo:   targetAddr,
				ProxyBind: proxyBind,
			}
			req := proxy.McRequest{
				Type:       tc.reqType,
				ServerAddr: serverAddr,
			}

			go func() {
				reqCh := newWorker(t, serverCfg)
				answer := sendRequest_TestTimeout(t, reqCh, req)
				answer.ServerConn() // Calling it instead of the player's goroutine
			}()

			connCh, errorCh := createListener(t, targetAddr)
			conn := <-connCh // State check call (proxy bind should be used here too)
			if netAddrToIp(conn.RemoteAddr()) != proxyBind {
				t.Errorf("expected: %v \ngot: %v", proxyBind, netAddrToIp(conn.RemoteAddr()))
			}

			select {
			case err := <-errorCh:
				t.Fatalf("error while accepting connection: %v", err)
			case conn := <-connCh:
				t.Log("connection has been created")
				if netAddrToIp(conn.RemoteAddr()) != proxyBind {
					t.Errorf("expected: %v \ngot: %v", proxyBind, netAddrToIp(conn.RemoteAddr()))
				}
			case <-time.After(defaultChTimeout):
				t.Error("timed out")
			}
		})
	}
}

func testProxyProtocol(t *testing.T, newWorker createWorker) {
	for _, tc := range LoginStatusTestCases {
		t.Run(fmt.Sprintf("reqType-%v", tc.reqType), func(t *testing.T) {
			serverAddr := "ultraviolet"
			targetAddr := testAddr()
			t.Log(targetAddr)
			playerAddr := &net.TCPAddr{
				IP:   net.ParseIP("187.34.26.123"),
				Port: 49473,
			}
			serverCfg := config.ServerConfig{
				Domains:           []string{serverAddr},
				ProxyTo:           targetAddr,
				SendProxyProtocol: true,
			}
			req := proxy.McRequest{
				Type:       tc.reqType,
				ServerAddr: serverAddr,
				Addr:       playerAddr,
			}
			listener, err := net.Listen("tcp", targetAddr)
			if err != nil {
				t.Fatal(err)
			}
			proxyListener := &proxyproto.Listener{Listener: listener}
			connCh := make(chan net.Conn)
			errorCh := make(chan error)
			go func() {
				for {
					conn, err := proxyListener.Accept()
					if err != nil {
						errorCh <- err
					}
					connCh <- conn
				}
			}()

			go func() {
				reqCh := newWorker(t, serverCfg)
				answer := sendRequest_TestTimeout(t, reqCh, req)
				answer.ServerConn() // Calling it instead of the player's goroutine
			}()

			<-connCh // State check call (no proxy protocol in here)
			select {
			case err := <-errorCh:
				t.Fatalf("error while accepting connection: %v", err)
			case conn := <-connCh:
				t.Log("connection has been created")
				if conn.RemoteAddr().String() != playerAddr.String() {
					t.Errorf("expected: %v \ngot: %v", playerAddr, conn.RemoteAddr())
				}
			case <-time.After(defaultChTimeout):
				t.Error("timed out")
			}
		})
	}
}

func testProxy_ManyRequestsWillRateLimit(t *testing.T, newWorker createWorker) {
	for _, tc := range LoginStatusTestCases {
		t.Run(fmt.Sprintf("reqType-%v", tc.reqType), func(t *testing.T) {
			serverAddr := "ultraviolet"
			targetAddr := testAddr()
			rateLimit := 3
			rateLimitDuration := time.Minute
			serverCfg := config.ServerConfig{
				Domains:         []string{serverAddr},
				ProxyTo:         targetAddr,
				RateLimit:       rateLimit,
				RateLimitStatus: true,
				RateDuration:    rateLimitDuration.String(),
			}
			reqCh := newWorker(t, serverCfg)
			req := proxy.McRequest{
				Type:       tc.reqType,
				ServerAddr: serverAddr,
			}
			acceptAllConnsListener(t, targetAddr)
			for i := 0; i < rateLimit; i++ {
				sendRequest_IgnoreResult(reqCh, req)
			}
			answer := sendRequest_TestTimeout(t, reqCh, req)
			if answer.Action() != tc.rateLimitAction {
				t.Fatalf("expected: %v \ngot: %v", tc.rateLimitAction, answer.Action())
			}
		})
	}
}

func testProxy_WillAllowNewConn_AfterDurationEnded(t *testing.T, newWorker createWorker) {
	for _, tc := range LoginStatusTestCases {
		t.Run(fmt.Sprintf("reqType-%v", tc.reqType), func(t *testing.T) {
			serverAddr := "ultraviolet"
			targetAddr := testAddr()
			rateLimit := 1
			rateLimitDuration := defaultChTimeout
			serverCfg := config.ServerConfig{
				Domains:         []string{serverAddr},
				ProxyTo:         targetAddr,
				RateLimit:       rateLimit,
				RateLimitStatus: true,
				RateDuration:    rateLimitDuration.String(),
			}
			reqCh := newWorker(t, serverCfg)
			req := proxy.McRequest{
				Type:       tc.reqType,
				ServerAddr: serverAddr,
			}
			acceptAllConnsListener(t, targetAddr)
			for i := 0; i < rateLimit; i++ {
				sendRequest_IgnoreResult(reqCh, req)
			}
			time.Sleep(longerChTimeout)

			answer := sendRequest_TestTimeout(t, reqCh, req)
			if answer.Action() != tc.onlineAction {
				t.Fatalf("expected: %v \ngot: %v", tc.onlineAction, answer.Action())
			}
		})
	}
}

func testProxy_ManyRequestsWillNotRateLimitStatus(t *testing.T, newWorker createWorker) {
	serverAddr := "ultraviolet"
	targetAddr := testAddr()
	rateLimit := 3
	rateLimitDuration := time.Minute
	serverCfg := config.ServerConfig{
		Domains:         []string{serverAddr},
		ProxyTo:         targetAddr,
		RateLimit:       rateLimit,
		RateLimitStatus: false,
		RateDuration:    rateLimitDuration.String(),
	}
	reqCh := newWorker(t, serverCfg)
	req := proxy.McRequest{
		Type:       proxy.STATUS,
		ServerAddr: serverAddr,
	}
	acceptAllConnsListener(t, targetAddr)
	for i := 0; i < rateLimit; i++ {
		sendRequest_IgnoreResult(reqCh, req)
	}
	answer := sendRequest_TestTimeout(t, reqCh, req)
	if answer.Action() != proxy.PROXY {
		t.Fatalf("expected: %v \ngot: %v", proxy.PROXY, answer.Action())
	}
}

func testServerState_DoesntCallBeforeCooldownIsOver(t *testing.T, newWorker createWorker) {
	for _, tc := range LoginStatusTestCases {
		t.Run(fmt.Sprintf("reqType-%v", tc.reqType), func(t *testing.T) {
			serverAddr := "ultraviolet"
			targetAddr := testAddr()
			updateCooldown := time.Minute
			serverCfg := config.ServerConfig{
				Domains:             []string{serverAddr},
				ProxyTo:             targetAddr,
				StateUpdateCooldown: updateCooldown.String(),
			}
			reqCh := newWorker(t, serverCfg)
			req := proxy.McRequest{
				Type:       tc.reqType,
				ServerAddr: serverAddr,
			}
			sendRequest_TestTimeout(t, reqCh, req)
			connCh, _ := createListener(t, targetAddr)
			sendRequest_IgnoreResult(reqCh, req)
			select {
			case <-connCh:
				t.Error("worker called server again")
			case <-time.After(defaultChTimeout):
				t.Log("worker didnt call again")
			}
		})
	}
}

func testServerState_ShouldCallAgainOutOfCooldown(t *testing.T, newWorker createWorker) {
	for _, tc := range LoginStatusTestCases {
		t.Run(fmt.Sprintf("reqType-%v", tc.reqType), func(t *testing.T) {
			serverAddr := "ultraviolet"
			targetAddr := testAddr()
			updateCooldown := defaultChTimeout
			serverCfg := config.ServerConfig{
				Domains:             []string{serverAddr},
				ProxyTo:             targetAddr,
				StateUpdateCooldown: updateCooldown.String(),
				DialTimeout:         "1s",
			}
			reqCh := newWorker(t, serverCfg)
			req := proxy.McRequest{
				Type:       tc.reqType,
				ServerAddr: serverAddr,
			}
			sendRequest_TestTimeout(t, reqCh, req)
			connCh, _ := createListener(t, targetAddr)
			time.Sleep(defaultChTimeout * 2)
			sendRequest_IgnoreResult(reqCh, req)
			select {
			case <-connCh: // receiving the state call
				t.Log("worker has successfully responded")
			case <-time.After(defaultChTimeout):
				t.Error("timed out")
			}
		})
	}
}

func testStatusCache_DoesntCallBeforeCooldownIsOver(t *testing.T, newWorker createWorker) {
	serverAddr := "ultraviolet"
	targetAddr := testAddr()
	cacheCooldown := time.Minute
	serverCfg := config.ServerConfig{
		Domains:             []string{serverAddr},
		ProxyTo:             targetAddr,
		CacheStatus:         true,
		CacheUpdateCooldown: cacheCooldown.String(),
		StateUpdateCooldown: cacheCooldown.String(),
	}
	reqCh := newWorker(t, serverCfg)
	answerCh := make(chan proxy.McAnswer)
	req := proxy.McRequest{
		Type:       proxy.STATUS,
		ServerAddr: serverAddr,
		Ch:         answerCh,
	}
	sendRequest_TestTimeout(t, reqCh, req)
	connCh, _ := createListener(t, targetAddr)
	reqCh <- req
	select {
	case <-connCh:
		t.Error("worker called server again")
	case <-time.After(defaultChTimeout):
		t.Log("worker didnt call again")
	}

	select {
	case answer := <-answerCh:
		t.Log("worker has successfully responded")
		if answer.Action() != proxy.SEND_STATUS {
			t.Errorf("expected: %v \ngot: %v", proxy.SEND_STATUS, answer.Action())
		}
	case <-time.After(defaultChTimeout):
		t.Error("timed out")
	}
}

func testStatusCache_ShouldCallAgainOutOfCooldown(t *testing.T, newWorker createWorker) {
	statusPacket := mc.SimpleStatus{
		Name:        "UV",
		Protocol:    700,
		Description: "Some test text",
	}.Marshal()
	serverAddr := "ultraviolet"
	targetAddr := testAddr()
	updateCooldown := defaultChTimeout
	serverCfg := config.ServerConfig{
		Domains:             []string{serverAddr},
		ProxyTo:             targetAddr,
		CacheStatus:         true,
		StateUpdateCooldown: time.Minute.String(),
		CacheUpdateCooldown: updateCooldown.String(),
	}
	reqCh := newWorker(t, serverCfg)
	answerCh := make(chan proxy.McAnswer)
	req := proxy.McRequest{
		Type:       proxy.STATUS,
		ServerAddr: serverAddr,
		Ch:         answerCh,
	}
	connCh, _ := createListener(t, targetAddr)
	go func() {
		<-connCh
		conn := <-connCh
		mcConn := proxy.NewMcConn(conn)
		mcConn.ReadPacket()
		mcConn.ReadPacket()
		mcConn.WritePacket(statusPacket)
		pingPk, _ := mcConn.ReadPacket()
		mcConn.WritePacket(pingPk)
	}()
	sendRequest_TestTimeout(t, reqCh, req)
	time.Sleep(updateCooldown * 2)
	reqCh <- req

	select {
	case conn := <-connCh: //state is still in cooldown
		t.Log("worker has successfully responded")
		mcConn := proxy.NewMcConn(conn)
		mcConn.ReadPacket()
		mcConn.ReadPacket()
		mcConn.WritePacket(statusPacket)
		pingPk, _ := mcConn.ReadPacket()
		mcConn.WritePacket(pingPk)
	case <-time.After(defaultChTimeout):
		t.Fatal("timed out")
	}

	select {
	case answer := <-answerCh:
		t.Log("worker has successfully responded")
		if answer.Action() != proxy.SEND_STATUS {
			t.Errorf("expected: %v \ngot: %v", proxy.SEND_STATUS, answer.Action())
		}
	case <-time.After(defaultChTimeout):
		t.Error("timed out")
	}
}
