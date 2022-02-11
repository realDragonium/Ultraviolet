package worker_test

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
	"github.com/realDragonium/Ultraviolet/worker"
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

func basicHandshakePacket(state byte) mc.Packet {
	return basicHandshake(state).Marshal()
}

func loginHandshakePacket() mc.Packet {
	return basicHandshake(2).Marshal()
}

func basicHandshake(state byte) mc.ServerBoundHandshake {
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

func newWorker(cfg config.WorkerConfig) (worker.BasicWorker, chan net.Conn) {
	reqCh := make(chan net.Conn)
	r := worker.NewWorker(cfg, reqCh)
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
	wrk, reqCh := newWorker(cfg)
	go wrk.Work()
	c := &net.TCPConn{}
	select {
	case reqCh <- c:
		t.Log("worker has successfully received request")
	case <-time.After(defaultChTimeout):
		t.Error("timed out")
	}
	c.Close()
}

func TestProcessRequest_UnknownAddr(t *testing.T) {
	for _, tc := range RequestStateInfo {
		t.Run(fmt.Sprintf("reqType-%v", tc.reqType), func(t *testing.T) {
			cfg := config.DefaultWorkerConfig()
			basicwrk, _ := newWorker(cfg)

			request := ultraviolet.RequestData{
				ServerAddr: "ultraviolet",
				Type:       tc.reqType,
			}
			ans := basicwrk.ProcessRequest(request)

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
			basicwrk, _ := newWorker(cfg)
			workerCh := make(chan worker.BackendRequest)
			servers := make(map[string]chan<- worker.BackendRequest)
			servers[serverAddr] = workerCh
			basicwrk.SetServers(servers)

			request := ultraviolet.RequestData{
				ServerAddr: serverAddr,
				Type:       mc.UnknownState,
			}
			go basicwrk.ProcessRequest(request)

			select {
			case receivedReq := <-workerCh:
				if receivedReq.Ch == nil {
					t.Error("received request doesnt have a response channel")
				}
				receivedReq.Ch <- worker.BackendAnswer{}
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
			basicwrk, _ := newWorker(cfg)
			workerCh := make(chan worker.BackendRequest)
			servers := make(map[string]chan<- worker.BackendRequest)
			servers[serverAddr] = workerCh
			basicwrk.SetServers(servers)

			request := ultraviolet.RequestData{
				ServerAddr: serverAddr,
				Type:       mc.UnknownState,
			}
			answer := worker.NewCloseAnswer()
			go func() {
				receivedReq := <-workerCh
				receivedReq.Ch <- answer
			}()

			receivedAns := basicwrk.ProcessRequest(request)
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
			basicwrk, _ := newWorker(cfg)

			finishedWritingCh := make(chan struct{})
			go func() {
				hsPk := basicHandshakePacket(byte(tc.reqType))
				mcConn := mc.NewMcConn(c1)
				mcConn.WritePacket(hsPk)
				finishedWritingCh <- struct{}{}
			}()

			go basicwrk.ReadConnection(c2)

			select {
			case <-finishedWritingCh:
				t.Log("test has successfully written data to server")
			case <-time.After(defaultChTimeout):
				t.Error("test hasnt finished writing to server in time")
			}
			c2.Close()
		})
	}
}

func TestProcessConnection_CanReadSecondPacket(t *testing.T) {
	for _, tc := range RequestStateInfo {
		t.Run(fmt.Sprintf("reqType-%v", tc.reqType), func(t *testing.T) {
			c1, c2 := net.Pipe()
			cfg := config.DefaultWorkerConfig()
			basicwrk, _ := newWorker(cfg)

			go basicwrk.ReadConnection(c2)

			finishedWritingCh := make(chan struct{})
			go func() {
				client := mc.NewMcConn(c1)
				hsPk := basicHandshakePacket(byte(tc.reqType))
				client.WritePacket(hsPk)
				var otherPacket mc.Packet
				if tc.reqType == mc.Login {
					otherPacket = basicLoginStartPacket()
				} else if tc.reqType == mc.Status {
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
			basicwrk, _ := newWorker(cfg)
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

			request, err := basicwrk.ReadConnection(c2)
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
	basicwrk, _ := newWorker(cfg)

	go func() {
		client := mc.NewMcConn(clientConn)
		hsPk := loginHandshakePacket()
		client.WritePacket(hsPk)
		loginPk := basicLoginStartPacket()
		client.WritePacket(loginPk)
	}()

	ans, err := basicwrk.ReadConnection(&mockClientconn)
	if err != nil {
		t.Fatalf("didnt expected error but got: %v", err)
	}

	t.Log("test has successfully written data to server")
	t.Log(ans)
	if ans.ServerAddr != "ultraviolet" {
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
		basicwrk, _ := newWorker(cfg)

		_, err := basicwrk.ReadConnection(&mockClientconn)
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
		basicwrk, _ := newWorker(cfg)
		go func() {
			client := mc.NewMcConn(clientConn)
			hsPk := loginHandshakePacket()
			client.WritePacket(hsPk)
		}()

		_, err := basicwrk.ReadConnection(&mockClientconn)
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
		basicwrk, _ := newWorker(cfg)
		go func() {
			client := mc.NewMcConn(clientConn)
			client.ReadPacket()
		}()
		answer := worker.NewStatusAnswer(mc.Packet{})
		finishedCh := make(chan struct{})
		go func() {
			basicwrk.ProcessAnswer(&mockClientconn, answer)
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
		basicwrk, _ := newWorker(cfg)
		disconPk := mc.ClientBoundDisconnect{
			Reason: "Because we dont want people like you",
		}.Marshal()
		ans := worker.NewDisconnectAnswer(disconPk)
		go basicwrk.ProcessAnswer(c2, ans)

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
		basicwrk, _ := newWorker(cfg)
		disconPk := mc.ClientBoundDisconnect{
			Reason: "Because we dont want people like you",
		}.Marshal()
		ans := worker.NewDisconnectAnswer(disconPk)
		go basicwrk.ProcessAnswer(c2, ans)

		client := mc.NewMcConn(c1)
		client.ReadPacket()

		testPipeConnClosed(t, c1)
	})
}

func TestProcessAnswer_Close(t *testing.T) {
	t.Run("closes connection", func(t *testing.T) {
		c1, c2 := net.Pipe()
		cfg := config.DefaultWorkerConfig()
		basicwrk, _ := newWorker(cfg)
		ans := worker.NewCloseAnswer()
		go basicwrk.ProcessAnswer(c2, ans)

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
		basicwrk, _ := newWorker(cfg)

		ans := worker.NewStatusAnswer(statusPk)
		go basicwrk.ProcessAnswer(c2, ans)

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
		basicwrk, _ := newWorker(cfg)

		ans := worker.NewStatusAnswer(statusPk)
		go basicwrk.ProcessAnswer(c2, ans)

		client := mc.NewMcConn(c1)
		client.ReadPacket()
		pingPk := mc.NewServerBoundPing().Marshal()
		client.WritePacket(pingPk)
		client.ReadPacket()

		testPipeConnClosed(t, c1)
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
			if tc.reqType == mc.Login {
				otherPacket = basicLoginStartPacket()
			} else if tc.reqType == mc.Status {
				otherPacket = basicStatusRequestPacket()
			}

			t.Run("can proxy connection", func(t *testing.T) {
				c1, c2 := net.Pipe()
				s1, s2 := net.Pipe()
				basicwrk, _ := newWorker(cfg)
				proxyCh := make(chan worker.ProxyAction)
				connFunc := func() (net.Conn, error) {
					return s2, nil
				}
				ans := worker.NewProxyAnswer(hsPk, otherPacket, proxyCh, connFunc)
				go basicwrk.ProcessAnswer(c2, ans)

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
				basicwrk, _ := newWorker(cfg)
				proxyCh := make(chan worker.ProxyAction)
				connFunc := func() (net.Conn, error) {
					return s2, nil
				}
				ans := worker.NewProxyAnswer(hsPk, otherPacket, proxyCh, connFunc)
				go basicwrk.ProcessAnswer(c2, ans)

				sConn := mc.NewMcConn(s1)
				sConn.ReadPacket()
				sConn.ReadPacket()

				select {
				case signal := <-proxyCh:
					if signal != worker.ProxyOpen {
						t.Errorf("received %v instead of %v", signal, worker.ProxyOpen)
					}
				case <-time.After(defaultChTimeout):
					t.Fatalf("didnt received proxy start signal")
				}
				c1.Close()
				select {
				case signal := <-proxyCh:
					if signal != worker.ProxyClose {
						t.Errorf("received %v instead of %v", signal, worker.ProxyClose)
					}
				case <-time.After(defaultChTimeout):
					t.Fatalf("didnt received proxy end signal")
				}
			})
		})
	}

}

func TestProxyConnection(t *testing.T) {
	proxyConns := func(c1, c2 net.Conn) {
		worker.ProxyConnection(c1, c2)
	}
	testProxy(t, proxyConns)
	testCloseProxy(t, proxyConns)
}

func TestProxy_IOCOPY(t *testing.T) {
	proxyConns := func(c1, c2 net.Conn) {
		worker.Proxy_IOCopy(c1, c2)
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
		c1.Close()
		s1.Close()
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
		c1.Close()
		s1.Close()
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
