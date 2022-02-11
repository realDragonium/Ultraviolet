package ultraviolet_test

import (
	"errors"
	"fmt"
	"io"
	"log"
	"net"
	"sync"
	"testing"
	"time"

	"github.com/google/go-cmp/cmp"
	"github.com/pires/go-proxyproto"
	ultraviolet "github.com/realDragonium/Ultraviolet"
	"github.com/realDragonium/Ultraviolet/config"
	"github.com/realDragonium/Ultraviolet/mc"
)

var (
	port     *int16
	portLock sync.Mutex = sync.Mutex{}
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
func StartProxy(cfg config.UltravioletConfig) (string, error) {
	serverAddr := testAddr()
	cfg.ListenTo = serverAddr
	uvReader := testUVReader{
		cfg: cfg,
	}
	serverCfgReader := testServerCfgReader{}
	listener, err := net.Listen("tcp", serverAddr)
	if err != nil {
		return serverAddr, err
	}
	proxy := ultraviolet.NewProxy(uvReader.Read, listener, serverCfgReader.Read)
	go proxy.Start()
	return serverAddr, nil
}

func TestProxyProtocol(t *testing.T) {
	t.SkipNow()
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
			serverAddr, err := StartProxy(cfg)
			if err != nil {
				t.Fatalf("received error: %v", err)
			}
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
			if !cmp.Equal(expectedStatusPacket, pk) {
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

var defaultStatus = mc.SimpleStatus{
	Name:        "uv",
	Protocol:    1,
	Description: "something",
}

var defaultStatusPk = defaultStatus.Marshal()

func TestFlowProxy(t *testing.T) {
	tt := []struct {
		name       string
		handshake  mc.Packet
		secondPk   mc.Packet
		expectedPk mc.Packet
		servers    []config.ServerConfig
	}{
		{
			name:       "no server found - default status",
			handshake:  statusHsPk,
			secondPk:   statusSecondPk,
			expectedPk: defaultStatusPk,
		},
		{
			name:       "server found - status",
			handshake:  statusHsPk,
			secondPk:   statusSecondPk,
			expectedPk: defaultStatusPk,
			servers: []config.ServerConfig{
				{
					Domains: []string{"Ultraviolet"},
					
				},
			},
		},
	}

	for _, tc := range tt {
		t.Run(tc.name, func(t *testing.T) {
			cfg := config.UltravioletConfig{
				IODeadline:    time.Millisecond,
				DefaultStatus: defaultStatus,
				LogOutput:     newTestLogger(t),
			}

			serverAddr, err := StartProxy(cfg)
			if err != nil {
				t.Fatalf("received error: %v", err)
			}
			log.Println(serverAddr)
			conn, err := net.Dial("tcp", serverAddr)
			if err != nil {
				t.Fatalf("received error: %v", err)
			}

			mcConn := mc.NewMcConn(conn)
			err = mcConn.WritePacket(tc.handshake)
			if err != nil {
				t.Fatalf("received error: %v", err)
			}

			err = mcConn.WritePacket(tc.secondPk)
			if err != nil {
				t.Fatalf("received error: %v", err)
			}

			pk, err := mcConn.ReadPacket()
			if err != nil {
				t.Fatalf("didnt expect an error but got: %v", err)
			}

			if !cmp.Equal(tc.expectedPk, pk) {
				expected, _ := mc.UnmarshalClientBoundResponse(tc.expectedPk)
				received, _ := mc.UnmarshalClientBoundResponse(pk)
				t.Errorf("expcted: %#v \ngot: %#v", expected, received)
			}

		})
	}
}
