package mc_test

import (
	"bytes"
	"errors"
	"fmt"
	"testing"

	"github.com/realDragonium/UltraViolet/mc"
)

func TestReadNBytes(t *testing.T) {
	tt := [][]byte{
		{0x00, 0x01, 0x02, 0x03},
		{0x03, 0x01, 0x02, 0x02},
	}

	for _, tc := range tt {
		bb, err := mc.ReadNBytes(bytes.NewBuffer(tc), len(tc))
		if err != nil {
			t.Errorf("reading bytes: %s", err)
		}

		if !bytes.Equal(bb, tc) {
			t.Errorf("got %v; want: %v", bb, tc)
		}
	}
}

func TestVarInt(t *testing.T) {
	tt := []struct {
		decoded mc.VarInt
		encoded []byte
	}{
		{
			decoded: mc.VarInt(0),
			encoded: []byte{0x00},
		},
		{
			decoded: mc.VarInt(1),
			encoded: []byte{0x01},
		},
		{
			decoded: mc.VarInt(2),
			encoded: []byte{0x02},
		},
		{
			decoded: mc.VarInt(127),
			encoded: []byte{0x7f},
		},
		{
			decoded: mc.VarInt(128),
			encoded: []byte{0x80, 0x01},
		},
		{
			decoded: mc.VarInt(255),
			encoded: []byte{0xff, 0x01},
		},
		{
			decoded: mc.VarInt(2097151),
			encoded: []byte{0xff, 0xff, 0x7f},
		},
		{
			decoded: mc.VarInt(2147483647),
			encoded: []byte{0xff, 0xff, 0xff, 0xff, 0x07},
		},
		{
			decoded: mc.VarInt(-1),
			encoded: []byte{0xff, 0xff, 0xff, 0xff, 0x0f},
		},
		{
			decoded: mc.VarInt(-2147483648),
			encoded: []byte{0x80, 0x80, 0x80, 0x80, 0x08},
		},
	}

	t.Run("encode", func(t *testing.T) {
		for _, tc := range tt {
			if !bytes.Equal(tc.decoded.Encode(), tc.encoded) {
				t.Errorf("encoding: got: %v; want: %v", tc.decoded.Encode(), tc.encoded)
			}
		}
	})

	t.Run("decode", func(t *testing.T) {
		for _, tc := range tt {
			var actualDecoded mc.VarInt
			if err := actualDecoded.Decode(bytes.NewReader(tc.encoded)); err != nil {
				t.Errorf("decoding: %s", err)
			}

			if actualDecoded != tc.decoded {
				t.Errorf("decoding: got %v; want: %v", actualDecoded, tc.decoded)
			}
		}
	})
}

func TestString(t *testing.T) {
	tt := []struct {
		decoded mc.String
		encoded []byte
	}{
		{
			decoded: mc.String(""),
			encoded: []byte{0x00},
		},
		{
			decoded: mc.String("Hello, World!"),
			encoded: []byte{0x0d, 0x48, 0x65, 0x6c, 0x6c, 0x6f, 0x2c, 0x20, 0x57, 0x6f, 0x72, 0x6c, 0x64, 0x21},
		},
		{
			decoded: mc.String("Minecraft"),
			encoded: []byte{0x09, 0x4d, 0x69, 0x6e, 0x65, 0x63, 0x72, 0x61, 0x66, 0x74},
		},
		{
			decoded: mc.String("♥"),
			encoded: []byte{0x03, 0xe2, 0x99, 0xa5},
		},
	}

	t.Run("encode", func(t *testing.T) {
		for _, tc := range tt {
			if !bytes.Equal(tc.decoded.Encode(), tc.encoded) {
				t.Errorf("encoding: got: %v; want: %v", tc.decoded.Encode(), tc.encoded)
			}
		}
	})

	t.Run("decode", func(t *testing.T) {
		for _, tc := range tt {
			var actualDecoded mc.String
			if err := actualDecoded.Decode(bytes.NewReader(tc.encoded)); err != nil {
				t.Errorf("decoding: %s", err)
			}

			if actualDecoded != tc.decoded {
				t.Errorf("decoding: got %v; want: %v", actualDecoded, tc.decoded)
			}
		}
	})

}

func TestByte(t *testing.T) {
	tt := []struct {
		decoded mc.Byte
		encoded []byte
	}{
		{
			decoded: mc.Byte(0x00),
			encoded: []byte{0x00},
		},
		{
			decoded: mc.Byte(0x0f),
			encoded: []byte{0x0f},
		},
	}

	t.Run("encode", func(t *testing.T) {
		for _, tc := range tt {
			if !bytes.Equal(tc.decoded.Encode(), tc.encoded) {
				t.Errorf("encoding: got: %v; want: %v", tc.decoded.Encode(), tc.encoded)
			}
		}
	})

	t.Run("decode", func(t *testing.T) {
		for _, tc := range tt {
			var actualDecoded mc.Byte
			if err := actualDecoded.Decode(bytes.NewReader(tc.encoded)); err != nil {
				t.Errorf("decoding: %s", err)
			}

			if actualDecoded != tc.decoded {
				t.Errorf("decoding: got %v; want: %v", actualDecoded, tc.decoded)
			}
		}
	})
}
func TestUnsignedShort(t *testing.T) {
	tt := []struct {
		decoded mc.UnsignedShort
		encoded []byte
	}{
		{
			decoded: mc.UnsignedShort(0),
			encoded: []byte{0x00, 0x00},
		},
		{
			decoded: mc.UnsignedShort(15),
			encoded: []byte{0x00, 0x0f},
		},
		{
			decoded: mc.UnsignedShort(16),
			encoded: []byte{0x00, 0x10},
		},
		{
			decoded: mc.UnsignedShort(255),
			encoded: []byte{0x00, 0xff},
		},
		{
			decoded: mc.UnsignedShort(256),
			encoded: []byte{0x01, 0x00},
		},
		{
			decoded: mc.UnsignedShort(65535),
			encoded: []byte{0xff, 0xff},
		},
	}

	t.Run("encode", func(t *testing.T) {
		for _, tc := range tt {
			if !bytes.Equal(tc.decoded.Encode(), tc.encoded) {
				t.Errorf("encoding: got: %v; want: %v", tc.decoded.Encode(), tc.encoded)
			}
		}
	})

	t.Run("decode", func(t *testing.T) {
		for _, tc := range tt {
			var actualDecoded mc.UnsignedShort
			if err := actualDecoded.Decode(bytes.NewReader(tc.encoded)); err != nil {
				t.Errorf("decoding: %s", err)
			}

			if actualDecoded != tc.decoded {
				t.Errorf("decoding: got %v; want: %v", actualDecoded, tc.decoded)
			}
		}
	})
}

func TestReadVarInt(t *testing.T) {
	tt := []struct {
		decoded mc.VarInt
		encoded []byte
	}{
		{
			decoded: 0,
			encoded: []byte{0x00},
		},
		{
			decoded: 1,
			encoded: []byte{0x01},
		},
		{
			decoded: 2,
			encoded: []byte{0x02},
		},
		{
			decoded: 127,
			encoded: []byte{0x7f},
		},
		{
			decoded: 128,
			encoded: []byte{0x80, 0x01},
		},
		{
			decoded: 129,
			encoded: []byte{0x81, 0x01},
		},
		{
			decoded: 255,
			encoded: []byte{0xff, 0x01},
		},
		{
			decoded: 256,
			encoded: []byte{0x80, 0x02},
		},
		{
			decoded: 2097151,
			encoded: []byte{0xff, 0xff, 0x7f},
		},
		{
			decoded: 2147483647,
			encoded: []byte{0xff, 0xff, 0xff, 0xff, 0x07},
		},
		{
			decoded: -1,
			encoded: []byte{0xff, 0xff, 0xff, 0xff, 0x0f},
		},
		{
			decoded: -2147483648,
			encoded: []byte{0x80, 0x80, 0x80, 0x80, 0x08},
		},
	}

	for _, tc := range tt {
		t.Run(fmt.Sprint(tc.decoded), func(t *testing.T) {
			t.Logf("%b", tc.encoded)
			reader := bytes.NewReader(tc.encoded)
			decodedValue, _ := mc.ReadVarInt(reader)
			t.Logf("%b", decodedValue)
			if decodedValue != tc.decoded {
				t.Errorf("decoding: got %v; want: %v", decodedValue, tc.decoded)
			}
		})
	}
}

func TestReadVarInt_ReturnError_WhenLargerThan5(t *testing.T) {
	data := []byte{0xff, 0xff, 0xff, 0xff, 0xff, 0xff}
	_, _, err := mc.ReadPacketSize_Bytes(data)
	if !errors.Is(err, mc.ErrVarIntSize) {
		t.Fatal("expected an error but didnt got one")
	}

}
