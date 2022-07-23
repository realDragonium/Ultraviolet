package ultravioletv2_test

import (
	"bytes"
	"testing"

	"github.com/realDragonium/Ultraviolet/mc"
	ultravioletv2 "github.com/realDragonium/Ultraviolet/src"
)

type MCPacket struct {
	ultravioletv2.MCPacket
	id   byte
	data []byte
}

func TestServerBoundHandshak(t *testing.T) {
	tt := []struct {
		packet          ultravioletv2.ServerBoundHandshakePacket
		marshaledPacket MCPacket
	}{
		{
			packet: ultravioletv2.ServerBoundHandshakePacket{
				ProtocolVersion: 578,
				ServerAddress:   "spook.space",
				ServerPort:      25565,
				NextState:       mc.StatusState,
			},
			marshaledPacket: MCPacket{
				id:   0x00,
				data: []byte{0xC2, 0x04, 0x0B, 0x73, 0x70, 0x6F, 0x6F, 0x6B, 0x2E, 0x73, 0x70, 0x61, 0x63, 0x65, 0x63, 0xDD, 0x01},
			},
		},
		{
			packet: ultravioletv2.ServerBoundHandshakePacket{
				ProtocolVersion: 578,
				ServerAddress:   "example.com",
				ServerPort:      1337,
				NextState:       mc.StatusState,
			},
			marshaledPacket: MCPacket{
				id:   0x00,
				data: []byte{0xC2, 0x04, 0x0B, 0x65, 0x78, 0x61, 0x6D, 0x70, 0x6C, 0x65, 0x2E, 0x63, 0x6F, 0x6D, 0x05, 0x39, 0x01},
			},
		},
	}

	for _, tc := range tt {
		reader := bytes.NewReader(tc.marshaledPacket.data)
		pk, err := ultravioletv2.ReadServerBoundHandshake(reader)
		
		if err != nil {
			t.Errorf("Error reading packet: %v", err)
		}

		if pk.ProtocolVersion != tc.packet.ProtocolVersion {
			t.Errorf("Expected protocol version %d, got %d", tc.packet.ProtocolVersion, pk.ProtocolVersion)
		}

		if pk.ServerAddress != tc.packet.ServerAddress {
			t.Errorf("Expected server address %s, got %s", tc.packet.ServerAddress, pk.ServerAddress)
		}

		if pk.ServerPort != tc.packet.ServerPort {
			t.Errorf("Expected server port %d, got %d", tc.packet.ServerPort, pk.ServerPort)
		}

		if pk.NextState != tc.packet.NextState {
			t.Errorf("Expected next state %d, got %d", tc.packet.NextState, pk.NextState)
		}
	}
}
