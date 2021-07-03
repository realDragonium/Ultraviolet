package mc_test

import (
	"bytes"
	"testing"

	"github.com/realDragonium/UltraViolet/mc"
)

func TestClientBoundResponse_Marshal(t *testing.T) {
	tt := []struct {
		packet          mc.ClientBoundResponse
		marshaledPacket mc.Packet
	}{
		{
			packet: mc.ClientBoundResponse{
				JSONResponse: mc.String(""),
			},
			marshaledPacket: mc.Packet{
				ID:   0x00,
				Data: []byte{0x00},
			},
		},
		{
			packet: mc.ClientBoundResponse{
				JSONResponse: mc.String("Hello, World!"),
			},
			marshaledPacket: mc.Packet{
				ID:   0x00,
				Data: []byte{0x0d, 0x48, 0x65, 0x6c, 0x6c, 0x6f, 0x2c, 0x20, 0x57, 0x6f, 0x72, 0x6c, 0x64, 0x21},
			},
		},
	}

	for _, tc := range tt {
		pk := tc.packet.Marshal()

		if pk.ID != mc.ClientBoundResponsePacketID {
			t.Error("invalid packet id")
		}

		if !bytes.Equal(pk.Data, tc.marshaledPacket.Data) {
			t.Errorf("got: %v, want: %v", pk.Data, tc.marshaledPacket.Data)
		}
	}
}

func TestUnmarshalClientBoundResponse(t *testing.T) {
	tt := []struct {
		packet             mc.Packet
		unmarshalledPacket mc.ClientBoundResponse
	}{
		{
			packet: mc.Packet{
				ID:   0x00,
				Data: []byte{0x00},
			},
			unmarshalledPacket: mc.ClientBoundResponse{
				JSONResponse: "",
			},
		},
		{
			packet: mc.Packet{
				ID:   0x00,
				Data: []byte{0x0d, 0x48, 0x65, 0x6c, 0x6c, 0x6f, 0x2c, 0x20, 0x57, 0x6f, 0x72, 0x6c, 0x64, 0x21},
			},
			unmarshalledPacket: mc.ClientBoundResponse{
				JSONResponse: mc.String("Hello, World!"),
			},
		},
	}

	for _, tc := range tt {
		actual, err := mc.UnmarshalClientBoundResponse(tc.packet)
		if err != nil {
			t.Error(err)
		}

		expected := tc.unmarshalledPacket

		if actual.JSONResponse != expected.JSONResponse {
			t.Errorf("got: %v, want: %v", actual, expected)
		}
	}

}

func TestServerBoundRequest_Marshal(t *testing.T) {
	tt := []struct {
		packet          mc.ServerBoundRequest
		marshaledPacket mc.Packet
	}{
		{
			packet: mc.ServerBoundRequest{},
			marshaledPacket: mc.Packet{
				ID:   0x00,
				Data: []byte{},
			},
		},
	}

	for _, tc := range tt {
		pk := tc.packet.Marshal()

		if pk.ID != mc.ServerBoundRequestPacketID {
			t.Error("invalid packet id")
		}
	}
}
