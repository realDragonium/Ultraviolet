package ultravioletv2_test

import (
	"bytes"
	"net"
	"testing"
	"time"

	"github.com/google/go-cmp/cmp"
	"github.com/realDragonium/Ultraviolet/config"
	"github.com/realDragonium/Ultraviolet/core"
	ultravioletv2 "github.com/realDragonium/Ultraviolet/src"
)

// timestamp returns a timestamp in milliseconds.
func timestamp() int64 {
	return time.Now().UnixNano() / int64(time.Second)
}

func TestConnectToServer(t *testing.T) {
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

func TestFullProxyConnection(t *testing.T) {

}

func TestCreateListener(t *testing.T) {
	cfg := config.UltravioletConfig{
		ListenTo: ":0",
	}

	ln, err := ultravioletv2.CreateListener(cfg)
	if err != nil {
		t.Fatalf("Error during creating listener: %v", err)
	}

	go func() {
		net.Dial("tcp", ln.Addr().String())
	}()

	conn, err := ln.Accept()
	if err != nil {
		t.Fatalf("Error during accepting connection: %v", err)
	}

	if conn == nil {
		t.Fatal("Expected connection, got nil")
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

func TestBedrockServerListener(t *testing.T) {
	config := ultravioletv2.BedrockServerConfig{
		BaseConfig: ultravioletv2.BaseConfig{
			ListenTo: ":0",
			ProxyTo:  "",
		},
	}

	ln, err := ultravioletv2.CreateBedrockListener(config)
	if err != nil {
		t.Errorf("Error creating listener: %v", err)
	}

	if ln == nil {
		t.Error("Expected listener, got nil")
	}

}

func TestBedrockServer(t *testing.T) {
	config := ultravioletv2.BedrockServerConfig{
		BaseConfig: ultravioletv2.BaseConfig{
			ListenTo: ":0",
			ProxyTo:  "",
		},
		ID: 23894692837498,
		ServerStatus: ultravioletv2.BedrockStatus{
			Edition:     "MCPE",
			Description: ultravioletv2.Description{Text: "This Server - UV"},
			Version: ultravioletv2.Version{
				Name:     "1.19.10",
				Protocol: 534,
			},
			Players: ultravioletv2.Players{
				Online: 0,
				Max:    100,
			},
			Gamemode: ultravioletv2.GameMode{
				Name: "Survival",
				ID:   1,
			},
			Port: ultravioletv2.Port{
				IPv4: 19132,
				IPv6: -1,
			},
		},
	}

	ln, err := ultravioletv2.CreateBedrockListener(config)
	if err != nil {
		t.Errorf("Error creating listener: %v", err)
	}

	go ultravioletv2.StartBedrockServer(ln, config)

	pingPk := ultravioletv2.UnconnectedPing{
		ClientGUID:    23794873294,
		Magic:         ultravioletv2.UnconnectedMessageSequence,
		SendTimestamp: timestamp(),
	}

	conn, err := net.Dial("udp", ln.LocalAddr().String())
	if err != nil {
		t.Errorf("Error dialing server: %v", err)
	}

	err = pingPk.Write(conn)
	if err != nil {
		t.Errorf("Error writing packet: %v", err)
	}

	readBytes := make([]byte, 2048)
	n, err := conn.Read(readBytes)
	if err != nil {
		t.Errorf("Error reading packet: %v", err)
	}

	buf := bytes.NewReader(readBytes[:n])
	buf.ReadByte() // Skip packet id
	pongPk := ultravioletv2.UnconnectedPong{}
	pongPk.Read(buf)

	if pongPk.Magic != ultravioletv2.UnconnectedMessageSequence {
		t.Errorf("Expected magic: %d, got %d", ultravioletv2.UnconnectedMessageSequence, pongPk.Magic)
	}

	if pongPk.ServerGUID != config.ID {
		t.Errorf("Expected server GUID: %d, got %d", config.ID, pongPk.ServerGUID)
	}

	if pongPk.SendTimestamp != pingPk.SendTimestamp {
		t.Errorf("Expected send timestamp: %d, got %d", pingPk.SendTimestamp, pongPk.SendTimestamp)
	}

	if pongPk.Data != config.Status() {
		t.Errorf("Expected status: %s, got %s", config.Status(), pongPk.Data)
	}

}
