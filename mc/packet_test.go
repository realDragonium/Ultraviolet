package mc_test

import (
	"bufio"
	"bytes"
	"net"
	"testing"

	"github.com/realDragonium/UltraViolet/mc"
)

func TestPacket_Marshal(t *testing.T) {
	tt := []struct {
		packet   mc.Packet
		expected []byte
	}{
		{
			packet: mc.Packet{
				ID:   0x00,
				Data: []byte{0x00, 0xf2},
			},
			expected: []byte{0x03, 0x00, 0x00, 0xf2},
		},
		{
			packet: mc.Packet{
				ID:   0x0f,
				Data: []byte{0x00, 0xf2, 0x03, 0x50},
			},
			expected: []byte{0x05, 0x0f, 0x00, 0xf2, 0x03, 0x50},
		},
	}

	for _, tc := range tt {
		actual, err := tc.packet.Marshal()
		if err != nil {
			t.Error(err)
		}

		if !bytes.Equal(actual, tc.expected) {
			t.Errorf("got: %v; want: %v", actual, tc.expected)
		}
	}
}

func TestPacket_Scan(t *testing.T) {
	packet := mc.Packet{
		ID:   0x00,
		Data: []byte{0xf2},
	}

	var byteField mc.Byte

	err := packet.Scan(
		&byteField,
	)

	if err != nil {
		t.Error(err)
	}

	if !bytes.Equal(byteField.Encode(), []byte{0xf2}) {
		t.Errorf("got: %x; want: %x", byteField.Encode(), 0xf2)
	}
}

func TestScanFields(t *testing.T) {
	packet := mc.Packet{
		ID:   0x00,
		Data: []byte{0xf2},
	}

	var byteField mc.Byte

	err := mc.ScanFields(
		bytes.NewReader(packet.Data),
		&byteField,
	)

	if err != nil {
		t.Error(err)
	}

	if !bytes.Equal(byteField.Encode(), []byte{0xf2}) {
		t.Errorf("got: %x; want: %x", byteField.Encode(), 0xf2)
	}
}

func TestMarshalPacket(t *testing.T) {
	packetId := byte(0x00)
	byteField := mc.Byte(0x0f)
	packetData := []byte{0x0f}

	packet := mc.MarshalPacket(packetId, byteField)

	if packet.ID != packetId {
		t.Errorf("packet id: got: %v; want: %v", packet.ID, packetId)
	}

	if !bytes.Equal(packet.Data, packetData) {
		t.Errorf("got: %v; want: %v", packet.Data, packetData)
	}
}

func TestReadPacketBytes(t *testing.T) {
	tt := []struct {
		data        []byte
		packetBytes []byte
	}{
		{
			data:        []byte{0x03, 0x00, 0x00, 0xf2, 0x05, 0x0f, 0x00, 0xf2, 0x03, 0x50},
			packetBytes: []byte{0x00, 0x00, 0xf2},
		},
		{
			data:        []byte{0x05, 0x0f, 0x00, 0xf2, 0x03, 0x50, 0x30, 0x01, 0xef, 0xaa},
			packetBytes: []byte{0x0f, 0x00, 0xf2, 0x03, 0x50},
		},
	}

	for _, tc := range tt {
		readBytes, err := mc.ReadPacketBytes(bytes.NewReader(tc.data))
		if err != nil {
			t.Error(err)
		}

		if !bytes.Equal(readBytes, tc.packetBytes) {
			t.Errorf("got: %v; want: %v", readBytes, tc.packetBytes)
		}
	}
}

func TestReadPacket(t *testing.T) {
	tt := []struct {
		data          []byte
		packet        mc.Packet
		dataAfterRead []byte
	}{
		{
			data: []byte{0x03, 0x00, 0x00, 0xf2, 0x05, 0x0f, 0x00, 0xf2, 0x03, 0x50},
			packet: mc.Packet{
				ID:   0x00,
				Data: []byte{0x00, 0xf2},
			},
			dataAfterRead: []byte{0x05, 0x0f, 0x00, 0xf2, 0x03, 0x50},
		},
		{
			data: []byte{0x05, 0x0f, 0x00, 0xf2, 0x03, 0x50, 0x30, 0x01, 0xef, 0xaa},
			packet: mc.Packet{
				ID:   0x0f,
				Data: []byte{0x00, 0xf2, 0x03, 0x50},
			},
			dataAfterRead: []byte{0x30, 0x01, 0xef, 0xaa},
		},
	}

	for _, tc := range tt {
		buf := bytes.NewBuffer(tc.data)
		pk, err := mc.ReadPacketOld(buf)
		if err != nil {
			t.Error(err)
		}

		if pk.ID != tc.packet.ID {
			t.Errorf("packet ID: got: %v; want: %v", pk.ID, tc.packet.ID)
		}

		if !bytes.Equal(pk.Data, tc.packet.Data) {
			t.Errorf("packet data: got: %v; want: %v", pk.Data, tc.packet.Data)
		}

		if !bytes.Equal(buf.Bytes(), tc.dataAfterRead) {
			t.Errorf("data after read: got: %v; want: %v", tc.data, tc.dataAfterRead)
		}
	}
}

func benchmarkReadPacker(b *testing.B, amountBytes int) {
	data := []byte{}

	for i := 0; i < amountBytes; i++ {
		data = append(data, 1)
	}
	pk := mc.Packet{ID: 0x05, Data: data}
	bytes, _ := pk.Marshal()
	c1, c2 := net.Pipe()
	r := bufio.NewReader(c1)

	go func() {
		for {
			c2.Write(bytes)
		}
	}()

	for n := 0; n < b.N; n++ {
		if _, err := mc.ReadPacketOld(r); err != nil {
			b.Error(err)
		}
	}

}

func BenchmarkReadPacker_SingleByteVarInt(b *testing.B) {
	size := 0b0101111
	benchmarkReadPacker(b, size)
}

func BenchmarkReadPacker_DoubleByteVarInt(b *testing.B) {
	size := 0b1111111_0101111
	benchmarkReadPacker(b, size)
}

func BenchmarkReadPacker_TripleByteVarInt(b *testing.B) {
	size := 0b1111111_1111111_0101111
	benchmarkReadPacker(b, size)
}
