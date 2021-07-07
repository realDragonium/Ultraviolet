package mc_test

import (
	"bytes"
	"net"
	"strconv"
	"strings"
	"testing"
	"time"

	"github.com/realDragonium/Ultraviolet/mc"
)

func TestServerBoundHandshake_Marshal(t *testing.T) {
	tt := []struct {
		packet          mc.ServerBoundHandshake
		marshaledPacket mc.Packet
	}{
		{
			packet: mc.ServerBoundHandshake{
				ProtocolVersion: 578,
				ServerAddress:   "spook.space",
				ServerPort:      25565,
				NextState:       mc.HandshakeStatusState,
			},
			marshaledPacket: mc.Packet{
				ID:   0x00,
				Data: []byte{0xC2, 0x04, 0x0B, 0x73, 0x70, 0x6F, 0x6F, 0x6B, 0x2E, 0x73, 0x70, 0x61, 0x63, 0x65, 0x63, 0xDD, 0x01},
			},
		},
		{
			packet: mc.ServerBoundHandshake{
				ProtocolVersion: 578,
				ServerAddress:   "example.com",
				ServerPort:      1337,
				NextState:       mc.HandshakeStatusState,
			},
			marshaledPacket: mc.Packet{
				ID:   0x00,
				Data: []byte{0xC2, 0x04, 0x0B, 0x65, 0x78, 0x61, 0x6D, 0x70, 0x6C, 0x65, 0x2E, 0x63, 0x6F, 0x6D, 0x05, 0x39, 0x01},
			},
		},
	}

	for _, tc := range tt {
		pk := tc.packet.Marshal()

		if pk.ID != mc.ServerBoundHandshakePacketID {
			t.Error("invalid packet id")
		}

		if !bytes.Equal(pk.Data, tc.marshaledPacket.Data) {
			t.Errorf("got: %v, want: %v", pk.Data, tc.marshaledPacket.Data)
		}
	}
}

func TestUnmarshalServerBoundHandshake(t *testing.T) {
	tt := []struct {
		packet             mc.Packet
		unmarshalledPacket mc.ServerBoundHandshake
	}{
		{
			packet: mc.Packet{
				ID: 0x00,
				//           ProtoVer. | Server Address                                                        |Serv. Port | Nxt State
				Data: []byte{0xC2, 0x04, 0x0B, 0x73, 0x70, 0x6F, 0x6F, 0x6B, 0x2E, 0x73, 0x70, 0x61, 0x63, 0x65, 0x63, 0xDD, 0x01},
			},
			unmarshalledPacket: mc.ServerBoundHandshake{
				ProtocolVersion: 578,
				ServerAddress:   "spook.space",
				ServerPort:      25565,
				NextState:       mc.HandshakeStatusState,
			},
		},
		{
			packet: mc.Packet{
				ID: 0x00,
				//           ProtoVer. | Server Address                                                        |Serv. Port | Nxt State
				Data: []byte{0xC2, 0x04, 0x0B, 0x65, 0x78, 0x61, 0x6D, 0x70, 0x6C, 0x65, 0x2E, 0x63, 0x6F, 0x6D, 0x05, 0x39, 0x01},
			},
			unmarshalledPacket: mc.ServerBoundHandshake{
				ProtocolVersion: 578,
				ServerAddress:   "example.com",
				ServerPort:      1337,
				NextState:       mc.HandshakeStatusState,
			},
		},
	}

	for _, tc := range tt {
		actual, err := mc.UnmarshalServerBoundHandshake(tc.packet)
		if err != nil {
			t.Error(err)
		}

		expected := tc.unmarshalledPacket

		if actual.ProtocolVersion != expected.ProtocolVersion ||
			actual.ServerAddress != expected.ServerAddress ||
			actual.ServerPort != expected.ServerPort ||
			actual.NextState != expected.NextState {
			t.Errorf("got: %v, want: %v", actual, tc.unmarshalledPacket)
		}
	}
}

func TestServerBoundHandshake_IsStatusRequest(t *testing.T) {
	tt := []struct {
		handshake mc.ServerBoundHandshake
		result    bool
	}{
		{
			handshake: mc.ServerBoundHandshake{
				NextState: mc.HandshakeStatusState,
			},
			result: true,
		},
		{
			handshake: mc.ServerBoundHandshake{
				NextState: mc.HandshakeLoginState,
			},
			result: false,
		},
	}

	for _, tc := range tt {
		if tc.handshake.IsStatusRequest() != tc.result {
			t.Fail()
		}
	}
}

func TestServerBoundHandshake_IsLoginRequest(t *testing.T) {
	tt := []struct {
		handshake mc.ServerBoundHandshake
		result    bool
	}{
		{
			handshake: mc.ServerBoundHandshake{
				NextState: mc.HandshakeStatusState,
			},
			result: false,
		},
		{
			handshake: mc.ServerBoundHandshake{
				NextState: mc.HandshakeLoginState,
			},
			result: true,
		},
	}

	for _, tc := range tt {
		if tc.handshake.IsLoginRequest() != tc.result {
			t.Fail()
		}
	}
}

func TestServerBoundHandshake_IsForgeAddress(t *testing.T) {
	tt := []struct {
		addr   string
		result bool
	}{
		{
			addr:   mc.ForgeSeparator,
			result: true,
		},
		{
			addr:   "example.com:1234" + mc.ForgeSeparator,
			result: true,
		},
		{
			addr:   "example.com" + mc.ForgeSeparator + "some data",
			result: true,
		},
		{
			addr:   "example.com" + mc.ForgeSeparator + "some data" + mc.RealIPSeparator + "more",
			result: true,
		},
		{
			addr:   "example.com",
			result: false,
		},
		{
			addr:   "",
			result: false,
		},
	}

	for _, tc := range tt {
		hs := mc.ServerBoundHandshake{ServerAddress: mc.String(tc.addr)}
		if hs.IsForgeAddress() != tc.result {
			t.Errorf("%s: got: %v; want: %v", tc.addr, !tc.result, tc.result)
		}
	}
}

func TestServerBoundHandshake_IsRealIPAddress(t *testing.T) {
	tt := []struct {
		addr   string
		result bool
	}{
		{
			addr:   mc.RealIPSeparator,
			result: true,
		},
		{
			addr:   "example.com:25565" + mc.RealIPSeparator,
			result: true,
		},
		{
			addr:   "example.com:1337" + mc.RealIPSeparator + "some data",
			result: true,
		},
		{
			addr:   "example.com" + mc.ForgeSeparator + "some data" + mc.RealIPSeparator + "more",
			result: true,
		},
		{
			addr:   "example.com",
			result: false,
		},
		{
			addr:   ":1234",
			result: false,
		},
	}

	for _, tc := range tt {
		hs := mc.ServerBoundHandshake{ServerAddress: mc.String(tc.addr)}
		if hs.IsRealIPAddress() != tc.result {
			t.Errorf("%s: got: %v; want: %v", tc.addr, !tc.result, tc.result)
		}
	}
}

func TestServerBoundHandshake_ParseServerAddress(t *testing.T) {
	tt := []struct {
		addr         string
		expectedAddr string
	}{
		{
			addr:         "",
			expectedAddr: "",
		},
		{
			addr:         "example.com:25565",
			expectedAddr: "example.com:25565",
		},
		{
			addr:         mc.ForgeSeparator,
			expectedAddr: "",
		},
		{
			addr:         mc.RealIPSeparator,
			expectedAddr: "",
		},
		{
			addr:         "example.com" + mc.ForgeSeparator,
			expectedAddr: "example.com",
		},
		{
			addr:         "example.com" + mc.ForgeSeparator + "some data",
			expectedAddr: "example.com",
		},
		{
			addr:         "example.com:25565" + mc.RealIPSeparator + "some data",
			expectedAddr: "example.com:25565",
		},
		{
			addr:         "example.com:1234" + mc.ForgeSeparator + "some data" + mc.RealIPSeparator + "more",
			expectedAddr: "example.com:1234",
		},
	}

	for _, tc := range tt {
		hs := mc.ServerBoundHandshake{ServerAddress: mc.String(tc.addr)}
		if hs.ParseServerAddress() != tc.expectedAddr {
			t.Errorf("got: %v; want: %v", hs.ParseServerAddress(), tc.expectedAddr)
		}
	}
}

func TestServerBoundHandshake_UpgradeToRealIP(t *testing.T) {
	tt := []struct {
		addr       string
		clientAddr net.TCPAddr
		timestamp  time.Time
	}{
		{
			addr: "example.com",
			clientAddr: net.TCPAddr{
				IP:   net.IPv4(127, 0, 0, 1),
				Port: 12345,
			},
			timestamp: time.Now(),
		},
		{
			addr: "sub.example.com:25565",
			clientAddr: net.TCPAddr{
				IP:   net.IPv4(127, 0, 1, 1),
				Port: 25565,
			},
			timestamp: time.Now(),
		},
		{
			addr: "example.com:25565",
			clientAddr: net.TCPAddr{
				IP:   net.IPv4(127, 0, 2, 1),
				Port: 6543,
			},
			timestamp: time.Now(),
		},
		{
			addr: "example.com",
			clientAddr: net.TCPAddr{
				IP:   net.IPv4(127, 0, 3, 1),
				Port: 7467,
			},
			timestamp: time.Now(),
		},
	}

	for _, tc := range tt {
		hs := mc.ServerBoundHandshake{ServerAddress: mc.String(tc.addr)}
		hs.UpgradeToRealIP(&tc.clientAddr, tc.timestamp)

		if hs.ParseServerAddress() != tc.addr {
			t.Errorf("got: %v; want: %v", hs.ParseServerAddress(), tc.addr)
		}

		realIpSegments := strings.Split(string(hs.ServerAddress), mc.RealIPSeparator)

		if realIpSegments[1] != tc.clientAddr.String() {
			t.Errorf("got: %v; want: %v", realIpSegments[1], tc.addr)
		}

		unixTimestamp, err := strconv.ParseInt(realIpSegments[2], 10, 64)
		if err != nil {
			t.Error(err)
		}

		if unixTimestamp != tc.timestamp.Unix() {
			t.Errorf("timestamp is invalid: got: %d; want: %d", unixTimestamp, tc.timestamp.Unix())
		}
	}
}

func BenchmarkHandshakingServerBoundHandshake_Marshal(b *testing.B) {
	isHandshakePk := mc.ServerBoundHandshake{
		ProtocolVersion: 578,
		ServerAddress:   "spook.space",
		ServerPort:      25565,
		NextState:       1,
	}

	pk := isHandshakePk.Marshal()

	for n := 0; n < b.N; n++ {
		if _, err := mc.UnmarshalServerBoundHandshake(pk); err != nil {
			b.Error(err)
		}
	}
}

func TestMarshalServerBoundLoginStart(t *testing.T) {
	tt := []struct {
		mcName string
	}{
		{
			mcName: "test",
		},
		{
			mcName: "infrared",
		},
		{
			mcName: "",
		},
	}

	for _, tc := range tt {
		t.Run(tc.mcName, func(t *testing.T) {
			expectedPk := mc.Packet{
				ID:   mc.ServerBoundLoginStartPacketID,
				Data: []byte(tc.mcName),
			}

			loginStart := mc.ServerLoginStart{}
			pk := loginStart.Marshal()

			if expectedPk.ID != pk.ID || bytes.Equal(expectedPk.Data, pk.Data) {
				t.Logf("expected:\t%v", expectedPk)
				t.Logf("got:\t\t%v", pk)
				t.Error("Difference be expected and received packet")
			}
		})
	}

}

func TestUnmarshalServerBoundLoginStart(t *testing.T) {
	tt := []struct {
		packet             mc.Packet
		unmarshalledPacket mc.ServerLoginStart
	}{
		{
			packet: mc.Packet{
				ID:   0x00,
				Data: []byte{0x00},
			},
			unmarshalledPacket: mc.ServerLoginStart{
				Name: mc.String(""),
			},
		},
		{
			packet: mc.Packet{
				ID:   0x00,
				Data: []byte{0x0d, 0x48, 0x65, 0x6c, 0x6c, 0x6f, 0x2c, 0x20, 0x57, 0x6f, 0x72, 0x6c, 0x64, 0x21},
			},
			unmarshalledPacket: mc.ServerLoginStart{
				Name: mc.String("Hello, World!"),
			},
		},
	}

	for _, tc := range tt {
		loginStart, err := mc.UnmarshalServerBoundLoginStart(tc.packet)
		if err != nil {
			t.Error(err)
		}

		if loginStart.Name != tc.unmarshalledPacket.Name {
			t.Errorf("got: %v, want: %v", loginStart.Name, tc.unmarshalledPacket.Name)
		}
	}
}

func TestClientBoundDisconnect_Marshal(t *testing.T) {
	tt := []struct {
		packet          mc.ClientBoundDisconnect
		marshaledPacket mc.Packet
	}{
		{
			packet: mc.ClientBoundDisconnect{
				Reason: mc.Chat(""),
			},
			marshaledPacket: mc.Packet{
				ID:   0x00,
				Data: []byte{0x00},
			},
		},
		{
			packet: mc.ClientBoundDisconnect{
				Reason: mc.Chat("Hello, World!"),
			},
			marshaledPacket: mc.Packet{
				ID:   0x00,
				Data: []byte{0x0d, 0x48, 0x65, 0x6c, 0x6c, 0x6f, 0x2c, 0x20, 0x57, 0x6f, 0x72, 0x6c, 0x64, 0x21},
			},
		},
	}

	for _, tc := range tt {
		pk := tc.packet.Marshal()

		if pk.ID != mc.ClientBoundDisconnectPacketID {
			t.Error("invalid packet id")
		}

		if !bytes.Equal(pk.Data, tc.marshaledPacket.Data) {
			t.Errorf("got: %v, want: %v", pk.Data, tc.marshaledPacket.Data)
		}
	}
}

func TestUnmarshalClientDisconnect(t *testing.T) {
	tt := []struct {
		packet             mc.Packet
		unmarshalledPacket mc.ClientBoundDisconnect
	}{
		{
			packet: mc.Packet{
				ID:   0x00,
				Data: []byte{0x00},
			},
			unmarshalledPacket: mc.ClientBoundDisconnect{
				Reason: mc.Chat(""),
			},
		},
		{
			packet: mc.Packet{
				ID:   0x00,
				Data: []byte{0x0d, 0x48, 0x65, 0x6c, 0x6c, 0x6f, 0x2c, 0x20, 0x57, 0x6f, 0x72, 0x6c, 0x64, 0x21},
			},
			unmarshalledPacket: mc.ClientBoundDisconnect{
				Reason: mc.Chat("Hello, World!"),
			},
		},
	}

	for _, tc := range tt {
		disconnectMessage, err := mc.UnmarshalClientDisconnect(tc.packet)
		if err != nil {
			t.Error(err)
		}

		if disconnectMessage.Reason != tc.unmarshalledPacket.Reason {
			t.Errorf("got: %v, want: %v", disconnectMessage.Reason, tc.unmarshalledPacket.Reason)
		}
	}
}
