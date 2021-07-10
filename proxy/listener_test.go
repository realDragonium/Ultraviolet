package proxy_test

import (
	"bytes"
	"errors"
	"io"
	"net"
	"testing"
	"time"

	"github.com/realDragonium/Ultraviolet/mc"
	"github.com/realDragonium/Ultraviolet/proxy"
)

var (
	defaultChTimeout = 10 * time.Millisecond
	longerChTimeout  = 100 * time.Millisecond
)

type testNetConn struct {
	conn       net.Conn
	remoteAddr net.Addr
}

func (c *testNetConn) RemoteAddr() net.Addr {
	return c.remoteAddr
}
func (c *testNetConn) Read(b []byte) (n int, err error) {
	return c.conn.Read(b)
}
func (c *testNetConn) Write(b []byte) (n int, err error) {
	return c.conn.Write(b)
}
func (c *testNetConn) Close() error {
	return c.conn.Close()
}
func (c *testNetConn) LocalAddr() net.Addr {
	return c.conn.LocalAddr()
}
func (c *testNetConn) SetDeadline(t time.Time) error {
	return c.conn.SetDeadline(t)
}
func (c *testNetConn) SetReadDeadline(t time.Time) error {
	return c.conn.SetReadDeadline(t)
}
func (c *testNetConn) SetWriteDeadline(t time.Time) error {
	return c.conn.SetWriteDeadline(t)
}

type testListener struct {
	newConnCh <-chan net.Conn
}

func (l *testListener) Close() error {
	return nil
}

func (l *testListener) Addr() net.Addr {
	return nil
}

func (l *testListener) Accept() (net.Conn, error) {
	return <-l.newConnCh, nil
}

func basicLoginStart() mc.ServerLoginStart {
	return mc.ServerLoginStart{
		Name: "Ultraviolet",
	}
}

func basicLoginStartPacket() mc.Packet {
	return basicLoginStart().Marshal()
}

func loginHandshakePacket() mc.Packet {
	return basicHandshake(2).Marshal()
}

func basicHandshake(state mc.VarInt) mc.ServerBoundHandshake {
	return mc.ServerBoundHandshake{
		ProtocolVersion: 751,
		ServerAddress:   "Ultraviolet",
		ServerPort:      25565,
		NextState:       state,
	}
}

func statusHandshakePacket() mc.Packet {
	return basicHandshake(1).Marshal()
}

func samePK(expected, received mc.Packet) bool {
	sameID := expected.ID == received.ID
	sameData := bytes.Equal(expected.Data, received.Data)

	return sameID && sameData
}

func newProxyChan() chan proxy.ProxyAction {
	proxyCh := make(chan proxy.ProxyAction)
	go func() {
		for {
			<-proxyCh
		}
	}()
	return proxyCh
}

func TestListener(t *testing.T) {
	runSimpleListener := func(newConnCh <-chan net.Conn) {
		reqCh := make(chan proxy.McRequest)
		mockListener := &testListener{
			newConnCh: newConnCh,
		}
		go func() {
			proxy.ServeListener(mockListener, reqCh)
		}()
	}

	t.Run("accept call", func(t *testing.T) {
		newConnCh := make(chan net.Conn)
		runSimpleListener(newConnCh)

		select {
		case newConnCh <- &net.TCPConn{}:
			t.Log("Listener called accept")
		case <-time.After(defaultChTimeout):
			t.Error("Listener didnt accept connection")
		}
	})

	t.Run("accept multiple calls", func(t *testing.T) {
		newConnCh := make(chan net.Conn)
		runSimpleListener(newConnCh)
		select {
		case newConnCh <- &net.TCPConn{}:
			t.Log("Listener accepted first connection")
		case <-time.After(defaultChTimeout):
			t.Error("Listener didnt accept first connection")
		}
		select {
		case newConnCh <- &net.TCPConn{}:
			t.Log("Listener accepted second connection")
		case <-time.After(defaultChTimeout):
			t.Error("Listener didnt accept second connection")
		}
	})

}

func TestReadConnection_ReceivesLoginRequest_ThroughChannel(t *testing.T) {
	clientConn, proxyFrontend := net.Pipe()
	clientAddr := net.TCPAddr{IP: []byte{1, 1, 1, 1}, Port: 0}
	mockClientconn := testNetConn{
		conn:       proxyFrontend,
		remoteAddr: &clientAddr,
	}
	reqCh := make(chan proxy.McRequest)
	go proxy.ReadConnection(&mockClientconn, reqCh)

	go func() {
		client := proxy.NewMcConn(clientConn)
		hsPk := loginHandshakePacket()
		client.WritePacket(hsPk)
		loginPk := basicLoginStartPacket()
		client.WritePacket(loginPk)
	}()

	select {
	case request := <-reqCh:
		t.Log("test has successfully written data to server")
		t.Log(request)
		if request.ServerAddr != "Ultraviolet" {
			t.Errorf("Expected: Ultraviolet got:%v", request.ServerAddr)
		}
		if request.Username != "Ultraviolet" {
			t.Errorf("Expected: Ultraviolet got: %v", request.Username)
		}
		if request.Addr != &clientAddr {
			t.Errorf("Expected: Ultraviolet got: %v", request.Addr)
		}
	case <-time.After(defaultChTimeout):
		t.Error("test hasnt finished writen to server in time")
	}
}

func TestReadConnection_CanReadHSPk(t *testing.T) {
	client, server := net.Pipe()
	reqCh := make(chan proxy.McRequest)
	go proxy.ReadConnection(server, reqCh)

	finishedWritingCh := make(chan struct{})
	go func() {
		hsPk := statusHandshakePacket()
		bytes, _ := hsPk.Marshal()
		client.Write(bytes)
		finishedWritingCh <- struct{}{}
	}()

	select {
	case <-finishedWritingCh:
		t.Log("test has successfully written data to server")
	case <-time.After(defaultChTimeout):
		t.Error("test hasnt finished writen to server in time")
	}
}

func TestReadConnection_WillCloseConn_WhenInvalidPacketSize(t *testing.T) {
	client, server := net.Pipe()
	reqCh := make(chan proxy.McRequest)
	go proxy.ReadConnection(server, reqCh)

	finishedWritingCh := make(chan struct{})
	go func() {
		pkData := make([]byte, 2097160)
		pk := mc.Packet{Data: pkData}
		bytes, _ := pk.Marshal()
		client.Write(bytes)
		finishedWritingCh <- struct{}{}
	}()

	select {
	case <-finishedWritingCh:
		t.Log("test has successfully written data to server")
	case <-time.After(defaultChTimeout):
		t.Error("test hasnt finished writen to server in time")
	}

	_, err := client.Write([]byte{0})
	if !errors.Is(err, io.ErrClosedPipe) {
		t.Fail()
	}
}

func TestReadConnection_CanReadStartLoginPk(t *testing.T) {
	clientConn, proxyFrontend := net.Pipe()
	reqCh := make(chan proxy.McRequest)
	go proxy.ReadConnection(proxyFrontend, reqCh)

	finishedWritingCh := make(chan struct{})
	go func() {
		client := proxy.NewMcConn(clientConn)
		hsPk := loginHandshakePacket()
		client.WritePacket(hsPk)
		loginPk := basicLoginStartPacket()
		client.WritePacket(loginPk)
		finishedWritingCh <- struct{}{}
	}()

	select {
	case <-finishedWritingCh:
		t.Log("test has successfully written data to server")
	case <-time.After(defaultChTimeout):
		t.Error("test hasnt finished writen to server in time")
	}
}

func TestReadConnection_CloseResponse_ClosesChannel(t *testing.T) {
	reqCh := make(chan proxy.McRequest)
	clientConn, proxyFrontend := net.Pipe()
	go proxy.ReadConnection(proxyFrontend, reqCh)

	go func() {
		client := proxy.NewMcConn(clientConn)
		hsPk := statusHandshakePacket()
		client.WritePacket(hsPk)
	}()

	request := <-reqCh

	request.Ch <- proxy.McAnswer{
		Action: proxy.CLOSE,
	}
	testConnectedClosed(t, clientConn)
}

func TestReadConnection_CanProxyLoginConnection(t *testing.T) {
	reqCh := make(chan proxy.McRequest)
	clientConn, proxyFrontend := net.Pipe()
	proxyBackend, serverConn := net.Pipe()
	go proxy.ReadConnection(proxyFrontend, reqCh)

	hsPk := loginHandshakePacket()
	loginPk := basicLoginStartPacket()

	client := proxy.NewMcConn(clientConn)
	client.WritePacket(hsPk)
	client.WritePacket(loginPk)
	request := <-reqCh

	connFunc := func(net.Addr) (net.Conn, error) {
		return proxyBackend, nil
	}

	request.Ch <- proxy.McAnswer{
		Action:  proxy.PROXY,
		ProxyCh: newProxyChan(),
		// ServerConn:     proxy.NewMcConn(proxyBackend),
		ServerConnFunc: connFunc,
	}

	server := proxy.NewMcConn(serverConn)
	receivedHsPk, _ := server.ReadPacket()
	if !samePK(hsPk, receivedHsPk) {
		t.Errorf("expected: %v, \ngot: %v", hsPk, receivedHsPk)
	}
	receivedLoginPk, _ := server.ReadPacket()
	if !samePK(loginPk, receivedLoginPk) {
		t.Errorf("expected:%v, \ngot: %v", loginPk, receivedLoginPk)
	}
	testProxyConn(t, clientConn, serverConn)
	testProxyConn(t, serverConn, clientConn)
}

func TestReadConnection_CanSendDisconnectPk(t *testing.T) {
	reqCh := make(chan proxy.McRequest)
	clientConn, proxyFrontend := net.Pipe()
	go proxy.ReadConnection(proxyFrontend, reqCh)

	client := proxy.NewMcConn(clientConn)
	hsPk := loginHandshakePacket()
	client.WritePacket(hsPk)
	loginPk := basicLoginStartPacket()
	client.WritePacket(loginPk)
	request := <-reqCh

	disconMessage := "Because we dont want people like you"
	disconPk := mc.ClientBoundDisconnect{
		Reason: mc.Chat(disconMessage),
	}.Marshal()

	request.Ch <- proxy.McAnswer{
		Action:        proxy.DISCONNECT,
		DisconMessage: disconPk,
	}

	disconnectPacket, _ := client.ReadPacket()
	byteReader := bytes.NewReader(disconnectPacket.Data)
	message, _ := mc.ReadString(byteReader)

	if string(message) != disconMessage {
		t.Errorf("expected: %v got: %v", disconMessage, string(message))
	}
}

func TestReadConnection_ExpectConnToBeClosed_AfterDisconnect(t *testing.T) {
	clientConn, proxyFrontend := net.Pipe()
	reqCh := make(chan proxy.McRequest)
	go proxy.ReadConnection(proxyFrontend, reqCh)

	client := proxy.NewMcConn(clientConn)
	hsPk := loginHandshakePacket()
	client.WritePacket(hsPk)
	loginPk := basicLoginStartPacket()
	client.WritePacket(loginPk)

	request := <-reqCh
	request.Ch <- proxy.McAnswer{
		Action:        proxy.DISCONNECT,
		DisconMessage: mc.ClientBoundDisconnect{}.Marshal(),
	}
	client.ReadPacket()

	testConnectedClosed(t, clientConn)
}

func TestReadConnection_SendStatusRequest_ThroughChannel(t *testing.T) {
	reqCh := make(chan proxy.McRequest)
	clientConn, proxyFrontend := net.Pipe()
	go proxy.ReadConnection(proxyFrontend, reqCh)

	client := proxy.NewMcConn(clientConn)
	hsPk := statusHandshakePacket()
	client.WritePacket(hsPk)

	select {
	case <-reqCh:
	case <-time.After(defaultChTimeout):
		t.Error("method didnt send status request")
	}
}

func TestReadConnection_CanProxyConnToServer(t *testing.T) {
	clientConn, proxyFrontend := net.Pipe()
	proxyBackend, serverConn := net.Pipe()
	reqCh := make(chan proxy.McRequest)
	go proxy.ReadConnection(proxyFrontend, reqCh)

	hsPk := statusHandshakePacket()

	go func() {
		client := proxy.NewMcConn(clientConn)
		client.WritePacket(hsPk)
		request := <-reqCh

		connFunc := func(net.Addr) (net.Conn, error) {
			return proxyBackend, nil
		}

		request.Ch <- proxy.McAnswer{
			Action:  proxy.PROXY,
			ProxyCh: newProxyChan(),
			// ServerConn: proxy.NewMcConn(proxyBackend),
			ServerConnFunc: connFunc,
		}
	}()

	server := proxy.NewMcConn(serverConn)
	receivedHsPk, _ := server.ReadPacket()
	if !samePK(hsPk, receivedHsPk) {
		t.Errorf("expected:\t %v, \ngot:\t %v", hsPk, receivedHsPk)
	}
	testProxyConn(t, clientConn, serverConn)
	testProxyConn(t, serverConn, clientConn)
}

func TestReadConnection_CanReplyToStatus(t *testing.T) {
	clientConn, proxyFrontend := net.Pipe()
	reqCh := make(chan proxy.McRequest)
	go proxy.ReadConnection(proxyFrontend, reqCh)

	client := proxy.NewMcConn(clientConn)
	hsPk := statusHandshakePacket()
	client.WritePacket(hsPk)

	request := <-reqCh
	statusPk := mc.AnotherStatusResponse{
		Name:        "Ultraviolet",
		Protocol:    751,
		Description: "Some broken proxy",
	}.Marshal()
	statusAnswer := proxy.McAnswer{
		Action:   proxy.SEND_STATUS,
		StatusPk: statusPk,
	}
	request.Ch <- statusAnswer

	client.WritePacket(mc.ServerBoundRequest{}.Marshal())
	receivedPk, _ := client.ReadPacket()
	if !samePK(statusPk, receivedPk) {
		t.Errorf("expected:\t %v, \ngot:\t %v", statusPk, receivedPk)
	}
	pingPk := mc.NewServerBoundPing().Marshal()
	client.WritePacket(pingPk)
	pongPk, _ := client.ReadPacket()
	if !samePK(pingPk, pongPk) {
		t.Errorf("expected:\t %v, \ngot:\t %v", statusPk, receivedPk)
	}
}

func TestReadConnection_CloseConnAfterNonProxiedStatusResponse(t *testing.T) {
	clientConn, proxyFrontend := net.Pipe()
	reqCh := make(chan proxy.McRequest)
	go proxy.ReadConnection(proxyFrontend, reqCh)

	client := proxy.NewMcConn(clientConn)
	hsPk := statusHandshakePacket()
	client.WritePacket(hsPk)

	request := <-reqCh
	statusPk := mc.AnotherStatusResponse{
		Name:        "Ultraviolet",
		Protocol:    751,
		Description: "Some broken proxy",
	}.Marshal()
	statusAnswer := proxy.McAnswer{
		Action:   proxy.SEND_STATUS,
		StatusPk: statusPk,
	}
	request.Ch <- statusAnswer

	client.WritePacket(mc.ServerBoundRequest{}.Marshal())
	client.ReadPacket()
	pingPk := mc.NewServerBoundPing().Marshal()
	client.WritePacket(pingPk)
	client.ReadPacket()

	testConnectedClosed(t, clientConn)
}

func testProxyConn(t *testing.T, conn1, conn2 net.Conn) {
	readBuffer := make([]byte, 10)
	couldReachCh := make(chan struct{})

	go func() {
		_, err := conn1.Write([]byte{1, 2, 3, 4, 5, 6, 7, 8, 9})
		t.Log(err)
		couldReachCh <- struct{}{}
	}()
	go func() {
		_, err := conn2.Read(readBuffer)
		t.Log(err)
	}()
	select {
	case <-couldReachCh:
	case <-time.After(defaultChTimeout):
		t.Helper()
		t.Error("conn1 couldnt write to conn2")
	}
}

func testConnectedClosed(t *testing.T, conn net.Conn) {
	errCh := make(chan error)
	go func() {
		_, err := conn.Write([]byte{1})
		errCh <- err
	}()

	select {
	case err := <-errCh:
		if !errors.Is(err, io.ErrClosedPipe) {
			t.Errorf("expected closed pipe error but got: %v", err)
		}
	case <-time.After(defaultChTimeout):
		t.Fatal("Expected connection to be closed")
	}
}

func TestProxyConnection(t *testing.T) {
	t.Run("Client writes to Server", func(t *testing.T) {
		client, server := net.Pipe()
		proxyCh := newProxyChan()

		go proxy.ProxyConnections(client, server, proxyCh)
		readBuffer := make([]byte, 10)
		couldReachCh := make(chan struct{})
		go func() {
			client.Write([]byte{1, 2, 3, 4, 5, 6, 7, 8, 9})
			couldReachCh <- struct{}{}
		}()
		go func() {
			server.Read(readBuffer)
		}()
		select {
		case <-couldReachCh:
		case <-time.After(defaultChTimeout):
			t.Fail()
		}
	})

	t.Run("Server writes to Client", func(t *testing.T) {
		client, server := net.Pipe()
		proxyCh := newProxyChan()

		go proxy.ProxyConnections(client, server, proxyCh)
		readBuffer := make([]byte, 10)
		couldReachCh := make(chan struct{})
		go func() {
			server.Write([]byte{1, 2, 3, 4, 5, 6, 7, 8, 9})
			couldReachCh <- struct{}{}
		}()
		go func() {
			client.Read(readBuffer)
		}()
		select {
		case <-couldReachCh:
		case <-time.After(defaultChTimeout):
			t.Fail()
		}
	})

}
