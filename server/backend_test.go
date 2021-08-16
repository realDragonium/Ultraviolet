package server_test

import (
	"bytes"
	"errors"
	"fmt"
	"net"
	"strings"
	"sync"
	"testing"
	"time"

	"github.com/pires/go-proxyproto"
	"github.com/realDragonium/Ultraviolet/config"
	"github.com/realDragonium/Ultraviolet/mc"
	"github.com/realDragonium/Ultraviolet/server"
)

var (
	defaultChTimeout = 25 * time.Millisecond
)

var RequestStateInfo = []struct {
	reqType         mc.HandshakeState
	denyAction      server.BackendAction
	unknownAction   server.BackendAction
	onlineAction    server.BackendAction
	offlineAction   server.BackendAction
	rateLimitAction server.BackendAction
}{
	{
		reqType:         mc.Status,
		denyAction:      server.Close,
		unknownAction:   server.SendStatus,
		onlineAction:    server.Proxy,
		offlineAction:   server.SendStatus,
		rateLimitAction: server.Close,
	},
	{
		reqType:         mc.Login,
		denyAction:      server.Disconnect,
		unknownAction:   server.Close,
		onlineAction:    server.Proxy,
		offlineAction:   server.Disconnect,
		rateLimitAction: server.Close,
	},
}

var ErrNoResponse = errors.New("there was no response from worker")

var port *int16
var portLock sync.Mutex = sync.Mutex{}

func testAddr() string {
	portLock.Lock()
	defer portLock.Unlock()
	if port == nil {
		port = new(int16)
		*port = 27000
	}
	addr := fmt.Sprintf("127.0.0.1:%d", *port)
	*port++
	return addr
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

var ErrEmptyConnCreator = errors.New("this is a test conn creator which doesnt provide connections")

type testConnCreator struct {
}

func (creator testConnCreator) Conn() func() (net.Conn, error) {
	return func() (net.Conn, error) {
		return nil, ErrEmptyConnCreator
	}
}

type testHandshakeModifier struct {
	hasBeenCalled bool
}

func (modifier *testHandshakeModifier) Modify(hs *mc.ServerBoundHandshake, addr string) {
	modifier.hasBeenCalled = true
}

type testConnectionLimiter struct {
	hasBeenCalled bool
	answer        server.BackendAnswer
	allow         bool
}

func (limiter *testConnectionLimiter) Allow(req server.BackendRequest) (server.BackendAnswer, bool) {
	limiter.hasBeenCalled = true
	return limiter.answer, limiter.allow
}

type testServerState struct {
	hasBeenCalled bool
	state         server.ServerState
}

func (state *testServerState) State() server.ServerState {
	state.hasBeenCalled = true
	return state.state
}

type testStatusCache struct {
	hasBeenCalled bool
	answer        server.BackendAnswer
	err           error
}

func (cache *testStatusCache) Status() (server.BackendAnswer, error) {
	cache.hasBeenCalled = true
	return cache.answer, cache.err
}

//Test Help methods
func setupBackendWorker(t *testing.T, serverCfg config.ServerConfig) server.BackendWorker {
	workerServerCfg, err := config.ServerToBackendConfig(serverCfg)
	if err != nil {
		t.Fatalf("error encounterd: %v", err)
	}
	serverWorker := server.NewBackendWorker(workerServerCfg)
	return serverWorker
}

func processRequest_TestTimeout(t *testing.T, worker server.BackendWorker, req server.BackendRequest) server.BackendAnswer {
	t.Helper()
	answerCh := make(chan server.BackendAnswer)
	go func() {
		answer := worker.HandleRequest(req)
		answerCh <- answer
	}()

	select {
	case answer := <-answerCh:
		t.Log("worker has successfully responded")
		return answer
	case <-time.After(defaultChTimeout):
		t.Fatal("timed out")
	}
	return server.BackendAnswer{}
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

func TestBackendWorker_OfflineServer(t *testing.T) {
	for _, tc := range RequestStateInfo {
		t.Run(fmt.Sprintf("reqType-%v", tc.reqType), func(t *testing.T) {
			serverAddr := "backend"
			disconnectMessage := "Some disconnect message right here"
			disconPacket := mc.ClientBoundDisconnect{
				Reason: mc.Chat(disconnectMessage),
			}.Marshal()
			serverCfg := config.ServerConfig{
				Domains:           []string{serverAddr},
				ProxyTo:           "1",
				OfflineStatus:     defaultOfflineStatus(),
				DisconnectMessage: disconnectMessage,
			}
			req := server.BackendRequest{
				Type: tc.reqType,
			}
			offlineStatusPk := defaultOfflineStatusPacket()
			worker := setupBackendWorker(t, serverCfg)
			answer := processRequest_TestTimeout(t, worker, req)
			if answer.Action() != tc.offlineAction {
				t.Errorf("expected: %v - got: %v", tc.offlineAction, answer.Action())
			}
			receivedPacket := answer.Response()
			if tc.reqType == mc.Status {
				if !samePk(offlineStatusPk, receivedPacket) {
					offlineStatus, _ := mc.UnmarshalClientBoundResponse(offlineStatusPk)
					receivedStatus, _ := mc.UnmarshalClientBoundResponse(receivedPacket)
					t.Errorf("expected: %v - got: %v", offlineStatus, receivedStatus)
				}
			} else if tc.reqType == mc.Login {
				if !samePk(disconPacket, receivedPacket) {
					expected, _ := mc.UnmarshalClientDisconnect(disconPacket)
					received, _ := mc.UnmarshalClientDisconnect(receivedPacket)
					t.Errorf("expected: %v - got: %v", expected, received)
				}
			}
		})
	}
}

func TestBackendWorker_OnlineServer(t *testing.T) {
	for _, tc := range RequestStateInfo {
		t.Run(fmt.Sprintf("reqType-%v", tc.reqType), func(t *testing.T) {
			serverAddr := "backend"
			targetAddr := testAddr()
			serverCfg := config.ServerConfig{
				Domains: []string{serverAddr},
				ProxyTo: targetAddr,
			}
			req := server.BackendRequest{
				Type: tc.reqType,
			}
			listener, err := net.Listen("tcp", targetAddr)
			if err != nil {
				t.Fatal(err)
			}
			go func() {
				listener.Accept()

			}()
			worker := setupBackendWorker(t, serverCfg)
			answer := processRequest_TestTimeout(t, worker, req)
			if answer.Action() != tc.onlineAction {
				t.Fatalf("expected: %v - got: %v", tc.onlineAction, answer.Action())
			}
			serverConn, _ := answer.ServerConn()
			testCloseConnection(t, serverConn)
			if answer.ProxyCh() == nil {
				t.Error("No proxy channel provided")
			}
		})
	}
}

func TestBackendWorker_HandshakeModifier(t *testing.T) {
	hsModifier := testHandshakeModifier{}
	worker := server.BackendWorker{
		HsModifier:  &hsModifier,
		ServerState: server.AlwaysOnlineState{},
		ConnCreator: testConnCreator{},
	}
	req := server.BackendRequest{
		Type: mc.Login,
		Handshake: mc.ServerBoundHandshake{
			ServerAddress: "Something",
		},
		Addr: &net.TCPAddr{
			IP:   net.ParseIP("1.1.1.1"),
			Port: 25560,
		},
	}
	answer := processRequest_TestTimeout(t, worker, req)
	if answer.Action() != server.Proxy {
		t.Fatalf("expected: %v - got: %v", server.Proxy, answer.Action())
	}

	if !hsModifier.hasBeenCalled {
		t.Error("expected handshake modifier to be called but wasnt")
	}

}

func TestBackendWorker_ProxyBind(t *testing.T) {
	for _, tc := range RequestStateInfo {
		t.Run(fmt.Sprintf("reqType-%v", tc.reqType), func(t *testing.T) {
			serverAddr := "backend"
			targetAddr := testAddr()
			proxyBind := "127.0.0.2"
			serverCfg := config.ServerConfig{
				Domains:   []string{serverAddr},
				ProxyTo:   targetAddr,
				ProxyBind: proxyBind,
			}
			req := server.BackendRequest{
				Type: tc.reqType,
			}

			go func() {
				worker := setupBackendWorker(t, serverCfg)
				answer := processRequest_TestTimeout(t, worker, req)
				answer.ServerConn() // Calling it instead of the player's goroutine
			}()

			listener, err := net.Listen("tcp", targetAddr)
			if err != nil {
				t.Fatal(err)
			}

			conn, err := listener.Accept() // State check call (proxy bind should be used here too)
			if err != nil {
				t.Fatal(err)
			}
			if netAddrToIp(conn.RemoteAddr()) != proxyBind {
				t.Errorf("expected: %v - got: %v", proxyBind, netAddrToIp(conn.RemoteAddr()))
			}

		})
	}
}

func TestBackendWorker_ProxyProtocol(t *testing.T) {
	for _, tc := range RequestStateInfo {
		t.Run(fmt.Sprintf("reqType-%v", tc.reqType), func(t *testing.T) {
			serverAddr := "backend"
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
			req := server.BackendRequest{
				Type: tc.reqType,
				Addr: playerAddr,
			}
			listener, err := net.Listen("tcp", targetAddr)
			if err != nil {
				t.Fatal(err)
			}
			proxyListener := &proxyproto.Listener{Listener: listener}
			connCh := make(chan net.Conn)
			errorCh := make(chan error)
			go func() {
				for i := 0; i < 2; i++ {
					conn, err := proxyListener.Accept()
					if err != nil {
						errorCh <- err
						return
					}
					connCh <- conn
				}
			}()

			go func() {
				worker := setupBackendWorker(t, serverCfg)
				answer := processRequest_TestTimeout(t, worker, req)
				answer.ServerConn() // Calling it instead of the player's goroutine
			}()

			<-connCh // State check call (no proxy protocol in here)
			select {
			case err := <-errorCh:
				t.Fatalf("error while accepting connection: %v", err)
			case conn := <-connCh:
				t.Log("connection has been created")
				if conn.RemoteAddr().String() != playerAddr.String() {
					t.Errorf("expected: %v - got: %v", playerAddr, conn.RemoteAddr())
				}
			case <-time.After(defaultChTimeout):
				t.Error("timed out")
			}
		})
	}
}

func TestBackendWorker_ConnLimiter(t *testing.T) {
	tt := []struct {
		allowConn         bool
		shouldAnswerMatch bool
		processAnswer     server.BackendAnswer
	}{
		{
			allowConn:         true,
			shouldAnswerMatch: false,
			processAnswer:     server.NewCloseAnswer(),
		},
		{
			allowConn:         false,
			shouldAnswerMatch: true,
			processAnswer:     server.NewCloseAnswer(),
		},
	}
	for _, tc := range tt {
		name := fmt.Sprintf("allows connection: %v", tc.allowConn)
		t.Run(name, func(t *testing.T) {
			connLimiter := testConnectionLimiter{
				answer: tc.processAnswer,
				allow:  tc.allowConn,
			}
			worker := server.BackendWorker{
				ServerState: server.AlwaysOnlineState{},
				ConnCreator: testConnCreator{},
				ConnLimiter: &connLimiter,
			}
			req := server.BackendRequest{}
			answer := processRequest_TestTimeout(t, worker, req)

			if !connLimiter.hasBeenCalled {
				t.Error("expected conn limiter to be called but it wasnt")
			}

			answerMatch := answer.Action() == tc.processAnswer.Action()
			if answerMatch != tc.shouldAnswerMatch {
				if tc.shouldAnswerMatch {
					t.Error("answer action didnt match to conn limiter answer action")
					t.Logf("received answer: %v", answer)
				} else {
					t.Error("answer action was equal to conn limiter answer action which shouldnt happen")
					t.Logf("received answer: %v", answer)
				}
			}
		})
	}
}

func TestBackendWorker_ServerState(t *testing.T) {
	tt := []struct {
		reqType        mc.HandshakeState
		serverState    server.ServerState
		expectedAction server.BackendAction
	}{
		{
			reqType:        mc.UnknownState,
			serverState:    server.Online,
			expectedAction: server.Proxy,
		},
		{
			reqType:        mc.Login,
			serverState:    server.Offline,
			expectedAction: server.Disconnect,
		},
		{
			reqType:        mc.Status,
			serverState:    server.Offline,
			expectedAction: server.SendStatus,
		},
	}
	for _, tc := range tt {
		name := fmt.Sprintf("reqType:%v - serverState:%v", tc.reqType, tc.serverState)
		t.Run(name, func(t *testing.T) {
			serverState := testServerState{
				state: tc.serverState,
			}
			worker := server.BackendWorker{
				ServerState: &serverState,
				ConnCreator: testConnCreator{},
			}
			req := server.BackendRequest{
				Type: tc.reqType,
			}
			answer := processRequest_TestTimeout(t, worker, req)

			if answer.Action() != tc.expectedAction {
				t.Errorf("expected %v but got %v instead", tc.expectedAction, answer.Action())
			}
			if !serverState.hasBeenCalled {
				t.Error("Expected serverstate to be called but wasnt")
			}
		})
	}
}

func TestBackendWorker_StatusCache(t *testing.T) {
	tt := []struct {
		reqType        mc.HandshakeState
		callsCache     bool
		errToReturn    error
		answer         server.BackendAnswer
		expectedAction server.BackendAction
	}{
		{
			reqType:        mc.Status,
			callsCache:     true,
			answer:         server.BackendAnswer{},
			expectedAction: server.Error,
		},
		{
			reqType:        mc.Login,
			callsCache:     false,
			expectedAction: server.Proxy,
		},
		{
			reqType:        mc.Status,
			callsCache:     true,
			answer:         server.NewDisconnectAnswer(mc.Packet{}),
			errToReturn:    nil,
			expectedAction: server.Disconnect,
		},
		{
			reqType:        mc.Status,
			callsCache:     true,
			answer:         server.NewDisconnectAnswer(mc.Packet{}),
			errToReturn:    errors.New("random error for testing"),
			expectedAction: server.SendStatus,
		},
	}

	for _, tc := range tt {
		name := fmt.Sprintf("reqType:%v - shouldCall:%v - returnErr:%v", tc.reqType, tc.callsCache, tc.errToReturn)
		t.Run(name, func(t *testing.T) {
			cache := testStatusCache{
				answer: tc.answer,
				err:    tc.errToReturn,
			}
			worker := server.BackendWorker{
				ServerState: server.AlwaysOnlineState{},
				ConnCreator: testConnCreator{},
				StatusCache: &cache,
			}
			req := server.BackendRequest{
				Type: tc.reqType,
			}
			answer := processRequest_TestTimeout(t, worker, req)
			if answer.Action() != tc.expectedAction {
				t.Errorf("expected %v but got %v instead", tc.expectedAction, answer.Action())
			}
			if cache.hasBeenCalled != tc.callsCache {
				t.Error("Expected cache to be called but wasnt")
			}
		})
	}
}

func TestBackendWorker_Update(t *testing.T) {
	t.Run("change when update config contains new value", func(t *testing.T) {
		worker := server.BackendWorker{}
		cfg := server.BackendConfig{
			Name:                "UV",
			UpdateProxyProtocol: true,
			SendProxyProtocol:   true,
			DisconnectPacket:    mc.Packet{ID: 0x45, Data: []byte{0x11, 0x12, 0x13}},
			OfflineStatusPacket: mc.Packet{ID: 0x45, Data: []byte{0x11, 0x12, 0x14}},
			HsModifier:          &testHandshakeModifier{},
			ConnCreator:         &stateConnCreator{},
			ConnLimiter:         &testConnectionLimiter{},
			ServerState:         &testServerState{},
			StatusCache:         &testStatusCache{},
		}
		worker.UpdateSameGoroutine(cfg)
		if worker.Name != cfg.Name {
			t.Errorf("expected: %v - got: %v", cfg.Name, worker.Name)
		}
		if worker.SendProxyProtocol != cfg.SendProxyProtocol {
			t.Errorf("expected: %v - got: %v", cfg.SendProxyProtocol, worker.SendProxyProtocol)
		}
		if !samePk(worker.DisconnectPacket, cfg.DisconnectPacket) {
			t.Errorf("expected: %v - got: %v", cfg.DisconnectPacket, worker.DisconnectPacket)
		}
		if !samePk(worker.OfflineStatusPacket, cfg.OfflineStatusPacket) {
			t.Errorf("expected: %v - got: %v", cfg.OfflineStatusPacket, worker.OfflineStatusPacket)
		}
		if worker.HsModifier != cfg.HsModifier {
			t.Errorf("expected: %v - got: %v", cfg.HsModifier, worker.HsModifier)
		}
		if worker.ConnCreator != cfg.ConnCreator {
			t.Errorf("expected: %v - got: %v", cfg.ConnCreator, worker.ConnCreator)
		}
		if worker.ConnLimiter != cfg.ConnLimiter {
			t.Errorf("expected: %v - got: %v", cfg.ConnLimiter, worker.ConnLimiter)
		}
		if worker.StatusCache != cfg.StatusCache {
			t.Errorf("expected: %v - got: %v", cfg.StatusCache, worker.StatusCache)
		}
		if worker.ServerState != cfg.ServerState {
			t.Errorf("expected: %v - got: %v", cfg.ServerState, worker.ServerState)
		}
	})

	t.Run("change when update config contains new value", func(t *testing.T) {
		worker := server.BackendWorker{
			Name:                "UV",
			SendProxyProtocol:   true,
			DisconnectPacket:    mc.Packet{ID: 0x45, Data: []byte{0x11, 0x12, 0x13}},
			OfflineStatusPacket: mc.Packet{ID: 0x45, Data: []byte{0x11, 0x12, 0x14}},
			HsModifier:          &testHandshakeModifier{},
			ConnCreator:         &stateConnCreator{},
			ConnLimiter:         &testConnectionLimiter{},
			ServerState:         &testServerState{},
			StatusCache:         &testStatusCache{},
		}
		cfg := server.BackendConfig{}
		worker.UpdateSameGoroutine(cfg)
		if worker.Name == cfg.Name {
			t.Errorf("didnt expect: %v", worker.Name)
		}
		if worker.SendProxyProtocol == cfg.SendProxyProtocol {
			t.Errorf("didnt expect: %v", worker.SendProxyProtocol)
		}
		if samePk(worker.DisconnectPacket, cfg.DisconnectPacket) {
			t.Errorf("didnt expect: %v", worker.DisconnectPacket)
		}
		if samePk(worker.OfflineStatusPacket, cfg.OfflineStatusPacket) {
			t.Errorf("didnt expect: %v", worker.OfflineStatusPacket)
		}
		if worker.HsModifier == cfg.HsModifier {
			t.Errorf("didnt expect: %v", worker.HsModifier)
		}
		if worker.ConnCreator == cfg.ConnCreator {
			t.Errorf("didnt expect: %v", worker.ConnCreator)
		}
		if worker.ConnLimiter == cfg.ConnLimiter {
			t.Errorf("didnt expect: %v", worker.ConnLimiter)
		}
		if worker.StatusCache == cfg.StatusCache {
			t.Errorf("didnt expect: %v", worker.StatusCache)
		}
		if worker.ServerState == cfg.ServerState {
			t.Errorf("didnt expect: %v", worker.ServerState)
		}
	})

	t.Run("can update while running", func(t *testing.T) {
		worker := server.NewEmptyBackendWorker()
		worker.ServerState = server.AlwaysOfflineState{}
		reqCh := worker.ReqCh()
		worker.Run()
		testPk := mc.ClientBoundDisconnect{
			Reason: mc.String("some text here"),
		}.Marshal()
		cfg := server.BackendConfig{
			DisconnectPacket: testPk,
		}
		err := worker.Update(cfg)
		if err != nil {
			t.Fatalf("got error: %v", err)
		}
		ansCh := make(chan server.BackendAnswer)
		reqCh <- server.BackendRequest{
			Type: mc.Login,
			Ch:   ansCh,
		}
		ans := <-ansCh

		if !samePk(testPk, ans.Response()) {
			t.Errorf("expected: %v - got: %v", testPk, ans.Response())
		}
		worker.Close()
	})
}

func TestBackendFactory(t *testing.T) {
	t.Run("lets the worker run", func(t *testing.T) {
		cfg := config.BackendWorkerConfig{}
		b := server.BackendFactory(cfg)
		reqCh := b.ReqCh()
		req := server.BackendRequest{}
		select {
		case reqCh <- req:
			t.Log("backend is running in different goroutine")
		case <-time.After(defaultChTimeout):
			t.Error("backend isnt running in different goroutine...?")
		}
	})

	t.Run("doesnt share data between goroutines", func(t *testing.T) {
		msg := "some text here"
		disconPk := mc.ClientBoundDisconnect{
			Reason: mc.String(msg),
		}.Marshal()

		cfg := config.BackendWorkerConfig{
			DisconnectPacket: disconPk,
			StateOption:      config.ALWAYS_OFFLINE,
		}

		b := server.BackendFactory(cfg)
		worker, ok := b.(*server.BackendWorker)
		if !ok {
			t.Fatalf("backend is different type then expected")
		}
		newCfg := server.BackendConfig{
			DisconnectPacket: mc.Packet{ID: 0x44, Data: []byte{0, 1, 2, 3, 4, 5, 6, 7, 8, 9}},
		}
		worker.UpdateSameGoroutine(newCfg)

		reqCh := b.ReqCh()
		rCh := make(chan server.BackendAnswer)
		req := server.BackendRequest{
			Ch:   rCh,
			Type: mc.Login,
		}
		reqCh <- req
		ans := <-rCh

		if !samePK(disconPk, ans.Response()) {
			t.Error("packets werent the same which might mean its sharing memory")
			t.Logf("expected: %v", disconPk)
			t.Logf("got: %v", ans.Response())
		}
	})

}
