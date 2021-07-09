package proxy_test

import (
	"fmt"
	"log"
	"net"
	"testing"
	"time"

	"github.com/pires/go-proxyproto"
	"github.com/realDragonium/Ultraviolet/config"
	"github.com/realDragonium/Ultraviolet/mc"
	"github.com/realDragonium/Ultraviolet/proxy"
)

var LoginStatusTestCases = []struct {
	reqType proxy.McRequestType
}{
	{
		reqType: proxy.STATUS,
	},
	{
		reqType: proxy.LOGIN,
	},
}

func ttTemplate(t *testing.T) {
	for _, tc := range LoginStatusTestCases {
		t.Run(fmt.Sprintf("reqType: %v", tc.reqType), func(t *testing.T) {
			serverCfg := config.ServerConfig{}
			req := proxy.McRequest{
				Type:       tc.reqType,
				ServerAddr: "some weird server address",
			}
			answer := genericTestMethod_Server(t, serverCfg, req)
			if answer.Action != proxy.CLOSE {
				t.Errorf("expcted: %v \ngot: %v", proxy.CLOSE, answer.Action)
			}
		})
	}
}

type testLogger struct {
	t *testing.T
}

func (tLog testLogger) Write(b []byte) (int, error) {
	tLog.t.Logf(string(b))
	return 0, nil
}

func setTestLogger(t *testing.T) {
	logger := testLogger{t: t}
	log.SetOutput(logger)
}

//Test Help methods
func setupBasicTestWorkers(serverCfgs ...config.ServerConfig) chan<- proxy.McRequest {
	reqCh, proxyCh := setupTestWorkers(simpleUltravioletConfig(), serverCfgs...)
	go func() {
		for {
			<-proxyCh
		}
	}()
	return reqCh
}

func setupTestWorkers(cfg config.UltravioletConfig, serverCfgs ...config.ServerConfig) (chan<- proxy.McRequest, <-chan proxy.ProxyAction) {
	reqCh := make(chan proxy.McRequest)
	proxyCh := make(chan proxy.ProxyAction)
	proxy.SetupWorkers(cfg, serverCfgs, reqCh, proxyCh)
	return reqCh, proxyCh
}

func simpleUltravioletConfig() config.UltravioletConfig {
	return config.UltravioletConfig{
		DefaultStatus:         unknownServerStatus(),
		NumberOfWorkers:       1,
		NumberOfConnWorkers:   1,
		NumberOfStatusWorkers: 1,
	}
}

func genericTestMethod_Server(t *testing.T, server config.ServerConfig, req proxy.McRequest) proxy.McAnswer {
	t.Helper()
	setTestLogger(t)
	reqCh := setupBasicTestWorkers(server)

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

func genericTestMethod_CallToServer(t *testing.T, server config.ServerConfig, req proxy.McRequest) {
	t.Helper()
	reqCh := setupBasicTestWorkers(server)
	answerCh := make(chan proxy.McAnswer)
	req.Ch = answerCh
	reqCh <- req
	<-answerCh
}

func testCloseConnection(t *testing.T, conn proxy.McConn) {
	emptyPk := mc.Packet{Data: []byte{0}}
	if err := conn.WritePacket(emptyPk); err != nil {
		t.Errorf("Got an unexpected error: %v", err)
	}
}

// Actual Tests
func TestWorker_CanReceiveRequests(t *testing.T) {
	serverCfg := config.ServerConfig{}
	reqCh := setupBasicTestWorkers(serverCfg)
	select {
	case reqCh <- proxy.McRequest{}:
		t.Log("worker has successfully received request")
	case <-time.After(defaultChTimeout):
		t.Error("timed out")
	}
}

func TestStatus_UnknownAddr_ReturnDefaultStatus(t *testing.T) {
	serverCfg := config.ServerConfig{}
	req := proxy.McRequest{
		Type:       proxy.STATUS,
		ServerAddr: "some weird server address",
	}
	answer := genericTestMethod_Server(t, serverCfg, req)
	defaultStatusPk := unknownServerStatusPk()
	if !samePk(defaultStatusPk, answer.StatusPk) {
		defaultStatus, _ := mc.UnmarshalClientBoundResponse(defaultStatusPk)
		receivedStatus, _ := mc.UnmarshalClientBoundResponse(answer.StatusPk)
		t.Errorf("expcted: %v \ngot: %v", defaultStatus, receivedStatus)
	}
	if answer.Action != proxy.SEND_STATUS {
		t.Errorf("expcted: %v \ngot: %v", proxy.SEND_STATUS, answer.Action)
	}
}

func TestStatus_KnownAddr_OfflineServer_ReturnsOfflineStatus(t *testing.T) {
	serverAddr := "ultraviolet"
	serverCfg := config.ServerConfig{
		MainDomain:    serverAddr,
		OfflineStatus: defaultOfflineStatus(),
	}
	req := proxy.McRequest{
		Type:       proxy.STATUS,
		ServerAddr: serverAddr,
	}
	offlineStatusPk := defaultOfflineStatusPacket()
	answer := genericTestMethod_Server(t, serverCfg, req)
	if !samePk(offlineStatusPk, answer.StatusPk) {
		offlineStatus, _ := mc.UnmarshalClientBoundResponse(offlineStatusPk)
		receivedStatus, _ := mc.UnmarshalClientBoundResponse(answer.StatusPk)
		t.Errorf("expcted: %v \ngot: %v", offlineStatus, receivedStatus)
	}
	if answer.Action != proxy.SEND_STATUS {
		t.Errorf("expcted: %v \ngot: %v", proxy.SEND_STATUS, answer.Action)
	}
}

func TestLogin_UnknownAddr_ShouldClose(t *testing.T) {
	serverCfg := config.ServerConfig{}
	req := proxy.McRequest{
		Type:       proxy.LOGIN,
		ServerAddr: "some weird server address",
	}
	answer := genericTestMethod_Server(t, serverCfg, req)
	if answer.Action != proxy.CLOSE {
		t.Errorf("expcted: %v \ngot: %v", proxy.CLOSE, answer.Action)
	}
}

func Test_KnownAddr_OnlineServer_ShouldProxy(t *testing.T) {
	for _, tc := range LoginStatusTestCases {
		t.Run(fmt.Sprintf("reqType: %v", tc.reqType), func(t *testing.T) {
			serverAddr := "ultraviolet"
			targetAddr := testAddr()
			serverCfg := config.ServerConfig{
				MainDomain: serverAddr,
				ProxyTo:    targetAddr,
			}
			req := proxy.McRequest{
				Type:       tc.reqType,
				ServerAddr: serverAddr,
			}
			createListener(t, targetAddr)
			answer := genericTestMethod_Server(t, serverCfg, req)
			if answer.Action != proxy.PROXY {
				t.Fatalf("expcted: %v \ngot: %v", proxy.CLOSE, answer.Action)
			}
			testCloseConnection(t, answer.ServerConn)
			if answer.ProxyCh == nil {
				t.Error("No proxy channel provided")
			}
		})
	}
}

func TestProxyBind(t *testing.T) {
	setTestLogger(t)
	for _, tc := range LoginStatusTestCases {
		t.Run(fmt.Sprintf("reqType: %v", tc.reqType), func(t *testing.T) {
			serverAddr := "ultraviolet"
			targetAddr := testAddr()
			proxyBind := "127.0.0.2"
			serverCfg := config.ServerConfig{
				MainDomain: serverAddr,
				ProxyTo:    targetAddr,
				ProxyBind:  proxyBind,
			}
			req := proxy.McRequest{
				Type:       tc.reqType,
				ServerAddr: serverAddr,
			}
			connCh, errorCh := createListener(t, targetAddr)
			go genericTestMethod_CallToServer(t, serverCfg, req)
			conn := <-connCh // State check call (proxy bind should be used here too)
			receivedBind := netAddrToIp(conn.RemoteAddr())
			if receivedBind != proxyBind {
				t.Errorf("expcted: %v \ngot: %v", proxyBind, receivedBind)
			}
			select {
			case err := <-errorCh:
				t.Fatalf("error while accepting connection: %v", err)
			case conn := <-connCh:
				t.Log("connection has been created")
				receivedBind := netAddrToIp(conn.RemoteAddr())
				if receivedBind != proxyBind {
					t.Errorf("expcted: %v \ngot: %v", proxyBind, receivedBind)
				}
			case <-time.After(defaultChTimeout):
				t.Error("timed out")
			}
		})
	}
}

func TestProxyProtocol(t *testing.T) {
	setTestLogger(t)
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
				MainDomain:        serverAddr,
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

			go genericTestMethod_CallToServer(t, serverCfg, req)
			<-connCh // State check call (no proxy protocol in here)
			select {
			case err := <-errorCh:
				t.Fatalf("error while accepting connection: %v", err)
			case conn := <-connCh:
				t.Log("connection has been created")
				if conn.RemoteAddr().String() != playerAddr.String() {
					t.Errorf("expcted: %v \ngot: %v", playerAddr, conn.RemoteAddr())
				}
			case <-time.After(defaultChTimeout):
				t.Error("timed out")
			}
		})
	}
}

func TestLogin_KnownAddr_OfflineServer_ShouldDisconnect(t *testing.T) {
	disconnectMessage := "Some disconnect message right here"
	disconPacket := mc.ClientBoundDisconnect{
		Reason: mc.Chat(disconnectMessage),
	}.Marshal()
	serverAddr := "ultraviolet"
	serverCfg := config.ServerConfig{
		MainDomain:        serverAddr,
		DisconnectMessage: disconnectMessage,
	}
	req := proxy.McRequest{
		Type:       proxy.LOGIN,
		ServerAddr: serverAddr,
	}
	answer := genericTestMethod_Server(t, serverCfg, req)
	if answer.Action != proxy.DISCONNECT {
		t.Errorf("expcted: %v got: %v", proxy.DISCONNECT, answer.Action)
	}
	if !samePk(disconPacket, answer.DisconMessage) {
		t.Errorf("expcted: %v \ngot: %v", disconPacket, answer.DisconMessage)
	}
}

