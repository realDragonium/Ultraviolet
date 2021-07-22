package server_test

import (
	"bufio"
	"bytes"
	"crypto/ecdsa"
	"crypto/elliptic"
	"crypto/rand"
	"crypto/x509"
	"errors"
	"fmt"
	"io/ioutil"
	"net"
	"os"
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
	defaultChTimeout = 10 * time.Millisecond
	longerChTimeout  =  defaultChTimeout * 2
)

var LoginStatusTestCases = []struct {
	reqType         mc.HandshakeState
	denyAction      server.BackendAction
	unknownAction   server.BackendAction
	onlineAction    server.BackendAction
	offlineAction   server.BackendAction
	rateLimitAction server.BackendAction
}{
	{
		reqType:         mc.STATUS,
		denyAction:      server.CLOSE,
		unknownAction:   server.SEND_STATUS,
		onlineAction:    server.PROXY,
		offlineAction:   server.SEND_STATUS,
		rateLimitAction: server.CLOSE,
	},
	{
		reqType:         mc.LOGIN,
		denyAction:      server.DISCONNECT,
		unknownAction:   server.CLOSE,
		onlineAction:    server.PROXY,
		offlineAction:   server.DISCONNECT,
		rateLimitAction: server.CLOSE,
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
func setupServer(t *testing.T, serverCfgs config.ServerConfig) chan<- server.BackendRequest {
	ch, err := server.StartBackendWorker(serverCfgs)
	if err != nil {
		t.Fatalf("error during starting up: %v", err)
	}
	return ch
}

func sendRequest_TestTimeout(t *testing.T, reqCh chan<- server.BackendRequest, req server.BackendRequest) server.ProcessAnswer {
	t.Helper()
	answerCh := make(chan server.ProcessAnswer)
	req.Ch = answerCh
	reqCh <- req
	select {
	case answer := <-answerCh:
		t.Log("worker has successfully responded")
		return answer
	case <-time.After(defaultChTimeout):
		t.Fatal("timed out")
	}
	return server.ProcessAnswer{}
}

func sendRequest_IgnoreResult(reqCh chan<- server.BackendRequest, req server.BackendRequest) {
	answerCh := make(chan server.ProcessAnswer)
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

func TestKnownAddr_OfflineServer(t *testing.T) {
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
			req := server.BackendRequest{
				Type: tc.reqType,
			}
			offlineStatusPk := defaultOfflineStatusPacket()
			reqCh := setupServer(t, serverCfg)
			answer := sendRequest_TestTimeout(t, reqCh, req)
			if answer.Action() != tc.offlineAction {
				t.Errorf("expected: %v \ngot: %v", tc.offlineAction, answer.Action())
			}
			receivedBytes := answer.Response()
			received, _ := mc.ReadPacket_WithBytes(receivedBytes)
			if tc.reqType == mc.STATUS {
				if !samePk(offlineStatusPk, received) {
					offlineStatus, _ := mc.UnmarshalClientBoundResponse(offlineStatusPk)
					receivedStatus, _ := mc.UnmarshalClientBoundResponse(received)
					t.Errorf("expected: %v \ngot: %v", offlineStatus, receivedStatus)
				}
			} else if tc.reqType == mc.LOGIN {
				if !samePk(disconPacket, received) {
					expected, _ := mc.UnmarshalClientDisconnect(disconPacket)
					received, _ := mc.UnmarshalClientDisconnect(received)
					t.Errorf("expected: %v \ngot: %v", expected, received)
				}
			}
		})
	}
}

func TestKnownAddr_OnlineServer(t *testing.T) {
	for _, tc := range LoginStatusTestCases {
		t.Run(fmt.Sprintf("reqType-%v", tc.reqType), func(t *testing.T) {
			serverAddr := "ultraviolet"
			targetAddr := testAddr()
			serverCfg := config.ServerConfig{
				Domains: []string{serverAddr},
				ProxyTo: targetAddr,
			}
			req := server.BackendRequest{
				Type: tc.reqType,
			}
			createListener(t, targetAddr)
			reqCh := setupServer(t, serverCfg)
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
func TestKnownAddr_OldRealIPProxy(t *testing.T) {
	serverAddr := "ultraviolet"
	targetAddr := testAddr()
	serverCfg := config.ServerConfig{
		Domains:   []string{serverAddr},
		ProxyTo:   targetAddr,
		OldRealIP: true,
	}
	req := server.BackendRequest{
		Type: mc.LOGIN,
		Handshake: mc.ServerBoundHandshake{
			ServerAddress: "Something",
		},
		Addr: &net.TCPAddr{
			IP:   net.ParseIP("1.1.1.1"),
			Port: 25560,
		},
	}
	createListener(t, targetAddr)
	reqCh := setupServer(t, serverCfg)
	answer := sendRequest_TestTimeout(t, reqCh, req)
	if answer.Action() != server.PROXY {
		t.Fatalf("expected: %v \ngot: %v", server.PROXY, answer.Action())
	}
	receivedHandshakeBytes := answer.Response()
	handshakePacket, _ := mc.ReadPacket_WithBytes(receivedHandshakeBytes)
	handshake, err := mc.UnmarshalServerBoundHandshake2(handshakePacket)
	if err != nil {
		t.Fatalf("error during unmarshal: %v", err)
	}
	if !handshake.IsRealIPAddress() {
		t.Fatal("Should have changed to RealIP format")
	}
	parts := strings.Split(handshake.ServerAddress, mc.RealIPSeparator)
	if len(parts) != 3 {
		t.Errorf("Wrong RealIP format?!? %v", parts)
	}
}

// Add new RealIP proxy
func TestKnownAddr_NewRealIPProxy(t *testing.T) {
	// Create Key file
	privKey, err := ecdsa.GenerateKey(elliptic.P256(), rand.Reader)
	if err != nil {
		t.Fatalf("got error: %v", err)
	}
	keyBytes, err := x509.MarshalECPrivateKey(privKey)
	if err != nil {
		t.Fatalf("error during marshal key: %v", err)
	}

	tmpfile, err := ioutil.TempFile("", "example")
	if err != nil {
		t.Fatal(err)
	}
	defer os.Remove(tmpfile.Name())
	if _, err := tmpfile.Write(keyBytes); err != nil {
		t.Fatal(err)
	}
	if err := tmpfile.Close(); err != nil {
		t.Fatal(err)
	}

	//Test logic
	serverAddr := "ultraviolet"
	targetAddr := testAddr()
	serverCfg := config.ServerConfig{
		Domains:   []string{serverAddr},
		ProxyTo:   targetAddr,
		NewRealIP: true,
		RealIPKey: tmpfile.Name(),
	}
	req := server.BackendRequest{
		Type: mc.LOGIN,
		Handshake: mc.ServerBoundHandshake{
			ServerAddress: "Something",
		},
		Addr: &net.TCPAddr{
			IP:   net.ParseIP("1.1.1.1"),
			Port: 25560,
		},
	}
	createListener(t, targetAddr)
	reqCh := setupServer(t, serverCfg)
	answer := sendRequest_TestTimeout(t, reqCh, req)
	if answer.Action() != server.PROXY {
		t.Fatalf("expected: %v \ngot: %v", server.PROXY, answer.Action())
	}
	receivedHandshakeBytes := answer.Response()
	handshakePacket, _ := mc.ReadPacket_WithBytes(receivedHandshakeBytes)
	handshake, err := mc.UnmarshalServerBoundHandshake2(handshakePacket)
	if err != nil {
		t.Fatalf("error during unmarshal: %v", err)
	}
	if !handshake.IsRealIPAddress() {
		t.Fatal("Should have changed to RealIP format")
	}
	parts := strings.Split(handshake.ServerAddress, mc.RealIPSeparator)
	if len(parts) != 4 {
		t.Errorf("Wrong RealIP format?!? %v", parts)
	}
}

func TestProxyBind(t *testing.T) {
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
			req := server.BackendRequest{
				Type: tc.reqType,
			}

			go func() {
				reqCh := setupServer(t, serverCfg)
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

func TestProxyProtocol(t *testing.T) {
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
				for {
					conn, err := proxyListener.Accept()
					if err != nil {
						errorCh <- err
					}
					connCh <- conn
				}
			}()

			go func() {
				reqCh := setupServer(t, serverCfg)
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

func TestProxy_ManyRequestsWillRateLimit(t *testing.T) {
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
			reqCh := setupServer(t, serverCfg)
			req := server.BackendRequest{
				Type: tc.reqType,
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

func TestProxy_WillAllowNewConn_AfterDurationEnded(t *testing.T) {
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
			reqCh := setupServer(t, serverCfg)
			req := server.BackendRequest{
				Type: tc.reqType,
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

func TestProxy_ManyRequestsWillNotRateLimitStatus(t *testing.T) {
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
	reqCh := setupServer(t, serverCfg)
	req := server.BackendRequest{
		Type: mc.STATUS,
	}
	acceptAllConnsListener(t, targetAddr)
	for i := 0; i < rateLimit; i++ {
		sendRequest_IgnoreResult(reqCh, req)
	}
	answer := sendRequest_TestTimeout(t, reqCh, req)
	if answer.Action() != server.PROXY {
		t.Fatalf("expected: %v \ngot: %v", server.PROXY, answer.Action())
	}
}

func TestServerState_DoesntCallBeforeCooldownIsOver(t *testing.T) {
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
			reqCh := setupServer(t, serverCfg)
			req := server.BackendRequest{
				Type: tc.reqType,
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

func TestServerState_ShouldCallAgainOutOfCooldown(t *testing.T) {
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
			reqCh := setupServer(t, serverCfg)
			req := server.BackendRequest{
				Type: tc.reqType,
			}
			sendRequest_TestTimeout(t, reqCh, req)
			connCh, _ := createListener(t, targetAddr)
			time.Sleep(longerChTimeout)
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

func TestStatusCache_DoesntCallBeforeCooldownIsOver(t *testing.T) {
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
	reqCh := setupServer(t, serverCfg)
	answerCh := make(chan server.ProcessAnswer)
	req := server.BackendRequest{
		Type: mc.STATUS,
		Ch:   answerCh,
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
		if answer.Action() != server.SEND_STATUS {
			t.Errorf("expected: %v \ngot: %v", server.SEND_STATUS, answer.Action())
		}
	case <-time.After(defaultChTimeout):
		t.Error("timed out")
	}
}

func TestStatusCache_ShouldCallAgainOutOfCooldown(t *testing.T) {
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
	reqCh := setupServer(t, serverCfg)
	answerCh := make(chan server.ProcessAnswer)
	req := server.BackendRequest{
		Type: mc.STATUS,
		Ch:   answerCh,
	}
	connCh, _ := createListener(t, targetAddr)
	go func() {
		<-connCh
		conn := <-connCh
		b := bufio.NewReaderSize(conn, 4096)
		mc.ReadPacket3(b)
		mc.ReadPacket3(b)
		packetbytes, _ := statusPacket.Marshal()
		conn.Write(packetbytes)
		buf := make([]byte, 1024)
		n, _ := conn.Read(buf)
		conn.Write(buf[:n])
	}()
	sendRequest_TestTimeout(t, reqCh, req)
	time.Sleep(updateCooldown * 2)
	reqCh <- req

	select {
	case conn := <-connCh: //state is still in cooldown
		t.Log("worker has successfully responded")
		b := bufio.NewReaderSize(conn, 4096)
		mc.ReadPacket3(b)
		mc.ReadPacket3(b)
		packetbytes, _ := statusPacket.Marshal()
		conn.Write(packetbytes)
		buf := make([]byte, 1024)
		n, _ := conn.Read(buf)
		conn.Write(buf[:n])
	case <-time.After(defaultChTimeout):
		t.Fatal("timed out")
	}

	select {
	case answer := <-answerCh:
		t.Log("worker has successfully responded")
		if answer.Action() != server.SEND_STATUS {
			t.Errorf("expected: %v \ngot: %v", server.SEND_STATUS, answer.Action())
		}
	case <-time.After(defaultChTimeout):
		t.Error("timed out")
	}
}
