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
	_, err := WriteVarInt(w, length)
	if err != nil {
		return 0, err
	}

	return w.Write([]byte(s))
}

func ReadShort(r io.Reader) (bb int16, err error) {
	err = binary.Read(r, binary.BigEndian, &bb)

	return
}
