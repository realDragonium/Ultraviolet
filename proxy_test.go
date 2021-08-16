package ultraviolet_test

import (
	"bytes"
	"errors"
	"fmt"
	"io"
	"net"
	"sync"
	"testing"
	"time"

	"github.com/pires/go-proxyproto"
	ultraviolet "github.com/realDragonium/Ultraviolet"
	"github.com/realDragonium/Ultraviolet/config"
	"github.com/realDragonium/Ultraviolet/mc"
)

var (
	defaultChTimeout = 25 * time.Millisecond
	port             *int16
	portLock         sync.Mutex = sync.Mutex{}
)

func newTestLogger(t *testing.T) io.Writer {
	return &testLogger{
		t: t,
	}
}

type testLogger struct {
	t *testing.T
}

func (logger *testLogger) Write(bb []byte) (int, error) {
	logger.t.Logf("%s", bb)
	return 0, nil
}

// To make sure every test gets its own unique port
func testAddr() string {
	portLock.Lock()
	defer portLock.Unlock()
	if port == nil {
		port = new(int16)
		*port = 26000
	}
	addr := fmt.Sprintf("127.0.0.1:%d", *port)
	*port++
	return addr
}

// Returns address of the server running
func StartProxy(cfg config.UltravioletConfig) string {
	serverAddr := testAddr()
	cfg.ListenTo = serverAddr
	uvReader := testUVReader{
		cfg: cfg,
	}
	serverCfgReader := testServerCfgReader{}
	ultraviolet.StartProxy(&uvReader, &serverCfgReader)
	return serverAddr
}

func samePK(expected, received mc.Packet) bool {
	sameID := expected.ID == received.ID
	sameData := bytes.Equal(expected.Data, received.Data)
	return sameID && sameData
}

func TestProxyProtocol(t *testing.T) {
	tt := []struct {
		acceptProxyProtocol bool
		sendProxyProtocol   bool
		shouldClose         bool
	}{
		{
			acceptProxyProtocol: true,
			sendProxyProtocol:   true,
			shouldClose:         false,
		},
		{
			acceptProxyProtocol: true,
			sendProxyProtocol:   false,
			shouldClose:         true,
		},
		{
			acceptProxyProtocol: false,
			sendProxyProtocol:   true,
			shouldClose:         true,
		},
	}

	for _, tc := range tt {
		name := fmt.Sprintf("accept:%v - send:%v", tc.acceptProxyProtocol, tc.sendProxyProtocol)
		t.Run(name, func(t *testing.T) {
			serverDomain := "Ultraviolet"
			defaultStatus := mc.SimpleStatus{
				Name:        "uv",
				Protocol:    710,
				Description: "something",
			}
			cfg := config.UltravioletConfig{
				NumberOfWorkers:     1,
				NumberOfListeners:   1,
				AcceptProxyProtocol: tc.acceptProxyProtocol,
				IODeadline:          time.Millisecond,
				DefaultStatus:       defaultStatus,
				LogOutput:           newTestLogger(t),
			}
			serverAddr := StartProxy(cfg)
			conn, err := net.Dial("tcp", serverAddr)
			if err != nil {
				t.Fatalf("received error: %v", err)
			}
			if tc.sendProxyProtocol {
				header := &proxyproto.Header{
					Version:           1,
					Command:           proxyproto.PROXY,
					TransportProtocol: proxyproto.TCPv4,
					SourceAddr: &net.TCPAddr{
						IP:   net.ParseIP("10.1.1.1"),
						Port: 1000,
					},
					DestinationAddr: &net.TCPAddr{
						IP:   net.ParseIP("20.2.2.2"),
						Port: 2000,
					},
				}
				_, err = header.WriteTo(conn)
				if err != nil {
					t.Fatalf("received error: %v", err)
				}
			}

			serverConn := mc.NewMcConn(conn)
			handshake := mc.ServerBoundHandshake{
				ServerAddress: serverDomain,
				NextState:     mc.StatusState,
			}.Marshal()

			err = serverConn.WritePacket(handshake)
			if err != nil {
				t.Fatalf("received error: %v", err)
			}
			err = serverConn.WritePacket(mc.Packet{ID: mc.ServerBoundRequestPacketID})
			if err != nil {
				t.Fatalf("received error: %v", err)
			}
			pk, err := serverConn.ReadPacket()
			if tc.shouldClose {
				if errors.Is(err, io.EOF) {
					return
				}
				t.Fatalf("expected an EOF error but got: %v", err)
			}
			if err != nil {
				t.Fatalf("didnt expect an error but got: %v", err)
			}

			expectedStatus := defaultStatus
			expectedStatusPacket := expectedStatus.Marshal()
			if !samePK(expectedStatusPacket, pk) {
				expected, _ := mc.UnmarshalClientBoundResponse(expectedStatusPacket)
				received, _ := mc.UnmarshalClientBoundResponse(pk)
				t.Errorf("expcted: %v \ngot: %v", expected, received)
			}

		})
	}
}

type testServerCfgReader struct {
}

func (reader *testServerCfgReader) Read() ([]config.ServerConfig, error) {
	return nil, nil
}

type testUVReader struct {
	called bool
	cfg    config.UltravioletConfig
}

func (reader *testUVReader) Read() (config.UltravioletConfig, error) {
	reader.called = true
	return reader.cfg, nil
}

func TestStartProxy(t *testing.T) {
	t.Run("reads config from UVReader", func(t *testing.T) {
		cfg := config.UltravioletConfig{
			LogOutput: newTestLogger(t),
		}
		uvReader := &testUVReader{
			cfg: cfg,
		}
		cfgsReader := testServerCfgReader{}
		ultraviolet.StartProxy(uvReader, &cfgsReader)
		if !uvReader.called {
			t.Error("expected config to be read")
		}
	})

	t.Run("listener can accept connections", func(t *testing.T) {
		cfg := config.UltravioletConfig{
			LogOutput:         newTestLogger(t),
			ListenTo:          testAddr(),
			NumberOfListeners: 1,
		}
		uvReader := &testUVReader{
			cfg: cfg,
		}
		cfgsReader := testServerCfgReader{}
		ultraviolet.StartProxy(uvReader, &cfgsReader)

		errCh := make(chan error)
		go func() {
			_, err := net.Dial("tcp", uvReader.cfg.ListenTo)
			errCh <- err
		}()
		var err error
		select {
		case err = <-errCh:
		case <-time.After(defaultChTimeout):
			t.Fatal("timed out")
		}

		if err != nil {
			t.Fatal(err)
		}
		t.Log("connection has been accepted")
	})

	t.Run("listeners passes request through channel", func(t *testing.T) {
		cfg := config.UltravioletConfig{
			LogOutput:         newTestLogger(t),
			ListenTo:          testAddr(),
			NumberOfListeners: 1,
		}
		uvReader := &testUVReader{
			cfg: cfg,
		}
		cfgsReader := testServerCfgReader{}
		ultraviolet.ReqCh = make(chan net.Conn)
		ultraviolet.StartProxy(uvReader, &cfgsReader)

		conn, err := net.Dial("tcp", uvReader.cfg.ListenTo)
		if err != nil {
			t.Fatal(err)
		}
		receivedConn := <-ultraviolet.ReqCh

		data := []byte{1, 1, 1, 1, 1, 1}
		_, err = conn.Write(data)
		if err != nil {
			t.Fatal(err)
		}

		receivedData := make([]byte, len(data))
		_, err = receivedConn.Read(receivedData)
		if err != nil {
			t.Fatal(err)
		}

		if !bytes.Equal(data, receivedData) {
			t.Error("expected the same data entries but")
			t.Logf("sent: %v", data)
			t.Logf("received: %v", receivedData)
		}
	})
}
