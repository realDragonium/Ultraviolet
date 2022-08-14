package ultravioletv2

import (
	"encoding/binary"
	"io"
)

func ReadVarInt(r io.Reader) (v int, err error) {
	for i := 0; ; i++ {
		var b byte
		b, err = ReadByte(r)
		if err != nil {
			return 0, err
		}

		v |= int(b&0x7f) << uint(7*i)
		if b&0x80 == 0 {
			break
		}
	}
	return
}

func WriteVarInt(w io.Writer, varInt int) (int, error) {
	var bb []byte
	for {
		b := varInt & 0x7F
		varInt >>= 7
		if varInt != 0 {
			b |= 0x80
		}
		bb = append(bb, byte(b))
		if varInt == 0 {
			break
		}
	}

	return w.Write(bb)
}

func ReadByte(r io.Reader) (b byte, err error) {
	binary.Read(r, binary.BigEndian, &b)
	return
}

func WriteByte(w io.Writer, b byte) error {
	return binary.Write(w, binary.BigEndian, b)
}

func ReadString(r io.Reader) (string, error) {
	length, err := ReadVarInt(r)
	if err != nil {
		return "", err
	}

	bb := make([]byte, length)
	_, err = r.Read(bb)
	if err != nil {
		return "", err
	}

	return string(bb), nil
}

func WriteString(w io.Writer, s string) (int, error) {
	length := len(s)
	m, err := WriteVarInt(w, length)
	if err != nil {
		return 0, err
	}

	n, err := w.Write([]byte(s))
	n += m
	return n, err
}

func ReadShort(r io.Reader) (bb int16, err error) {
	err = binary.Read(r, binary.BigEndian, &bb)
	return
}

func WriteShort(w io.Writer, short int16) error {
	return binary.Write(w, binary.BigEndian, short)
}
