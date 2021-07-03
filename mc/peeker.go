package mc

import "io"

type PeekReader interface {
	Peek(n int) ([]byte, error)
	io.Reader
}

type BytePeeker struct {
	PeekReader
	Cursor int
}

func (peeker *BytePeeker) Read(b []byte) (int, error) {
	buf, err := peeker.Peek(len(b) + peeker.Cursor)
	if err != nil {
		return 0, err
	}

	for i := 0; i < len(b); i++ {
		b[i] = buf[i+peeker.Cursor]
	}

	peeker.Cursor += len(b)

	return len(b), nil
}

func (peeker *BytePeeker) ReadByte() (byte, error) {
	buf, err := peeker.Peek(1 + peeker.Cursor)
	if err != nil {
		return 0x00, err
	}

	b := buf[peeker.Cursor]
	peeker.Cursor++

	return b, nil
}
