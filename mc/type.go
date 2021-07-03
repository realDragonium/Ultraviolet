package mc

import (
	"errors"
	"io"
)

// A Field is both FieldEncoder and FieldDecoder
type Field interface {
	FieldEncoder
	FieldDecoder
}

// A FieldEncoder can be encode as minecraft protocol used.
type FieldEncoder interface {
	Encode() []byte
}

// A FieldDecoder can Decode from minecraft protocol
type FieldDecoder interface {
	Decode(r DecodeReader) error
}

//DecodeReader is both io.Reader and io.ByteReader
type DecodeReader interface {
	io.ByteReader
	io.Reader
}

type (
	// Byte is signed 8-bit integer, two's complement
	Byte int8
	// UnsignedShort is unsigned 16-bit integer
	UnsignedShort uint16
	// String is sequence of Unicode scalar values
	String string
	// Chat is encoded as a String with max length of 32767.
	Chat = String
	// VarInt is variable-length data encoding a two's complement signed 32-bit integer
	VarInt int32
)

// ReadNBytes read N bytes from bytes.Reader
func ReadNBytes(r DecodeReader, n int) ([]byte, error) {
	bb := make([]byte, n)
	var err error
	for i := 0; i < n; i++ {
		bb[i], err = r.ReadByte()
		if err != nil {
			return nil, err
		}
	}
	return bb, nil
}

// Encode a String
func (s String) Encode() []byte {
	byteString := []byte(s)
	var bb []byte
	bb = append(bb, VarInt(len(byteString)).Encode()...) // len
	bb = append(bb, byteString...)                       // data
	return bb
}

// Decode a String
func (s *String) Decode(r DecodeReader) error {
	var l VarInt // String length
	if err := l.Decode(r); err != nil {
		return err
	}

	bb, err := ReadNBytes(r, int(l))
	if err != nil {
		return err
	}

	*s = String(bb)
	return nil
}

// Encode a Byte
func (b Byte) Encode() []byte {
	return []byte{byte(b)}
}

// Decode a Byte
func (b *Byte) Decode(r DecodeReader) error {
	v, err := r.ReadByte()
	if err != nil {
		return err
	}
	*b = Byte(v)
	return nil
}

// Encode a Unsigned Short
func (us UnsignedShort) Encode() []byte {
	n := uint16(us)
	return []byte{
		byte(n >> 8),
		byte(n),
	}
}

// Decode a UnsignedShort
func (us *UnsignedShort) Decode(r DecodeReader) error {
	bb, err := ReadNBytes(r, 2)
	if err != nil {
		return err
	}

	*us = UnsignedShort(int16(bb[0])<<8 | int16(bb[1]))
	return nil
}

// Encode a VarInt
func (v VarInt) Encode() []byte {
	num := uint32(v)
	var bb []byte
	for {
		b := num & 0x7F
		num >>= 7
		if num != 0 {
			b |= 0x80
		}
		bb = append(bb, byte(b))
		if num == 0 {
			break
		}
	}
	return bb
}

// Decode a VarInt
func (v *VarInt) Decode(r DecodeReader) error {
	var n uint32
	for i := 0; ; i++ {
		sec, err := r.ReadByte()
		if err != nil {
			return err
		}

		n |= uint32(sec&0x7F) << uint32(7*i)

		if i >= 5 {
			return errors.New("VarInt is too big")
		} else if sec&0x80 == 0 {
			break
		}
	}

	*v = VarInt(n)
	return nil
}
