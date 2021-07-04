package conn_test

import (
	"bytes"
	"errors"
	"io"
	"net"
	"testing"
	"time"

	"github.com/realDragonium/UltraViolet/conn"
	"github.com/realDragonium/UltraViolet/mc"
)

var defaultChTimeout time.Duration = 10 * time.Millisecond

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
		Name: "UltraViolet",
	}
}

func basicLoginStartPacket() mc.Packet {
	return basicLoginStart().Marshal()
}

func basicHandshake() mc.ServerBoundHandshake {
	return mc.ServerBoundHandshake{
		ProtocolVersion: 751,
		ServerAddress:   "UltraViolet",
		ServerPort:      25565,
		NextState:       0, // notice: invalid value
	}
}

func basicHandshakePacket() mc.Packet {
	return basicHandshake().Marshal()
}

func samePK(expected, received mc.Packet) bool {
	sameID := expected.ID == received.ID
	sameData := bytes.Equal(expected.Data, received.Data)

	return sameID && sameData
}

func TestListener(t *testing.T) {
	runSimpleListener := func(newConnCh <-chan net.Conn) {
		mockListener := &testListener{
			newConnCh: newConnCh,
		}
		listener := conn.NewListener(mockListener)
		go func() {
			listener.Serve()
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

func TestReadConnection(t *testing.T) {
	t.Run("Can read handshake packet", func(t *testing.T) {
		client, server := net.Pipe()
		listener := conn.NewListener(nil)
		go listener.ReadConnection(server)

		finishedWritingCh := make(chan struct{})
		go func() {
			hsPk := basicHandshakePacket()
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
	})

	t.Run("will close connection by invalid packet size", func(t *testing.T) {
		client, server := net.Pipe()
		listener := conn.NewListener(nil)
		go listener.ReadConnection(server)

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
	})

	t.Run("can reach DoStatusSequence", func(t *testing.T) {
		client, server := net.Pipe()
		listener := conn.NewListener(nil)
		go listener.ReadConnection(server)

		go func() {
			hs := basicHandshake()
			hs.NextState = 1
			hsPk := hs.Marshal()
			bytes, _ := hsPk.Marshal()
			client.Write(bytes)
		}()

		select {
		case <-listener.StatusReqCh:
			t.Log("successfully reached DoStatusSequence method")
		case <-time.After(defaultChTimeout):
			t.Error("didnt reach DoStatusSequence method in time")
		}
	})

	t.Run("can reach DoLoginSequence", func(t *testing.T) {
		client, server := net.Pipe()
		listener := conn.NewListener(nil)
		go listener.ReadConnection(server)

		go func() {
			hs := basicHandshake()
			hs.NextState = 2
			hsPk := hs.Marshal()
			bytes, _ := hsPk.Marshal()
			client.Write(bytes)
			pk := basicLoginStartPacket()
			bytes, _ = pk.Marshal()
			client.Write(bytes)
		}()

		select {
		case <-listener.LoginReqCh:
			t.Log("successfully reached DoLoginSequence method")
		case <-time.After(defaultChTimeout):
			t.Error("didnt reach DoLoginSequence method in time")
		}
	})
}

func TestDoLoginSequence(t *testing.T) {
	t.Run("Can read start login packet", func(t *testing.T) {
		client, server := net.Pipe()
		handshakeConn := conn.NewHandshakeConn(server, nil)
		handshake := basicHandshake()
		handshakeConn.Handshake = handshake
		listener := conn.NewListener(nil)
		go listener.DoLoginSequence(handshakeConn)

		finishedWritingCh := make(chan struct{})
		go func() {
			hsPk := basicLoginStartPacket()
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
	})

	t.Run("Receives login request through channel", func(t *testing.T) {
		client, server := net.Pipe()
		clientAddr := net.TCPAddr{IP: []byte{1, 1, 1, 1}, Port: 0}
		handshakeConn := conn.NewHandshakeConn(server, &clientAddr)
		handshake := basicHandshake()
		handshakeConn.Handshake = handshake
		listener := conn.NewListener(nil)
		go listener.DoLoginSequence(handshakeConn)

		finishedWritingCh := make(chan struct{})
		go func() {
			pk := basicLoginStartPacket()
			bytes, _ := pk.Marshal()
			client.Write(bytes)
			finishedWritingCh <- struct{}{}
		}()

		select {
		case request := <-listener.LoginReqCh:
			t.Log("test has successfully written data to server")
			if request.ServerAddr != "UltraViolet" {
				t.Errorf("Expected: UltraViolet got:%v", request.ServerAddr)
			}
			if request.Username != "UltraViolet" {
				t.Errorf("Expected: UltraViolet got: %v", request.Username)
			}
			if request.Ip != &clientAddr {
				t.Errorf("Expected: UltraViolet got: %v", request.Ip)
			}
		case <-time.After(defaultChTimeout):
			t.Error("test hasnt finished writen to server in time")
		}
	})

	t.Run("can proxy connection", func(t *testing.T) {
		client, proxyFrontend := net.Pipe()
		handshakeConn := conn.NewHandshakeConn(proxyFrontend, &net.TCPAddr{IP: []byte{1, 1, 1, 1}, Port: 0})
		handshake := basicHandshake()
		listener := conn.NewListener(nil)
		handshakeConn.Handshake = handshake
		go listener.DoLoginSequence(handshakeConn)
		hsPk := basicLoginStartPacket()
		bytes, _ := hsPk.Marshal()
		client.Write(bytes)
		<-listener.LoginReqCh

		proxyBackend, server := net.Pipe()
		listener.LoginAnsCh <- conn.LoginAnswer{
			Proxy:      true,
			ServerConn: proxyBackend,
		}
		testProxyConn(t, client, server)
	})

	t.Run("can send disconnect packet", func(t *testing.T) {
		client, proxyFrontend := net.Pipe()
		handshakeConn := conn.NewHandshakeConn(proxyFrontend, &net.TCPAddr{IP: []byte{1, 1, 1, 1}, Port: 0})
		handshake := basicHandshake()
		listener := conn.NewListener(nil)
		handshakeConn.Handshake = handshake
		go listener.DoLoginSequence(handshakeConn)
		hsPk := basicLoginStartPacket()
		clientConn := conn.NewHandshakeConn(client, nil)
		clientConn.WritePacket(hsPk)
		<-listener.LoginReqCh
		disconMessage := "Because we dont want people like you"
		disconPk := mc.ClientBoundDisconnect{
			Reason: mc.Chat(disconMessage),
		}.Marshal()
		listener.LoginAnsCh <- conn.LoginAnswer{
			Proxy:         false,
			DisconMessage: disconPk,
		}

		disconnectPacket, _ := clientConn.ReadPacket()
		byteReader := bytes.NewReader(disconnectPacket.Data)
		message, _ := mc.ReadString(byteReader)

		if string(message) != disconMessage {
			t.Errorf("expected: %v got: %v", disconMessage, string(message))
		}
	})

	t.Run("expect connection to be closed after disconnect", func(t *testing.T) {
		client, proxyFrontend := net.Pipe()
		handshakeConn := conn.NewHandshakeConn(proxyFrontend, &net.TCPAddr{IP: []byte{1, 1, 1, 1}, Port: 0})
		handshake := basicHandshake()
		listener := conn.NewListener(nil)
		handshakeConn.Handshake = handshake
		go listener.DoLoginSequence(handshakeConn)
		hsPk := basicLoginStartPacket()
		clientConn := conn.NewHandshakeConn(client, nil)
		clientConn.WritePacket(hsPk)
		<-listener.LoginReqCh
		listener.LoginAnsCh <- conn.LoginAnswer{
			Proxy:         false,
			DisconMessage: mc.ClientBoundDisconnect{}.Marshal(),
		}
		clientConn.ReadPacket()

		testConnectedClosed(t, client)
	})
}

func testProxyConn(t *testing.T, client, server net.Conn) {
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
		t.Fatal("Client couldnt write to Server")
	}

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
		t.Fatal("Server couldnt write to Client")
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
		go conn.ProxyConnections(client, server)
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
		go conn.ProxyConnections(client, server)
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

func TestDoStatusSequence(t *testing.T) {
	t.Run("send request through channel", func(t *testing.T) {
		handshakeConn := conn.NewHandshakeConn(nil, &net.TCPAddr{IP: []byte{1, 1, 1, 1}, Port: 0})
		listener := conn.NewListener(nil)

		go listener.DoStatusSequence(handshakeConn)

		select {
		case <-listener.StatusReqCh:
		case <-time.After(defaultChTimeout):
			t.Fatal("method didnt send status request")
		}
	})

	t.Run("replay answer through channel", func(t *testing.T) {
		_, proxyFrontend := net.Pipe()
		handshakeConn := conn.NewHandshakeConn(proxyFrontend, &net.TCPAddr{IP: []byte{1, 1, 1, 1}, Port: 0})
		listener := conn.NewListener(nil)

		go listener.DoStatusSequence(handshakeConn)
		<-listener.StatusReqCh

		statusAnswer := conn.StatusAnswer{}

		select {
		case listener.StatusAnsCh <- statusAnswer:
		case <-time.After(defaultChTimeout):
			t.Fatal("method receive status answer")
		}
	})

	t.Run("can proxy connection to server", func(t *testing.T) {
		client, proxyFrontend := net.Pipe()
		handshakeConn := conn.NewHandshakeConn(proxyFrontend, &net.TCPAddr{IP: []byte{1, 1, 1, 1}, Port: 0})
		listener := conn.NewListener(nil)

		go listener.DoStatusSequence(handshakeConn)
		<-listener.StatusReqCh
		proxyBackend, server := net.Pipe()
		statusAnswer := conn.StatusAnswer{
			Proxy:      true,
			ServerConn: proxyBackend,
		}
		listener.StatusAnsCh <- statusAnswer
		testProxyConn(t, client, server)
	})

	t.Run("can proxy connection to server", func(t *testing.T) {
		client, proxyFrontend := net.Pipe()
		handshakeConn := conn.NewHandshakeConn(proxyFrontend, &net.TCPAddr{IP: []byte{1, 1, 1, 1}, Port: 0})
		listener := conn.NewListener(nil)

		go listener.DoStatusSequence(handshakeConn)
		<-listener.StatusReqCh
		statusPk := mc.AnotherStatusResponse{
			Name:        "UltraViolet",
			Protocol:    751,
			Description: "Some broken proxy",
		}.Marshal()
		statusAnswer := conn.StatusAnswer{
			Proxy:    false,
			StatusPk: statusPk,
		}
		listener.StatusAnsCh <- statusAnswer

		clientConn := conn.NewHandshakeConn(client, nil)
		err := clientConn.WritePacket(mc.ServerBoundRequest{}.Marshal())
		if err != nil {
			t.Fatalf("expected no error but got: %v", err)
		}

		receivedPk, err := clientConn.ReadPacket()
		if err != nil {
			t.Fatalf("expected no error but got: %v", err)
		}
		if !samePK(statusPk, receivedPk) {
			t.Errorf("different packet received. \nexpected:\t %v, \ngot:\t %v", statusPk, receivedPk)
		}

		pingPk := mc.NewServerBoundPing().Marshal()
		if err := clientConn.WritePacket(pingPk); err != nil {
			t.Fatalf("expected no error but got: %v", err)
		}

		pongPk, err := clientConn.ReadPacket()
		if err != nil {
			t.Fatalf("expected no error but got: %v", err)
		}
		if !samePK(pingPk, pongPk) {
			t.Errorf("different packet received. \nexpected:\t %v, \ngot:\t %v", statusPk, receivedPk)
		}
	})

	t.Run("close connection after non proxied response", func(t *testing.T) {
		client, proxyFrontend := net.Pipe()
		handshakeConn := conn.NewHandshakeConn(proxyFrontend, &net.TCPAddr{IP: []byte{1, 1, 1, 1}, Port: 0})
		listener := conn.NewListener(nil)

		go listener.DoStatusSequence(handshakeConn)
		<-listener.StatusReqCh
		statusPk := mc.AnotherStatusResponse{}.Marshal()

		listener.StatusAnsCh <- conn.StatusAnswer{
			Proxy:    false,
			StatusPk: statusPk,
		}

		clientConn := conn.NewHandshakeConn(client, nil)
		statusRequestPk := mc.ServerBoundRequest{}.Marshal()
		clientConn.WritePacket(statusRequestPk)

		clientConn.ReadPacket()
		pingPk := mc.NewServerBoundPing().Marshal()
		clientConn.WritePacket(pingPk)
		clientConn.ReadPacket()

		testConnectedClosed(t, client)
	})
}
