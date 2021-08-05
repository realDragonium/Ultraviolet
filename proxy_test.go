package ultraviolet_test

import (
	"fmt"
	"sync"
	"testing"

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
	return serverAddr
}

func TestStatusRequest(t *testing.T) {
	t.SkipNow()
	serverDomain := "Ultraviolet"
	cfg := config.UltravioletConfig{
		NumberOfWorkers:   1,
		NumberOfListeners: 1,
	}
	serverCfgs := []config.ServerConfig{
		{
			Domains: []string{serverDomain},
		},
	}

	serverAddr := StartProxy(cfg, serverCfgs)

	serverConn, err := mc.DialMCServer(serverAddr)
	if err != nil {
		t.Fatalf("received error: %v", err)
	}

	handshake := mc.ServerBoundHandshake{
		ProtocolVersion: 10,
		ServerAddress:   serverDomain,
		NextState:       mc.StatusState,
	}
	serverConn.WriteMcPacket(&handshake)
	statusRequest := mc.ServerBoundRequest{}
	serverConn.WriteMcPacket(&statusRequest)

}
