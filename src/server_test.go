package ultravioletv2_test

import (
	"testing"

	"github.com/realDragonium/Ultraviolet/core"
	ultravioletv2 "github.com/realDragonium/Ultraviolet/src"
)

func TestProcessConnection(t *testing.T) {

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
