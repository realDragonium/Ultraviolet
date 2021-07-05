package conn_test

import (
	"bytes"
	"errors"
	"io"
	"net"
	"testing"
	"time"

	"github.com/realDragonium/Ultraviolet/conn"
	"github.com/realDragonium/Ultraviolet/mc"
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
		Name: "Ultraviolet",
	}
}

func basicLoginStartPacket() mc.Packet {
	return basicLoginStart().Marshal()
}

func basicHandshake() mc.ServerBoundHandshake {
	return mc.ServerBoundHandshake{
		ProtocolVersion: 751,
		ServerAddress:   "Ultraviolet",
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
		reqCh := make(chan conn.ConnRequest)
		mockListener := &testListener{
			newConnCh: newConnCh,
		}
		go func() {
			conn.Serve(mockListener, reqCh)
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
		reqCh := make(chan conn.ConnRequest)
		go conn.ReadConnection(server, reqCh)

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
		reqCh := make(chan conn.ConnRequest)
		go conn.ReadConnection(server, reqCh)

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
		reqCh := make(chan conn.ConnRequest)
		go conn.ReadConnection(server, reqCh)
		go func() {
			hs := basicHandshake()
			hs.NextState = 1
			hsPk := hs.Marshal()
			bytes, _ := hsPk.Marshal()
			client.Write(bytes)
		}()

		select {
		case <-reqCh:
			t.Log("successfully reached DoStatusSequence method")
		case <-time.After(defaultChTimeout):
			t.Error("didnt reach DoStatusSequence method in time")
		}
	})

	t.Run("can reach DoLoginSequence", func(t *testing.T) {
		client, server := net.Pipe()
		reqCh := make(chan conn.ConnRequest)
		go conn.ReadConnection(server, reqCh)

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
		case <-reqCh:
			t.Log("successfully reached DoLoginSequence method")
		case <-time.After(defaultChTimeout):
			t.Error("didnt reach DoLoginSequence method in time")
		}
	})
}

func TestDoLoginSequence(t *testing.T) {
	t.Run("Can read start login packet", func(t *testing.T) {
		client, server := net.Pipe()
		reqCh := make(chan conn.ConnRequest)
		go conn.ReadConnection(server, reqCh)

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

		reqCh := make(chan conn.ConnRequest)
		go conn.ReadConnection(server, reqCh)

		finishedWritingCh := make(chan struct{})
		go func() {
			pk := basicLoginStartPacket()
			conn.NewMcConn(client).WritePacket(pk)
			finishedWritingCh <- struct{}{}
		}()

		select {
		case request := <-reqCh:
			t.Log("test has successfully written data to server")
			if request.ServerAddr != "Ultraviolet" {
				t.Errorf("Expected: Ultraviolet got:%v", request.ServerAddr)
			}
			if request.Username != "Ultraviolet" {
				t.Errorf("Expected: Ultraviolet got: %v", request.Username)
			}
			if request.Ip != &clientAddr {
				t.Errorf("Expected: Ultraviolet got: %v", request.Ip)
			}
		case <-time.After(defaultChTimeout):
			t.Error("test hasnt finished writen to server in time")
		}
	})

	t.Run("can proxy connection", func(t *testing.T) {
		client, proxyFrontend := net.Pipe()
		reqCh := make(chan conn.ConnRequest)
		go conn.ReadConnection(proxyFrontend, reqCh)

		hsPk := basicLoginStartPacket()
		bytes, _ := hsPk.Marshal()
		client.Write(bytes)
		request := <-reqCh

		proxyBackend, server := net.Pipe()
		request.Ch <- conn.ConnAnswer{
			Action:     conn.PROXY,
			ServerConn: conn.NewMcConn(proxyBackend),
		}
		server.Read([]byte{0, 0, 0, 0, 0, 0, 0, 0, 0, 0, 0, 0, 0, 0}) // Read handshake
		server.Read([]byte{0, 0, 0, 0, 0, 0, 0, 0, 0, 0, 0, 0, 0, 0}) // Read LoginStart
		testProxyConn(t, client, server)
		testProxyConn(t, server, server)
	})

	t.Run("can send disconnect packet", func(t *testing.T) {
		client, proxyFrontend := net.Pipe()
		reqCh := make(chan conn.ConnRequest)
		go conn.ReadConnection(proxyFrontend, reqCh)

		hsPk := basicLoginStartPacket()
		clientConn := conn.NewMcConn(client)
		clientConn.WritePacket(hsPk)
		request := <-reqCh

		disconMessage := "Because we dont want people like you"
		disconPk := mc.ClientBoundDisconnect{
			Reason: mc.Chat(disconMessage),
		}.Marshal()

		request.Ch <- conn.ConnAnswer{
			Action:        conn.DISCONNECT,
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
		reqCh := make(chan conn.ConnRequest)
		go conn.ReadConnection(proxyFrontend, reqCh)

		hsPk := basicLoginStartPacket()
		clientConn := conn.NewMcConn(client)
		clientConn.WritePacket(hsPk)

		request := <-reqCh
		request.Ch <- conn.ConnAnswer{
			Action:        conn.DISCONNECT,
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

func TestDoStatusSequence(t *testing.T) {
	t.Run("send request through channel", func(t *testing.T) {
		reqCh := make(chan conn.ConnRequest)
		go conn.ReadConnection(nil, reqCh)

		select {
		case <-reqCh:
		case <-time.After(defaultChTimeout):
			t.Fatal("method didnt send status request")
		}
	})

	t.Run("replay answer through channel", func(t *testing.T) {
		_, proxyFrontend := net.Pipe()
		reqCh := make(chan conn.ConnRequest)
		go conn.ReadConnection(proxyFrontend, reqCh)

		request := <-reqCh

		statusAnswer := conn.ConnAnswer{
			Action: conn.CLOSE,
		}

		select {
		case request.Ch <- statusAnswer:
		case <-time.After(defaultChTimeout):
			t.Fatal("method receive status answer")
		}
	})

	t.Run("can proxy connection to server", func(t *testing.T) {
		client, proxyFrontend := net.Pipe()
		reqCh := make(chan conn.ConnRequest)
		go conn.ReadConnection(proxyFrontend, reqCh)

		request := <-reqCh
		proxyBackend, server := net.Pipe()
		statusAnswer := conn.ConnAnswer{
			Action:     conn.PROXY,
			ServerConn: conn.NewMcConn(proxyBackend),
		}
		request.Ch <- statusAnswer
		server.Read([]byte{0, 0, 0, 0, 0, 0, 0, 0, 0, 0, 0, 0, 0, 0}) // Read handshake
		testProxyConn(t, client, server)
		testProxyConn(t, server, client)
	})

	t.Run("can proxy connection to server", func(t *testing.T) {
		client, proxyFrontend := net.Pipe()
		reqCh := make(chan conn.ConnRequest)
		go conn.ReadConnection(proxyFrontend, reqCh)

		request := <-reqCh
		statusPk := mc.AnotherStatusResponse{
			Name:        "Ultraviolet",
			Protocol:    751,
			Description: "Some broken proxy",
		}.Marshal()
		statusAnswer := conn.ConnAnswer{
			Action:   conn.SEND_STATUS,
			StatusPk: statusPk,
		}
		request.Ch <- statusAnswer

		clientConn := conn.NewMcConn(client)
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
		reqCh := make(chan conn.ConnRequest)
		go conn.ReadConnection(proxyFrontend, reqCh)

		request := <-reqCh
		statusPk := mc.AnotherStatusResponse{}.Marshal()
		request.Ch <- conn.ConnAnswer{
			Action:   conn.SEND_STATUS,
			StatusPk: statusPk,
		}

		clientConn := conn.NewMcConn(client)
		statusRequestPk := mc.ServerBoundRequest{}.Marshal()
		clientConn.WritePacket(statusRequestPk)

		clientConn.ReadPacket()
		pingPk := mc.NewServerBoundPing().Marshal()
		clientConn.WritePacket(pingPk)
		clientConn.ReadPacket()

		testConnectedClosed(t, client)
	})
}
