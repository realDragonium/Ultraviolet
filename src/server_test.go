package ultravioletv2_test

import (
	"bytes"
	"net"
	"testing"

	"github.com/google/go-cmp/cmp"
	"github.com/realDragonium/Ultraviolet/core"
	ultravioletv2 "github.com/realDragonium/Ultraviolet/src"
)

func TestProcessConnection(t *testing.T) {
	listener, err := net.Listen("tcp", "")
	if err != nil {
		t.Fatalf("Error during creating listener: %v", err)
	}

	ultravioletv2.Servers = map[string]string{
		"localhost": listener.Addr().String(),
	}

	pk := ultravioletv2.ServerBoundHandshakePacket{
		ServerAddress: "localhost",
	}

	go ultravioletv2.ConnectToServer(pk)

	conn, err := listener.Accept()
	if err != nil {
		t.Fatalf("Error during accepting connection: %v", err)
	}

	n, data, err := ultravioletv2.ReadPacketData(conn)

	if err != nil {
		t.Fatalf("Error during reading packet data: %v", err)
	}

	if n != len(data) {
		t.Errorf("Expected packet length: %d, got %d", len(data), n)
	}

	r := bytes.NewReader(data)
	receivedPk, _ := ultravioletv2.ReadServerBoundHandshake(r)

	if !cmp.Equal(pk, receivedPk) {
		t.Errorf("Expected packet: %#v, got %#v", pk, receivedPk)
	}
}

func TestServerAddress(t *testing.T) {
	tt := []struct {
		address           string
		registeredAddress string
		proxyToAddress    string
		expectedError     error
	}{
		{
			address:           "localhost",
			registeredAddress: "localhost",
			proxyToAddress:    "localhost:25566",
		},
		{
			address:           "local",
			registeredAddress: "localhost",
			expectedError:     core.ErrNoServerFound,
		},
	}

	for _, tc := range tt {
		ultravioletv2.Servers = map[string]string{
			tc.registeredAddress: tc.proxyToAddress,
		}

		pk := ultravioletv2.ServerBoundHandshakePacket{
			ProtocolVersion: 0,
			ServerAddress:   tc.address,
			ServerPort:      0,
			NextState:       0,
		}

		serverAddr, err := ultravioletv2.ServerAddress(pk)

		if err != nil {
			if tc.expectedError == nil {
				t.Errorf("Error getting server address: %v", err)
				continue
			}

			if err != tc.expectedError {
				t.Errorf("Expected error %v, got %v", tc.expectedError, err)
			}
		}

		if err != tc.expectedError {
			t.Fatalf("Expected error %v, got %v", tc.expectedError, err)
		}

		if err == nil && serverAddr != tc.proxyToAddress {
			t.Errorf("Expected server address: '%s', got '%s'", tc.proxyToAddress, serverAddr)
		}

	}

}
