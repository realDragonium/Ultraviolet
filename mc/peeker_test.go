package mc_test

import (
	"bufio"
	"bytes"
	"io"
	"testing"

	"github.com/realDragonium/UltraViolet/mc"
)

func TestBytePeeker_ReadByte(t *testing.T) {
	tt := []struct {
		peeker       mc.BytePeeker
		data         []byte
		expectedByte byte
	}{
		{
			peeker: mc.BytePeeker{
				Cursor: 0,
			},
			data:         []byte{0x00, 0x01, 0x02, 0x03},
			expectedByte: 0x00,
		},
		{
			peeker: mc.BytePeeker{
				Cursor: 1,
			},
			data:         []byte{0x00, 0x01, 0x02, 0x03},
			expectedByte: 0x01,
		},
		{
			peeker: mc.BytePeeker{
				Cursor: 3,
			},
			data:         []byte{0x00, 0x01, 0x02, 0x03},
			expectedByte: 0x03,
		},
	}

	for _, tc := range tt {
		clonedData := make([]byte, len(tc.data))
		copy(clonedData, tc.data)
		tc.peeker.PeekReader = bufio.NewReader(bytes.NewReader(clonedData))

		b, err := tc.peeker.ReadByte()
		if err != nil && err != io.EOF {
			t.Error(err)
		}

		if b != tc.expectedByte {
			t.Errorf("got: %v; want: %v", b, tc.expectedByte)
		}
		if !bytes.Equal(clonedData, tc.data) {
			t.Errorf("data modified: got: %v; want: %v", clonedData, tc.data)
		}
	}
}

func TestBytePeeker_Read(t *testing.T) {
	tt := []struct {
		peeker       mc.BytePeeker
		data         []byte
		expectedData []byte
		expectedN    int
	}{
		{
			peeker: mc.BytePeeker{
				Cursor: 0,
			},
			data:         []byte{0x00, 0x01, 0x02, 0x03},
			expectedData: []byte{0x00, 0x01, 0x02, 0x03},
			expectedN:    4,
		},
		{
			peeker: mc.BytePeeker{
				Cursor: 1,
			},
			data:         []byte{0x00, 0x01, 0x02, 0x03},
			expectedData: []byte{0x01, 0x02, 0x03},
			expectedN:    3,
		},
		{
			peeker: mc.BytePeeker{
				Cursor: 3,
			},
			data:         []byte{0x00, 0x01, 0x02, 0x03},
			expectedData: []byte{0x03},
			expectedN:    1,
		},
	}

	for _, tc := range tt {
		clonedData := make([]byte, len(tc.data))
		copy(clonedData, tc.data)
		tc.peeker.PeekReader = bufio.NewReader(bytes.NewReader(clonedData))
		resultData := make([]byte, len(tc.expectedData))

		n, err := tc.peeker.Read(resultData)
		if err != nil && err != io.EOF {
			t.Error(err)
		}

		if n != tc.expectedN {
			t.Errorf("got: %v; want: %v", n, tc.expectedN)
		}
		if !bytes.Equal(resultData, tc.expectedData) {
			t.Errorf("got: %v; want: %v", resultData, tc.expectedData)
		}
		if !bytes.Equal(clonedData, tc.data) {
			t.Errorf("data modified: got: %v; want: %v", clonedData, tc.data)
		}
	}
}
