package ultraviolet_test

import (
	"bytes"
	"errors"
	"fmt"
	"io"
	"log"
	"net"
	"testing"
	"time"

	ultraviolet "github.com/realDragonium/Ultraviolet"
	"github.com/realDragonium/Ultraviolet/config"
	"github.com/realDragonium/Ultraviolet/mc"
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

func basicLoginStart() mc.ServerLoginStart {
	return mc.ServerLoginStart{
		Name: "Ultraviolet",
	}
}

func basicStatusRequestPacket() mc.Packet {
	return mc.ServerBoundRequest{}.Marshal()
}

func basicLoginStartPacket() mc.Packet {
	return basicLoginStart().Marshal()
}

func basicHandshakePacket(state int) mc.Packet {
	return basicHandshake(state).Marshal()
}

func loginHandshakePacket() mc.Packet {
	return basicHandshake(2).Marshal()
}

func basicHandshake(state int) mc.ServerBoundHandshake {
	return mc.ServerBoundHandshake{
		ProtocolVersion: 751,
		ServerAddress:   "Ultraviolet",
		ServerPort:      25565,
		NextState:       state,
	}
}

func samePK(expected, received mc.Packet) bool {
	sameID := expected.ID == received.ID
	sameData := bytes.Equal(expected.Data, received.Data)

	return sameID && sameData
}

func newWorker(cfg config.WorkerConfig) (ultraviolet.BasicWorker, chan net.Conn) {
	reqCh := make(chan net.Conn)
	r := ultraviolet.NewWorker(cfg, reqCh)
	return r, reqCh
}

func testPipeConnClosed(t *testing.T, conn net.Conn) {
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

func testProxyConn(t *testing.T, conn1, conn2 net.Conn) {
	t.Helper()
	readBuffer := make([]byte, 10)
	couldReachCh := make(chan struct{})

	go func() {
		_, err := conn1.Write([]byte{1, 2, 3, 4, 5, 6, 7, 8, 9})
		if err != nil {
			t.Log(err)
		}
		couldReachCh <- struct{}{}
	}()
	go func() {
		_, err := conn2.Read(readBuffer)
		if err != nil {
			t.Log(err)
		}
	}()
	select {
	case <-couldReachCh:
	case <-time.After(defaultChTimeout):
		t.Error("conn1 couldnt write to conn2")
	}
}

func TestWorker_CanReceiveConnection(t *testing.T) {
	cfg := config.DefaultWorkerConfig()
	worker, reqCh := newWorker(cfg)
	go worker.Work()
	select {
	case reqCh <- &net.TCPConn{}:
		t.Log("worker has successfully received request")
	case <-time.After(defaultChTimeout):
		t.Error("timed out")
	}
}

func TestProcessRequest_UnknownAddr(t *testing.T) {
	for _, tc := range RequestStateInfo {
		t.Run(fmt.Sprintf("reqType-%v", tc.reqType), func(t *testing.T) {
			cfg := config.DefaultWorkerConfig()
			basicWorker, _ := newWorker(cfg)

			request := ultraviolet.BackendRequest{
				ServerAddr: "ultraviolet",
				Type:       tc.reqType,
			}
			ans := basicWorker.ProcessRequest(request)

			if ans.Action() != tc.unknownAction {
				t.Errorf("expected: %v \ngot: %v", tc.unknownAction, ans.Action())
			}
		})
	}
}

func TestProcessRequest_KnownAddr_SendsRequestToWorker(t *testing.T) {
	for _, tc := range RequestStateInfo {
		t.Run(fmt.Sprintf("reqType-%v", tc.reqType), func(t *testing.T) {
			serverAddr := "ultraviolet"
			cfg := config.DefaultWorkerConfig()
			basicWorker, _ := newWorker(cfg)
			workerCh := make(chan ultraviolet.BackendRequest)
			worker := ultraviolet.BackendWorker{
				ReqCh: workerCh,
			}
			basicWorker.RegisterBackendWorker(serverAddr, worker)

			request := ultraviolet.BackendRequest{
				ServerAddr: serverAddr,
				Type:       mc.UNKNOWN_STATE,
			}
			go basicWorker.ProcessRequest(request)

			select {
			case receivedReq := <-workerCh:
				if receivedReq.Ch == nil {
					t.Error("received request doesnt have a response channel")
				}
			case <-time.After(defaultChTimeout):
				t.Fatal("worker didnt receive connection")
			}
		})
	}
}

func TestProcessRequest_KnownAddr_ReturnsAnswer(t *testing.T) {
	for _, tc := range RequestStateInfo {
		t.Run(fmt.Sprintf("reqType-%v", tc.reqType), func(t *testing.T) {
			serverAddr := "ultraviolet"
			cfg := config.DefaultWorkerConfig()
			basicWorker, _ := newWorker(cfg)
			workerCh := make(chan ultraviolet.BackendRequest)
			worker := ultraviolet.BackendWorker{
				ReqCh: workerCh,
			}
			basicWorker.RegisterBackendWorker(serverAddr, worker)

			request := ultraviolet.BackendRequest{
				ServerAddr: serverAddr,
				Type:       mc.UNKNOWN_STATE,
			}
			answer := ultraviolet.NewCloseAnswer()
			go func() {
				receivedReq := <-workerCh
				receivedReq.Ch <- answer
			}()

			receivedAns := basicWorker.ProcessRequest(request)
			if receivedAns.Action() != answer.Action() {
				t.Error("received different answer than we expected!")
				t.Logf("expected: %v", answer)
				t.Logf("received: %v", receivedAns)
			}
		})
	}
}

func TestProcessConnection_CanReadHandshake(t *testing.T) {
	for _, tc := range RequestStateInfo {
		t.Run(fmt.Sprintf("reqType-%v", tc.reqType), func(t *testing.T) {
			c1, c2 := net.Pipe()
			cfg := config.DefaultWorkerConfig()
			basicWorker, _ := newWorker(cfg)

			finishedWritingCh := make(chan struct{})
			go func() {
				hsPk := basicHandshakePacket(int(tc.reqType))
				mcConn := mc.NewMcConn(c1)
				mcConn.WritePacket(hsPk)
				finishedWritingCh <- struct{}{}
			}()

			go basicWorker.ProcessConnection(c2)

			select {
			case <-finishedWritingCh:
				t.Log("test has successfully written data to server")
			case <-time.After(defaultChTimeout):
				t.Error("test hasnt finished writing to server in time")
			}
		})
	}
}

func TestProcessConnection_CanReadSecondPacket(t *testing.T) {
	for _, tc := range RequestStateInfo {
		t.Run(fmt.Sprintf("reqType-%v", tc.reqType), func(t *testing.T) {
			c1, c2 := net.Pipe()
			cfg := config.DefaultWorkerConfig()
			basicWorker, _ := newWorker(cfg)

			go basicWorker.ProcessConnection(c2)

			finishedWritingCh := make(chan struct{})
			go func() {
				client := mc.NewMcConn(c1)
				hsPk := basicHandshakePacket(int(tc.reqType))
				client.WritePacket(hsPk)
				var otherPacket mc.Packet
				if tc.reqType == mc.LOGIN {
					otherPacket = basicLoginStartPacket()
				} else if tc.reqType == mc.STATUS {
					otherPacket = basicStatusRequestPacket()
				}
				client.WritePacket(otherPacket)
				finishedWritingCh <- struct{}{}
			}()

			select {
			case <-finishedWritingCh:
				t.Log("test has successfully written data to server")
			case <-time.After(defaultChTimeout):
				t.Error("test hasnt finished writen to server in time")
			}
		})
	}
}

func TestProcessConnection_CanUseServerAddress(t *testing.T) {
	tt := []struct {
		name         string
		serverAddr   string
		expectedAddr string
	}{
		{
			name:         "normale address",
			serverAddr:   "ultraviolet",
			expectedAddr: "ultraviolet",
		},
		{
			name:         "RealIP separator in addr",
			serverAddr:   "ultra///violet",
			expectedAddr: "ultra",
		},
		{
			name:         "Forge separator in addr",
			serverAddr:   "ultra\x00violet",
			expectedAddr: "ultra",
		},
	}

	for _, tc := range tt {
		t.Run(tc.name, func(t *testing.T) {
			c1, c2 := net.Pipe()
			cfg := config.DefaultWorkerConfig()
			basicWorker, _ := newWorker(cfg)
			go func() {
				hsPk := mc.ServerBoundHandshake{
					ProtocolVersion: 751,
					ServerAddress:   tc.serverAddr,
					ServerPort:      25565,
					NextState:       2,
				}.Marshal()
				loginPk := basicLoginStartPacket()

				client := mc.NewMcConn(c1)
				client.WritePacket(hsPk)
				client.WritePacket(loginPk)
			}()

			request, err := basicWorker.ProcessConnection(c2)
			if err != nil {
				t.Fatalf("received error: %v", err)
			}
			if request.ServerAddr != tc.expectedAddr {
				t.Error("got different server addr then what we expected")
				t.Logf("expected: %v", tc.expectedAddr)
				t.Logf("got: %v", request.ServerAddr)
			}

		})
	}
}

func TestProcessConnection_LoginRequest(t *testing.T) {
	clientConn, proxyFrontend := net.Pipe()
	clientAddr := net.TCPAddr{IP: []byte{1, 1, 1, 1}, Port: 0}
	mockClientconn := testNetConn{
		conn:       proxyFrontend,
		remoteAddr: &clientAddr,
	}

	cfg := config.DefaultWorkerConfig()
	basicWorker, _ := newWorker(cfg)

	go func() {
		client := mc.NewMcConn(clientConn)
		hsPk := loginHandshakePacket()
		client.WritePacket(hsPk)
		loginPk := basicLoginStartPacket()
		client.WritePacket(loginPk)
	}()

	ans, err := basicWorker.ProcessConnection(&mockClientconn)
	if err != nil {
		t.Fatalf("didnt expected error but got: %v", err)
	}

	t.Log("test has successfully written data to server")
	t.Log(ans)
	if ans.ServerAddr != "Ultraviolet" {
		t.Errorf("Expected: Ultraviolet got:%v", ans.ServerAddr)
	}
	if ans.Username != "Ultraviolet" {
		t.Errorf("Expected: Ultraviolet got: %v", ans.Username)
	}
	if ans.Addr != &clientAddr {
		t.Errorf("Expected: Ultraviolet got: %v", ans.Addr)
	}
}

func TestProcessConnection_IODeadlines(t *testing.T) {
	t.Run("Handshake", func(t *testing.T) {
		_, proxyFrontend := net.Pipe()
		mockClientconn := testNetConn{
			conn: proxyFrontend,
		}
		deadlineDuration := time.Millisecond
		cfg := config.WorkerConfig{
			IOTimeout: deadlineDuration,
		}
		basicWorker, _ := newWorker(cfg)

		_, err := basicWorker.ProcessConnection(&mockClientconn)
		if !errors.Is(err, ultraviolet.ErrClientToSlow) {
			t.Fatalf("did expect client to slow error but got: %v", err)
		}
	})

	t.Run("Second packet", func(t *testing.T) {
		clientConn, proxyFrontend := net.Pipe()
		mockClientconn := testNetConn{
			conn: proxyFrontend,
		}
		deadlineDuration := time.Millisecond
		cfg := config.WorkerConfig{
			IOTimeout: deadlineDuration,
		}
		basicWorker, _ := newWorker(cfg)
		go func() {
			client := mc.NewMcConn(clientConn)
			hsPk := loginHandshakePacket()
			client.WritePacket(hsPk)
		}()

		_, err := basicWorker.ProcessConnection(&mockClientconn)
		if !errors.Is(err, ultraviolet.ErrClientToSlow) {
			t.Fatalf("did expect client to slow error but got: %v", err)
		}
	})
}

func TestProcessAnswer_IODeadlines(t *testing.T) {
	t.Run("send status", func(t *testing.T) {
		clientConn, proxyFrontend := net.Pipe()
		mockClientconn := testNetConn{
			conn: proxyFrontend,
		}
		deadlineDuration := time.Millisecond
		cfg := config.WorkerConfig{
			IOTimeout: deadlineDuration,
		}
		basicWorker, _ := newWorker(cfg)
		go func() {
			client := mc.NewMcConn(clientConn)
			client.ReadPacket()
		}()
		answer := ultraviolet.NewStatusAnswer(mc.Packet{})
		finishedCh := make(chan struct{})
		go func() {
			basicWorker.ProcessAnswer(&mockClientconn, answer)
			log.Println("finished processing answer")
			finishedCh <- struct{}{}
			log.Println("finished sending signal through channel")
		}()

		select {
		case <-finishedCh:
			t.Log("Process is cancelled")
		case <-time.After(defaultChTimeout):
			t.Error("Process was not cancelled")
		}
	})
}

func TestProcessAnswer_Disconnect(t *testing.T) {
	t.Run("Can send disconnect packet", func(t *testing.T) {
		c1, c2 := net.Pipe()
		cfg := config.DefaultWorkerConfig()
		basicWorker, _ := newWorker(cfg)
		disconPk := mc.ClientBoundDisconnect{
			Reason: "Because we dont want people like you",
		}.Marshal()
		ans := ultraviolet.NewDisconnectAnswer(disconPk)
		go basicWorker.ProcessAnswer(c2, ans)

		client := mc.NewMcConn(c1)
		receivedPk, _ := client.ReadPacket()

		if !samePK(disconPk, receivedPk) {
			t.Error("received different packet than we expected!")
			t.Logf("expected: %v", disconPk)
			t.Logf("received: %v", receivedPk)
		}
	})

	t.Run("connection is closed after its done", func(t *testing.T) {
		c1, c2 := net.Pipe()
		cfg := config.DefaultWorkerConfig()
		basicWorker, _ := newWorker(cfg)
		disconPk := mc.ClientBoundDisconnect{
			Reason: "Because we dont want people like you",
		}.Marshal()
		ans := ultraviolet.NewDisconnectAnswer(disconPk)
		go basicWorker.ProcessAnswer(c2, ans)

		client := mc.NewMcConn(c1)
		client.ReadPacket()

		testPipeConnClosed(t, c1)
	})
}

func TestProcessAnswer_Close(t *testing.T) {
	t.Run("closes connection", func(t *testing.T) {
		c1, c2 := net.Pipe()
		cfg := config.DefaultWorkerConfig()
		basicWorker, _ := newWorker(cfg)
		ans := ultraviolet.NewCloseAnswer()
		go basicWorker.ProcessAnswer(c2, ans)

		client := mc.NewMcConn(c1)
		client.ReadPacket()
		testPipeConnClosed(t, c1)
	})
}

func TestProcessAnswer_Status(t *testing.T) {
	statusPk := mc.SimpleStatus{
		Name:        "Ultraviolet",
		Protocol:    751,
		Description: "Some broken proxy",
	}.Marshal()
	t.Run("can send packet and ping", func(t *testing.T) {
		c1, c2 := net.Pipe()
		cfg := config.DefaultWorkerConfig()
		basicWorker, _ := newWorker(cfg)

		ans := ultraviolet.NewStatusAnswer(statusPk)
		go basicWorker.ProcessAnswer(c2, ans)

		client := mc.NewMcConn(c1)
		receivedPk, _ := client.ReadPacket()
		if !samePK(statusPk, receivedPk) {
			t.Error("received different packet than we expected!")
			t.Logf("expected: %v", statusPk)
			t.Logf("received: %v", receivedPk)
		}

		pingPk := mc.NewServerBoundPing().Marshal()
		client.WritePacket(pingPk)
		pongPk, _ := client.ReadPacket()
		if !samePK(pingPk, pongPk) {
			t.Error("received different packet than we expected!")
			t.Logf("expected: %v", pingPk)
			t.Logf("received: %v", pongPk)
		}
	})

	t.Run("is closed after finishing writing", func(t *testing.T) {
		c1, c2 := net.Pipe()
		cfg := config.DefaultWorkerConfig()
		basicWorker, _ := newWorker(cfg)

		ans := ultraviolet.NewStatusAnswer(statusPk)
		go basicWorker.ProcessAnswer(c2, ans)

		client := mc.NewMcConn(c1)
		client.ReadPacket()
		pingPk := mc.NewServerBoundPing().Marshal()
		client.WritePacket(pingPk)
		client.ReadPacket()

		testPipeConnClosed(t, c1)
	})

	t.Run("latency delays response", func(t *testing.T) {
		c1, c2 := net.Pipe()
		cfg := config.DefaultWorkerConfig()
		basicWorker, _ := newWorker(cfg)
		latency := defaultChTimeout
		ans := ultraviolet.NewStatusLatencyAnswer(statusPk, latency)
		go basicWorker.ProcessAnswer(c2, ans)

		client := mc.NewMcConn(c1)
		client.ReadPacket()
		pingPk := mc.NewServerBoundPing().Marshal()
		client.WritePacket(pingPk)

		receivedPacket := make(chan struct{})

		go func() {
			client.ReadPacket()
			receivedPacket <- struct{}{}
		}()

		select {
		case <-receivedPacket:
			t.Fatal("did receive packet before latency had passed")
		case <-time.After(latency / 10):
			t.Log("didnt receive packet just, exactly what we expected")
		}

		select {
		case <-receivedPacket:
			t.Log("did receive packet")
		case <-time.After(latency * 2):
			t.Error("didnt receive packet")
		}

	})
}

func TestProcessAnswer_Proxy(t *testing.T) {
	for _, tc := range RequestStateInfo {
		t.Run(fmt.Sprintf("reqType-%v", tc.reqType), func(t *testing.T) {
			cfg := config.DefaultWorkerConfig()
			hsPk := mc.ServerBoundHandshake{
				NextState: mc.LoginState,
			}.Marshal()
			var otherPacket mc.Packet
			if tc.reqType == mc.LOGIN {
				otherPacket = basicLoginStartPacket()
			} else if tc.reqType == mc.STATUS {
				otherPacket = basicStatusRequestPacket()
			}

			t.Run("can proxy connection", func(t *testing.T) {
				c1, c2 := net.Pipe()
				s1, s2 := net.Pipe()
				basicWorker, _ := newWorker(cfg)
				proxyCh := make(chan ultraviolet.ProxyAction)
				connFunc := func() (net.Conn, error) {
					return s2, nil
				}
				ans := ultraviolet.NewProxyAnswer(hsPk, otherPacket, proxyCh, connFunc)
				go basicWorker.ProcessAnswer(c2, ans)

				sConn := mc.NewMcConn(s1)
				receivedHsPk, _ := sConn.ReadPacket()
				if !samePK(hsPk, receivedHsPk) {
					t.Error("received different packet than we expected!")
					t.Logf("expected: %v", hsPk)
					t.Logf("received: %v", receivedHsPk)
				}
				receivedOtherPk, _ := sConn.ReadPacket()
				if !samePK(otherPacket, receivedOtherPk) {
					t.Error("received different packet than we expected!")
					t.Logf("expected: %v", otherPacket)
					t.Logf("received: %v", receivedOtherPk)
				}
				<-proxyCh
				testProxyConn(t, c1, s1)
				testProxyConn(t, s1, c1)
			})

			t.Run("proxy channel send signals", func(t *testing.T) {
				c1, c2 := net.Pipe()
				s1, s2 := net.Pipe()
				basicWorker, _ := newWorker(cfg)
				proxyCh := make(chan ultraviolet.ProxyAction)
				connFunc := func() (net.Conn, error) {
					return s2, nil
				}
				ans := ultraviolet.NewProxyAnswer(hsPk, otherPacket, proxyCh, connFunc)
				go basicWorker.ProcessAnswer(c2, ans)

				sConn := mc.NewMcConn(s1)
				sConn.ReadPacket()
				sConn.ReadPacket()

				select {
				case signal := <-proxyCh:
					if signal != ultraviolet.PROXY_OPEN {
						t.Errorf("received %v instead of %v", signal, ultraviolet.PROXY_OPEN)
					}
				case <-time.After(defaultChTimeout):
					t.Fatalf("didnt received proxy start signal")
				}
				c1.Close()
				select {
				case signal := <-proxyCh:
					if signal != ultraviolet.PROXY_CLOSE {
						t.Errorf("received %v instead of %v", signal, ultraviolet.PROXY_CLOSE)
					}
				case <-time.After(defaultChTimeout):
					t.Fatalf("didnt received proxy end signal")
				}
			})
		})
	}

}

// //This test is now broken.
// // func TestReadConnection_WillCloseConn_WhenInvalidPacketSize(t *testing.T) {
// // 	client, server := net.Pipe()
// // 	reqCh := make(chan ultraviolet.McRequest)
// // 	go ultraviolet.ReadConnection(server, reqCh)

// // 	finishedWritingCh := make(chan struct{})
// // 	go func() {
// // 		pkData := make([]byte, 5097160)
// // 		pk := mc.Packet{Data: pkData}
// // 		bytes, _ := pk.Marshal()
// // 		client.Write(bytes)
// // 		finishedWritingCh <- struct{}{}
// // 	}()

// // 	select {
// // 	case <-finishedWritingCh:
// // 		t.Log("test has successfully written data to server")
// // 	case <-time.After(defaultChTimeout):
// // 		t.Error("test hasnt finished writen to server in time")
// // 	}

// // 	testConnectionClosed(t, client)
// // }

func TestProxyConnection(t *testing.T) {
	proxyConns := func(c1, c2 net.Conn) {
		ultraviolet.ProxyConnection(c1, c2)
	}
	testProxy(t, proxyConns)
	testCloseProxy(t, proxyConns)
}

func TestProxy_IOCOPY(t *testing.T) {
	proxyConns := func(c1, c2 net.Conn) {
		ultraviolet.Proxy_IOCopy(c1, c2)
	}
	testProxy(t, proxyConns)
	// testCloseProxy(t, proxyConns) // ERRORS
}

func testProxy(t *testing.T, proxyConns func(net.Conn, net.Conn)) {
	t.Run("Client writes to Server", func(t *testing.T) {
		c1, c2 := net.Pipe()
		s1, s2 := net.Pipe()
		go proxyConns(c2, s2)
		readBuffer := make([]byte, 10)
		couldReachCh := make(chan struct{})
		go func() {
			c1.Write([]byte{1, 2, 3, 4, 5, 6, 7, 8, 9})
			couldReachCh <- struct{}{}
		}()
		go func() {
			s1.Read(readBuffer)
		}()
		select {
		case <-couldReachCh:
		case <-time.After(defaultChTimeout):
			t.Fatal()
		}
	})

	t.Run("Server writes to Client", func(t *testing.T) {
		c1, c2 := net.Pipe()
		s1, s2 := net.Pipe()
		go proxyConns(c2, s2)
		readBuffer := make([]byte, 10)
		couldReachCh := make(chan struct{})
		go func() {
			s1.Write([]byte{1, 2, 3, 4, 5, 6, 7, 8, 9})
			couldReachCh <- struct{}{}
		}()
		go func() {
			c1.Read(readBuffer)
		}()
		select {
		case <-couldReachCh:
		case <-time.After(defaultChTimeout):
			t.Fail()
		}
	})
}

func testCloseProxy(t *testing.T, proxyConns func(net.Conn, net.Conn)) {
	t.Run("Closing client will close server", func(t *testing.T) {
		c1, c2 := net.Pipe()
		s1, s2 := net.Pipe()
		go proxyConns(c2, s2)
		c1.Close()
		time.Sleep(defaultChTimeout)
		testPipeConnClosed(t, s1)
	})

	t.Run("Closing server will close client", func(t *testing.T) {
		c1, c2 := net.Pipe()
		s1, s2 := net.Pipe()
		go proxyConns(c2, s2)
		s1.Close()
		time.Sleep(defaultChTimeout)
		testPipeConnClosed(t, c1)
	})
}
