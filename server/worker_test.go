package server_test

import (
	"bufio"
	"bytes"
	"errors"
	"fmt"
	"io"
	"net"
	"testing"
	"time"

	"github.com/realDragonium/Ultraviolet/config"
	"github.com/realDragonium/Ultraviolet/mc"
	"github.com/realDragonium/Ultraviolet/server"
)

func newTestMcConn(conn net.Conn) testMcConn {
	return testMcConn{
		netConn: conn,
		reader:  bufio.NewReader(conn),
	}
}

type testMcConn struct {
	netConn net.Conn
	reader  mc.DecodeReader
}

func (conn testMcConn) ReadPacket() (mc.Packet, error) {
	pk, err := mc.ReadPacket(conn.reader)
	return pk, err
}

func (conn testMcConn) WritePacket(p mc.Packet) error {
	pk := p.Marshal()
	_, err := conn.netConn.Write(pk)
	return err
}

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

func startWorker(cfg config.WorkerConfig) chan net.Conn {
	r := server.NewWorker(cfg)
	go func() {
		r.Work()
	}()
	return r.ReqCh
}

func testConnectionClosed(t *testing.T, conn net.Conn) {
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
	cfg := config.WorkerConfig{}
	reqCh := startWorker(cfg)
	select {
	case reqCh <- &net.TCPConn{}:
		t.Log("worker has successfully received request")
	case <-time.After(defaultChTimeout):
		t.Error("timed out")
	}
}

func TestProcessRequest_UnknownAddr(t *testing.T) {
	for _, tc := range LoginStatusTestCases {
		t.Run(fmt.Sprintf("reqType-%v", tc.reqType), func(t *testing.T) {
			cfg := config.WorkerConfig{}
			r := server.NewWorker(cfg)

			request := server.BackendRequest{
				ServerAddr: "ultraviolet",
				Type:       tc.reqType,
			}
			ans := r.ProcessRequest(request)

			if ans.Action() != tc.unknownAction {
				t.Errorf("expected: %v \ngot: %v", tc.unknownAction, ans.Action())
			}
		})
	}
}

func TestProcessRequest_KnownAddr_SendsRequestToWorker(t *testing.T) {
	for _, tc := range LoginStatusTestCases {
		t.Run(fmt.Sprintf("reqType-%v", tc.reqType), func(t *testing.T) {
			serverAddr := "ultraviolet"
			cfg := config.WorkerConfig{}
			r := server.NewWorker(cfg)
			workerCh := make(chan server.BackendRequest)
			worker := server.BackendWorker{
				ReqCh: workerCh,
			}
			r.RegisterWorker(serverAddr, worker)

			request := server.BackendRequest{
				ServerAddr: serverAddr,
				Type:       mc.UNKNOWN_STATE,
			}
			go r.ProcessRequest(request)

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
	for _, tc := range LoginStatusTestCases {
		t.Run(fmt.Sprintf("reqType-%v", tc.reqType), func(t *testing.T) {
			serverAddr := "ultraviolet"
			cfg := config.WorkerConfig{}
			r := server.NewWorker(cfg)
			workerCh := make(chan server.BackendRequest)
			worker := server.BackendWorker{
				ReqCh: workerCh,
			}
			r.RegisterWorker(serverAddr, worker)

			request := server.BackendRequest{
				ServerAddr: serverAddr,
				Type:       mc.UNKNOWN_STATE,
			}
			answer := server.NewCloseAnswer()
			go func() {
				receivedReq := <-workerCh
				receivedReq.Ch <- answer
			}()

			receivedAns := r.ProcessRequest(request)
			if receivedAns.Action() != answer.Action() {
				t.Error("received different answer than we expected!")
				t.Logf("expected: %v", answer)
				t.Logf("received: %v", receivedAns)
			}
		})
	}
}

func TestProcessConnection_CanReadHandshake(t *testing.T) {
	for _, tc := range LoginStatusTestCases {
		t.Run(fmt.Sprintf("reqType-%v", tc.reqType), func(t *testing.T) {
			c1, c2 := net.Pipe()
			cfg := config.WorkerConfig{}
			r := server.NewWorker(cfg)

			finishedWritingCh := make(chan struct{})
			go func() {
				hsPk := basicHandshakePacket(int(tc.reqType))
				mcConn := newTestMcConn(c1)
				mcConn.WritePacket(hsPk)
				finishedWritingCh <- struct{}{}
			}()

			go r.ProcessConnection(c2)

			select {
			case <-finishedWritingCh:
				t.Log("test has successfully written data to server")
			case <-time.After(defaultChTimeout):
				t.Error("test hasnt finished writen to server in time")
			}
		})
	}
}

func TestProcessConnection_CanReadSecondPacket(t *testing.T) {
	for _, tc := range LoginStatusTestCases {
		t.Run(fmt.Sprintf("reqType-%v", tc.reqType), func(t *testing.T) {
			c1, c2 := net.Pipe()
			cfg := config.WorkerConfig{}
			r := server.NewWorker(cfg)

			go r.ProcessConnection(c2)

			finishedWritingCh := make(chan struct{})
			go func() {
				client := newTestMcConn(c1)
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
			cfg := config.WorkerConfig{}
			r := server.NewWorker(cfg)
			go func() {
				hsPk := mc.ServerBoundHandshake{
					ProtocolVersion: 751,
					ServerAddress:   tc.serverAddr,
					ServerPort:      25565,
					NextState:       2,
				}.Marshal()
				loginPk := basicLoginStartPacket()

				client := newTestMcConn(c1)
				client.WritePacket(hsPk)
				client.WritePacket(loginPk)
			}()

			request, err := r.ProcessConnection(c2)
			if err != nil {
				t.Fatalf("received error: %v", err)
			}
			if request.ServerAddr != tc.expectedAddr {
				t.Errorf("expected:\t %v, \ngot:\t %v", tc.expectedAddr, request.ServerAddr)
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

	cfg := config.WorkerConfig{}
	r := server.NewWorker(cfg)

	go func() {
		client := newTestMcConn(clientConn)
		hsPk := loginHandshakePacket()
		client.WritePacket(hsPk)
		loginPk := basicLoginStartPacket()
		client.WritePacket(loginPk)
	}()

	ans, err := r.ProcessConnection(&mockClientconn)
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

func TestProcessAnswer_Disconnect(t *testing.T) {
	t.Run("Can send disconnect packet", func(t *testing.T) {
		c1, c2 := net.Pipe()
		cfg := config.WorkerConfig{}
		r := server.NewWorker(cfg)
		disconPk := mc.ClientBoundDisconnect{
			Reason: "Because we dont want people like you",
		}.Marshal()
		disconBytes := disconPk.Marshal()
		ans := server.NewDisconnectAnswer(disconBytes)
		go r.ProcessAnswer(c2, ans)

		client := newTestMcConn(c1)
		receivedPk, _ := client.ReadPacket()

		if !samePK(disconPk, receivedPk) {
			t.Error("received different packet than we expected!")
			t.Logf("expected: %v", disconPk)
			t.Logf("received: %v", receivedPk)
		}
	})

	t.Run("connection is closed after its done", func(t *testing.T) {
		c1, c2 := net.Pipe()
		cfg := config.WorkerConfig{}
		r := server.NewWorker(cfg)
		disconPk := mc.ClientBoundDisconnect{
			Reason: "Because we dont want people like you",
		}.Marshal()
		disconBytes := disconPk.Marshal()
		ans := server.NewDisconnectAnswer(disconBytes)
		go r.ProcessAnswer(c2, ans)

		client := newTestMcConn(c1)
		client.ReadPacket()

		testConnectionClosed(t, c1)
	})
}

func TestProcessAnswer_Close(t *testing.T) {
	t.Run("closes connection", func(t *testing.T) {
		c1, c2 := net.Pipe()
		cfg := config.WorkerConfig{}
		r := server.NewWorker(cfg)
		ans := server.NewCloseAnswer()
		go r.ProcessAnswer(c2, ans)

		client := newTestMcConn(c1)
		client.ReadPacket()
		testConnectionClosed(t, c1)
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
		cfg := config.WorkerConfig{}
		r := server.NewWorker(cfg)
		statusBytes := statusPk.Marshal()

		ans := server.NewStatusAnswer(statusBytes)
		go r.ProcessAnswer(c2, ans)

		client := newTestMcConn(c1)
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
		cfg := config.WorkerConfig{}
		r := server.NewWorker(cfg)
		statusBytes := statusPk.Marshal()

		ans := server.NewStatusAnswer(statusBytes)
		go r.ProcessAnswer(c2, ans)

		client := newTestMcConn(c1)
		client.ReadPacket()
		pingPk := mc.NewServerBoundPing().Marshal()
		client.WritePacket(pingPk)
		client.ReadPacket()

		testConnectionClosed(t, c1)
	})

	t.Run("latency delays response", func(t *testing.T) {
		c1, c2 := net.Pipe()
		cfg := config.WorkerConfig{}
		r := server.NewWorker(cfg)
		statusBytes := statusPk.Marshal()
		latency := defaultChTimeout
		ans := server.NewStatusLatencyAnswer(statusBytes, latency)
		go r.ProcessAnswer(c2, ans)

		client := newTestMcConn(c1)
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
			t.Fatal("did receive packet before latence had passed")
		case <-time.After(latency - time.Millisecond):
			t.Log("didnt receive packet as expected")
		}

		select {
		case <-receivedPacket:
			t.Log("did receive packet")
		case <-time.After(latency - time.Millisecond):
			t.Error("didnt receive packet")
		}

	})
}

func TestProcessAnswer_Proxy(t *testing.T) {
	for _, tc := range LoginStatusTestCases {
		t.Run(fmt.Sprintf("reqType-%v", tc.reqType), func(t *testing.T) {
			cfg := config.WorkerConfig{}
			hsPk := mc.ServerBoundHandshake{
				NextState: mc.LoginState,
			}.Marshal()
			var otherPacket mc.Packet
			if tc.reqType == mc.LOGIN {
				otherPacket = basicLoginStartPacket()
			} else if tc.reqType == mc.STATUS {
				otherPacket = basicStatusRequestPacket()
			}
			hsBytes := hsPk.Marshal()
			otherPkBytes := otherPacket.Marshal()

			t.Run("can proxy connection", func(t *testing.T) {
				c1, c2 := net.Pipe()
				s1, s2 := net.Pipe()
				r := server.NewWorker(cfg)
				proxyCh := make(chan server.ProxyAction)
				connFunc := func() (net.Conn, error) {
					return s2, nil
				}
				ans := server.NewProxyAnswer(hsBytes, otherPkBytes, proxyCh, connFunc)
				go r.ProcessAnswer(c2, ans)

				sConn := newTestMcConn(s1)
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
				r := server.NewWorker(cfg)
				proxyCh := make(chan server.ProxyAction)
				connFunc := func() (net.Conn, error) {
					return s2, nil
				}
				ans := server.NewProxyAnswer(hsBytes, otherPkBytes, proxyCh, connFunc)
				go r.ProcessAnswer(c2, ans)

				sConn := newTestMcConn(s1)
				sConn.ReadPacket()
				sConn.ReadPacket()

				select {
				case signal := <-proxyCh:
					if signal != server.PROXY_OPEN {
						t.Errorf("received %v instead of %v", signal, server.PROXY_OPEN)
					}
				case <-time.After(defaultChTimeout):
					t.Fatalf("didnt received proxy start signal")
				}
				c1.Close()
				select {
				case signal := <-proxyCh:
					if signal != server.PROXY_CLOSE {
						t.Errorf("received %v instead of %v", signal, server.PROXY_CLOSE)
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
// // 	reqCh := make(chan server.McRequest)
// // 	go server.ReadConnection(server, reqCh)

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
		server.ProxyConnection(c1, c2)
	}
	testProxy(t, proxyConns)
	testCloseProxy(t, proxyConns)
}

func TestProxy_IOCOPY(t *testing.T) {
	proxyConns := func(c1, c2 net.Conn) {
		server.Proxy_IOCopy(c1, c2)
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
		testConnectionClosed(t, s1)
	})

	t.Run("Closing server will close client", func(t *testing.T) {
		c1, c2 := net.Pipe()
		s1, s2 := net.Pipe()
		go proxyConns(c2, s2)
		s1.Close()
		time.Sleep(defaultChTimeout)
		testConnectionClosed(t, c1)
	})
}
