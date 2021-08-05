package ultraviolet_test

import (
	"fmt"
	"io"
	"net"
	"sync"
	"testing"

	"github.com/pires/go-proxyproto"
	ultraviolet "github.com/realDragonium/Ultraviolet"
	"github.com/realDragonium/Ultraviolet/config"
	"github.com/realDragonium/Ultraviolet/mc"
)

var port *int16
var portLock sync.Mutex = sync.Mutex{}

// To make sure every test gets its own unique port
func testAddr() string {
	portLock.Lock()
	defer portLock.Unlock()
	if port == nil {
		port = new(int16)
		*port = 25000
	}
	addr := fmt.Sprintf("127.0.0.1:%d", *port)
	*port++
	return addr
}

// Returns address of the server running
func StartProxy(cfg config.UltravioletConfig, serverCfgs []config.ServerConfig) string {
	serverAddr := testAddr()
	cfg.ListenTo = serverAddr
	ultraviolet.StartWorkers(cfg, serverCfgs)
	return serverAddr
}

func TestStatusRequest(t *testing.T) {
	serverDomain := "Ultraviolet"
	cfg := config.UltravioletConfig{
		NumberOfWorkers:   1,
		NumberOfListeners: 1,
		EnableHotSwap:     false,
	}
	serverCfgs := []config.ServerConfig{}

	serverAddr := StartProxy(cfg, serverCfgs)
	conn, err := net.Dial("tcp", serverAddr)
	if err != nil {
		t.Fatalf("received error: %v", err)
	}
	serverConn := mc.NewMcConn(conn)
	handshake := mc.ServerBoundHandshake{
		ServerAddress: serverDomain,
	}.Marshal()
	err = serverConn.WritePacket(handshake)
	if err != nil {
		t.Fatalf("received error: %v", err)
	}
	_, err = conn.Read([]byte{0})
	if err != io.EOF {
		t.Fatal(err)
	}
}

//Improve true&true test since we cant see the difference between working or not atm
func TestProxyProtocol(t *testing.T) {
	tt := []struct {
		acceptProxyProtocol bool
		sendProxyProtocol   bool
	}{
		{
			acceptProxyProtocol: true,
			sendProxyProtocol:   true,
		},
		{
			acceptProxyProtocol: true,
			sendProxyProtocol:   false,
		},
		{
			acceptProxyProtocol: false,
			sendProxyProtocol:   true,
		},
	}

	for _, tc := range tt {
		name := fmt.Sprintf("accept:%v - send:%v", tc.acceptProxyProtocol, tc.sendProxyProtocol)
		t.Run(name, func(t *testing.T) {
			serverDomain := "Ultraviolet"
			cfg := config.UltravioletConfig{
				NumberOfWorkers:     1,
				NumberOfListeners:   1,
				AcceptProxyProtocol: tc.acceptProxyProtocol,
				EnableHotSwap:       false,
			}
			serverCfgs := []config.ServerConfig{}

			serverAddr := StartProxy(cfg, serverCfgs)
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
			}.Marshal()

			err = serverConn.WritePacket(handshake)
			if err != nil {
				t.Fatalf("received error: %v", err)
			}

			_, err = conn.Read([]byte{0})
			if err != io.EOF {
				t.Fatal(err)
			}
		})
	}
}
