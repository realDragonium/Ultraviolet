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

func unknownServerStatus() mc.AnotherStatusResponse {
	return mc.AnotherStatusResponse{
		Name:        "Ultraviolet",
		Protocol:    0,
		Description: "No server found",
	}
}

func defaultOfflineStatusPacket() mc.Packet {
	return defaultOfflineStatus().Marshal()
}

func defaultOfflineStatus() mc.AnotherStatusResponse {
	return mc.AnotherStatusResponse{
		Name:        "Ultraviolet-ff",
		Protocol:    755,
		Description: "offline proxy being tested",
	}
}

//Test Help methods
func setupTestWorkers(cfg config.UltravioletConfig, serverCfgs ...config.ServerConfig) chan<- proxy.McRequest {
	reqCh := make(chan proxy.McRequest)
	proxyCh := make(chan proxy.ProxyAction)
	proxy.SetupWorkers(cfg, serverCfgs, reqCh, proxyCh)
	go func() {
		for {
			<-proxyCh
		}
	}()
	return reqCh
}

func setupPublicTestWorkers(cfg config.UltravioletConfig, serverCfgs ...config.ServerConfig) chan<- proxy.McRequest {
	reqCh := make(chan proxy.McRequest)
	proxy.SetupNewWorkers(cfg, serverCfgs, reqCh)
	return reqCh
}

func simpleUltravioletConfig(logOutput io.Writer) config.UltravioletConfig {
	log.SetPrefix("log-text: ")
	log.SetFlags(0)
	return config.UltravioletConfig{
		DefaultStatus:         unknownServerStatus(),
		NumberOfWorkers:       1,
		NumberOfConnWorkers:   1,
		NumberOfStatusWorkers: 1,
		LogOutput:             logOutput,
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
	return proxy.McAnswer{}
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
func TestBasicWorkers(t *testing.T) {
	setupWorker := func(tLocal *testing.T, serverCfgs ...config.ServerConfig) chan<- proxy.McRequest {
		logOutput := &testLogger{t: tLocal}
		reqCh := setupTestWorkers(simpleUltravioletConfig(logOutput), serverCfgs...)
		return reqCh
	}
	runAllWorkerTests(t, setupWorker)
}

func TestPublicAndPrivateWorkers(t *testing.T) {
	setupWorker := func(tLocal *testing.T, serverCfgs ...config.ServerConfig) chan<- proxy.McRequest {
		logOutput := &testLogger{t: tLocal}
		reqCh := setupPublicTestWorkers(simpleUltravioletConfig(logOutput), serverCfgs...)
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
	t.Run("proxy bind", func(t *testing.T) {
		testProxyBind(t, newWorker)
	})
	t.Run("proxy protocol", func(t *testing.T) {
		testProxyProtocol(t, newWorker)
	})
	t.Run("many request will rate limit connection to backend", func(t *testing.T) {
		testProxy_ManyRequestsWillRateLimit(t, newWorker)
	})
	t.Run("will allow new connections after ratelimit cooldown", func(t *testing.T) {
		testProxy_WillAllowNewConn_AfterDurationEnded(t, newWorker)
	})
	t.Run("server state - doesnt call backend again before cooldown is over", func(t *testing.T) {
		testServerState_DoesntCallBeforeCooldownIsOver(t, newWorker)
	})
	t.Run("server state - call backend again when cooldown is over", func(t *testing.T) {
		testServerState_ShouldCallAgainOutOfCooldown(t, newWorker)
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
			if answer.Action != tc.unknownAction {
				t.Errorf("expected: %v \ngot: %v", tc.unknownAction, answer.Action)
			}
			if tc.reqType == proxy.STATUS {
				defaultStatusPk := unknownServerStatusPk()
				if !samePk(defaultStatusPk, answer.StatusPk) {
					defaultStatus, _ := mc.UnmarshalClientBoundResponse(defaultStatusPk)
					receivedStatus, _ := mc.UnmarshalClientBoundResponse(answer.StatusPk)
					t.Errorf("expected: %v \ngot: %v", defaultStatus, receivedStatus)
				}
			}
		})
	}
}

// Add test for when offline status isnt configured...?
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
			if answer.Action != tc.offlineAction {
				t.Errorf("expected: %v \ngot: %v", tc.offlineAction, answer.Action)
			}
			if tc.reqType == proxy.STATUS {
				if !samePk(offlineStatusPk, answer.StatusPk) {
					offlineStatus, _ := mc.UnmarshalClientBoundResponse(offlineStatusPk)
					receivedStatus, _ := mc.UnmarshalClientBoundResponse(answer.StatusPk)
					t.Errorf("expected: %v \ngot: %v", offlineStatus, receivedStatus)
				}
			} else if tc.reqType == proxy.LOGIN {
				if !samePk(disconPacket, answer.DisconMessage) {
					expected, _ := mc.UnmarshalClientDisconnect(disconPacket)
					received, _ := mc.UnmarshalClientDisconnect(answer.DisconMessage)
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
			if answer.Action != tc.onlineAction {
				t.Fatalf("expected: %v \ngot: %v", tc.onlineAction, answer.Action)
			}
			serverConn, _ := answer.ServerConnFunc()
			testCloseConnection(t, serverConn)
			if answer.ProxyCh == nil {
				t.Error("No proxy channel provided")
			}
		})
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
				answer.ServerConnFunc() // Calling it instead of the player's goroutine
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
				answer.ServerConnFunc() // Calling it instead of the player's goroutine
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
				Domains:      []string{serverAddr},
				ProxyTo:      targetAddr,
				RateLimit:    rateLimit,
				RateDuration: rateLimitDuration.String(),
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
			if answer.Action != tc.rateLimitAction {
				t.Fatalf("expected: %v \ngot: %v", tc.rateLimitAction, answer.Action)
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
				Domains:      []string{serverAddr},
				ProxyTo:      targetAddr,
				RateLimit:    rateLimit,
				RateDuration: rateLimitDuration.String(),
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
			if answer.Action != tc.onlineAction {
				t.Fatalf("expected: %v \ngot: %v", tc.onlineAction, answer.Action)
			}
		})
	}
}

func testServerState_DoesntCallBeforeCooldownIsOver(t *testing.T, newWorker createWorker) {
	for _, tc := range LoginStatusTestCases {
		t.Run(fmt.Sprintf("reqType-%v", tc.reqType), func(t *testing.T) {
			serverAddr := "ultraviolet"
			targetAddr := testAddr()
			updateCooldown := time.Minute
			serverCfg := config.ServerConfig{
				Domains:        []string{serverAddr},
				ProxyTo:        targetAddr,
				UpdateCooldown: updateCooldown.String(),
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
				Domains:        []string{serverAddr},
				ProxyTo:        targetAddr,
				UpdateCooldown: updateCooldown.String(),
				DialTimeout:    "1s",
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

func TestStatusWorker_ShareServerData(t *testing.T) {
	targetAddr := testAddr()
	cooldown := time.Minute
	servers := make(map[int]proxy.WorkerServerConfig)
	servers[0] = proxy.WorkerServerConfig{
		ProxyTo:             targetAddr,
		State:               proxy.UNKNOWN,
		StateUpdateCooldown: cooldown,
	}
	connCh := make(chan proxy.ConnRequest)
	statusCh := make(chan proxy.StatusRequest)
	proxy.RunConnWorkers(1, connCh, statusCh, servers)
	proxy.RunStatusWorkers(2, statusCh, connCh, servers)

	answerCh := make(chan proxy.StatusAnswer)
	statusCh <- proxy.StatusRequest{
		ServerId: 0,
		Type:     proxy.STATE_REQUEST,
		AnswerCh: answerCh,
	}
	time.Sleep(longerChTimeout)
	answerCh2 := make(chan proxy.StatusAnswer)
	statusCh <- proxy.StatusRequest{
		ServerId: 0,
		Type:     proxy.STATE_REQUEST,
		AnswerCh: answerCh2,
	}

	select {
	case answer := <-answerCh2:
		t.Log("worker has successfully responded")
		if answer.State != proxy.OFFLINE {
			t.Errorf("expected: %v got: %v", proxy.OFFLINE, answer)
		}
	case <-time.After(defaultChTimeout):
		t.Error("timed out")
	}
}
