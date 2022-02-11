package worker_test

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
	ultraviolet "github.com/realDragonium/Ultraviolet"
	"github.com/realDragonium/Ultraviolet/config"
	"github.com/realDragonium/Ultraviolet/mc"
	"github.com/realDragonium/Ultraviolet/module"
	"github.com/realDragonium/Ultraviolet/worker"
)

var (
	defaultChTimeout = 25 * time.Millisecond
)

var RequestStateInfo = []struct {
	reqType         mc.HandshakeState
	denyAction      worker.BackendAction
	unknownAction   worker.BackendAction
	onlineAction    worker.BackendAction
	offlineAction   worker.BackendAction
	rateLimitAction worker.BackendAction
}{
	{
		reqType:         mc.Status,
		denyAction:      worker.Close,
		unknownAction:   worker.SendStatus,
		onlineAction:    worker.Proxy,
		offlineAction:   worker.SendStatus,
		rateLimitAction: worker.Close,
	},
	{
		reqType:         mc.Login,
		denyAction:      worker.Disconnect,
		unknownAction:   worker.Close,
		onlineAction:    worker.Proxy,
		offlineAction:   worker.Disconnect,
		rateLimitAction: worker.Close,
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
	allow         bool
}

func (limiter *testConnectionLimiter) Allow(reqData ultraviolet.RequestData) (allowed bool, err error) {
	limiter.hasBeenCalled = true
	return limiter.allow, nil
}

type testServerState struct {
	hasBeenCalled bool
	state         ultraviolet.ServerState
}

func (state *testServerState) State() ultraviolet.ServerState {
	state.hasBeenCalled = true
	return state.state
}

type testStatusCache struct {
	hasBeenCalled bool
	answer        mc.Packet
	err           error
}

func (cache *testStatusCache) Status() (mc.Packet, error) {
	cache.hasBeenCalled = true
	return cache.answer, cache.err
}

type stateConnCreator struct {
	callAmount  int
	returnError bool
}

func (creator *stateConnCreator) Conn() func() (net.Conn, error) {
	creator.callAmount++
	if creator.returnError {
		return func() (net.Conn, error) {
			return nil, ErrEmptyConnCreator
		}
	}
	return func() (net.Conn, error) {
		return &net.TCPConn{}, nil
	}
}

//Test Help methods
func setupBackendWorker(t *testing.T, serverCfg config.ServerConfig) worker.BackendWorker {
	workerServerCfg, err := config.ServerToBackendConfig(serverCfg)
	if err != nil {
		t.Fatalf("error encounterd: %v", err)
	}
	serverwrk := worker.NewBackendWorker(workerServerCfg)
	return serverwrk
}

func processRequest_TestTimeout(t *testing.T, wrk worker.BackendWorker, req worker.BackendRequest) worker.BackendAnswer {
	t.Helper()
	answerCh := make(chan worker.BackendAnswer)
	go func() {
		answer := wrk.HandleRequest(req)
		answerCh <- answer
	}()

	select {
	case answer := <-answerCh:
		t.Log("worker has successfully responded")
		return answer
	case <-time.After(defaultChTimeout):
		t.Fatal("timed out")
	}
	return worker.BackendAnswer{}
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

			req := worker.BackendRequest{
				ultraviolet.RequestData{
					Type: tc.reqType,
				},
				make(chan<- worker.BackendAnswer),
			}
			offlineStatusPk := defaultOfflineStatusPacket()
			wrk := setupBackendWorker(t, serverCfg)
			answer := processRequest_TestTimeout(t, wrk, req)
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
			req := worker.BackendRequest{
				ReqData: ultraviolet.RequestData{
					Type: tc.reqType,
				},
			}
			listener, err := net.Listen("tcp", targetAddr)
			if err != nil {
				t.Fatal(err)
			}
			go func() {
				listener.Accept()

			}()
			wrk := setupBackendWorker(t, serverCfg)
			answer := processRequest_TestTimeout(t, wrk, req)
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
	wrk := worker.BackendWorker{
		HsModifier:  &hsModifier,
		ServerState: module.AlwaysOnlineState{},
		ConnCreator: testConnCreator{},
	}
	req := worker.BackendRequest{
		ReqData: ultraviolet.RequestData{
			Type: mc.Login,
			Handshake: mc.ServerBoundHandshake{
				ServerAddress: "Something",
			},
			Addr: &net.TCPAddr{
				IP:   net.ParseIP("1.1.1.1"),
				Port: 25560,
			},
		},
	}
	answer := processRequest_TestTimeout(t, wrk, req)
	if answer.Action() != worker.Proxy {
		t.Fatalf("expected: %v - got: %v", worker.Proxy, answer.Action())
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
			req := worker.BackendRequest{
				ReqData: ultraviolet.RequestData{
					Type: tc.reqType,
				},
			}

			go func() {
				wrk := setupBackendWorker(t, serverCfg)
				answer := processRequest_TestTimeout(t, wrk, req)
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
			req := worker.BackendRequest{
				ReqData: ultraviolet.RequestData{
					Type: tc.reqType,
					Addr: playerAddr,
				},
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
				wrk := setupBackendWorker(t, serverCfg)
				answer := processRequest_TestTimeout(t, wrk, req)
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
		processAnswer     worker.BackendAnswer
	}{
		{
			allowConn:         true,
			shouldAnswerMatch: false,
			processAnswer:     worker.NewCloseAnswer(),
		},
		{
			allowConn:         false,
			shouldAnswerMatch: true,
			processAnswer:     worker.NewCloseAnswer(),
		},
	}
	for _, tc := range tt {
		name := fmt.Sprintf("allows connection: %v", tc.allowConn)
		t.Run(name, func(t *testing.T) {
			connLimiter := testConnectionLimiter{
				allow: tc.allowConn,
			}
			wrk := worker.BackendWorker{
				ServerState: module.AlwaysOnlineState{},
				ConnCreator: testConnCreator{},
				ConnLimiter: &connLimiter,
			}
			req := worker.BackendRequest{}
			processRequest_TestTimeout(t, wrk, req)

			if !connLimiter.hasBeenCalled {
				t.Error("expected conn limiter to be called but it wasnt")
			}

		})
	}
}

func TestBackendWorker_ServerState(t *testing.T) {
	tt := []struct {
		reqType        mc.HandshakeState
		serverState    ultraviolet.ServerState
		expectedAction worker.BackendAction
	}{
		{
			reqType:        mc.UnknownState,
			serverState:    ultraviolet.Online,
			expectedAction: worker.Proxy,
		},
		{
			reqType:        mc.Login,
			serverState:    ultraviolet.Offline,
			expectedAction: worker.Disconnect,
		},
		{
			reqType:        mc.Status,
			serverState:    ultraviolet.Offline,
			expectedAction: worker.SendStatus,
		},
	}
	for _, tc := range tt {
		name := fmt.Sprintf("reqType:%v - serverState:%v", tc.reqType, tc.serverState)
		t.Run(name, func(t *testing.T) {
			serverState := testServerState{
				state: tc.serverState,
			}
			wrk := worker.BackendWorker{
				ServerState: &serverState,
				ConnCreator: testConnCreator{},
			}
			req := worker.BackendRequest{
				ReqData: ultraviolet.RequestData{
					Type: tc.reqType,
				},
			}
			answer := processRequest_TestTimeout(t, wrk, req)

			if answer.Action() != tc.expectedAction {
				t.Errorf("expected %v but got %v instead", tc.expectedAction, answer.Action())
			}
			if !serverState.hasBeenCalled {
				t.Error("Expected serverstate to be called but wasnt")
			}
		})
	}
}

func TestBackendWorker_Update(t *testing.T) {
	t.Run("change when update config contains new value", func(t *testing.T) {
		wrk := worker.BackendWorker{}
		cfg := worker.BackendConfig{
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
		wrk.UpdateSameGoroutine(cfg)
		if wrk.Name != cfg.Name {
			t.Errorf("expected: %v - got: %v", cfg.Name, wrk.Name)
		}
		if wrk.SendProxyProtocol != cfg.SendProxyProtocol {
			t.Errorf("expected: %v - got: %v", cfg.SendProxyProtocol, wrk.SendProxyProtocol)
		}
		if !samePk(wrk.DisconnectPacket, cfg.DisconnectPacket) {
			t.Errorf("expected: %v - got: %v", cfg.DisconnectPacket, wrk.DisconnectPacket)
		}
		if !samePk(wrk.OfflineStatusPacket, cfg.OfflineStatusPacket) {
			t.Errorf("expected: %v - got: %v", cfg.OfflineStatusPacket, wrk.OfflineStatusPacket)
		}
		if wrk.HsModifier != cfg.HsModifier {
			t.Errorf("expected: %v - got: %v", cfg.HsModifier, wrk.HsModifier)
		}
		if wrk.ConnCreator != cfg.ConnCreator {
			t.Errorf("expected: %v - got: %v", cfg.ConnCreator, wrk.ConnCreator)
		}
		if wrk.ConnLimiter != cfg.ConnLimiter {
			t.Errorf("expected: %v - got: %v", cfg.ConnLimiter, wrk.ConnLimiter)
		}
		if wrk.StatusCache != cfg.StatusCache {
			t.Errorf("expected: %v - got: %v", cfg.StatusCache, wrk.StatusCache)
		}
		if wrk.ServerState != cfg.ServerState {
			t.Errorf("expected: %v - got: %v", cfg.ServerState, wrk.ServerState)
		}
	})

	t.Run("change when update config contains new value", func(t *testing.T) {
		wrk := worker.BackendWorker{
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
		cfg := worker.BackendConfig{}
		wrk.UpdateSameGoroutine(cfg)
		if wrk.Name == cfg.Name {
			t.Errorf("didnt expect: %v", wrk.Name)
		}
		if wrk.SendProxyProtocol == cfg.SendProxyProtocol {
			t.Errorf("didnt expect: %v", wrk.SendProxyProtocol)
		}
		if samePk(wrk.DisconnectPacket, cfg.DisconnectPacket) {
			t.Errorf("didnt expect: %v", wrk.DisconnectPacket)
		}
		if samePk(wrk.OfflineStatusPacket, cfg.OfflineStatusPacket) {
			t.Errorf("didnt expect: %v", wrk.OfflineStatusPacket)
		}
		if wrk.HsModifier == cfg.HsModifier {
			t.Errorf("didnt expect: %v", wrk.HsModifier)
		}
		if wrk.ConnCreator == cfg.ConnCreator {
			t.Errorf("didnt expect: %v", wrk.ConnCreator)
		}
		if wrk.ConnLimiter == cfg.ConnLimiter {
			t.Errorf("didnt expect: %v", wrk.ConnLimiter)
		}
		if wrk.StatusCache == cfg.StatusCache {
			t.Errorf("didnt expect: %v", wrk.StatusCache)
		}
		if wrk.ServerState == cfg.ServerState {
			t.Errorf("didnt expect: %v", wrk.ServerState)
		}
	})

	t.Run("can update while running", func(t *testing.T) {
		wrk := worker.NewEmptyBackendWorker()
		wrk.ServerState = module.AlwaysOfflineState{}
		reqCh := wrk.ReqCh()
		wrk.Run()
		testPk := mc.ClientBoundDisconnect{
			Reason: mc.String("some text here"),
		}.Marshal()
		cfg := worker.BackendConfig{
			DisconnectPacket: testPk,
		}
		err := wrk.Update(cfg)
		if err != nil {
			t.Fatalf("got error: %v", err)
		}
		ansCh := make(chan worker.BackendAnswer)
		reqCh <- worker.BackendRequest{
			ReqData: ultraviolet.RequestData{
				Type: mc.Login,
			},
			Ch: ansCh,
		}
		ans := <-ansCh

		if !samePk(testPk, ans.Response()) {
			t.Errorf("expected: %v - got: %v", testPk, ans.Response())
		}
		wrk.Close()
	})
}

func TestBackendFactory(t *testing.T) {
	t.Run("lets the worker run", func(t *testing.T) {
		cfg := config.BackendWorkerConfig{}
		b := worker.BackendFactory(cfg)
		reqCh := b.ReqCh()
		req := worker.BackendRequest{}
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

		b := worker.BackendFactory(cfg)
		wrk, ok := b.(*worker.BackendWorker)
		if !ok {
			t.Fatalf("backend is different type then expected")
		}
		newCfg := worker.BackendConfig{
			DisconnectPacket: mc.Packet{ID: 0x44, Data: []byte{0, 1, 2, 3, 4, 5, 6, 7, 8, 9}},
		}
		wrk.UpdateSameGoroutine(newCfg)

		reqCh := b.ReqCh()
		rCh := make(chan worker.BackendAnswer)
		req := worker.BackendRequest{
			ReqData: ultraviolet.RequestData{
				Type: mc.Login,
			},
			Ch: rCh,
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
