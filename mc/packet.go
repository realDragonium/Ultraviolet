package mc

import (
	"bytes"
	"errors"
	"fmt"
	"io"
)

var (
	ErrInvalidPacketID = errors.New("invalid packet id")
	ErrPacketTooBig    = errors.New("packet contains too much data")
	MaxPacketSize      = 2097151
)

const (
	ServerBoundHandshakePacketID byte = 0x00

	StatusState = 1
	LoginState  = 2

	HandshakeStatusState = VarInt(StatusState)
	HandshakeLoginState  = VarInt(LoginState)

	ForgeSeparator  = "\x00"
	RealIPSeparator = "///"
)

// Packet is the raw representation of message that is send between the client and the server
type Packet struct {
	ID   byte
	Data []byte
}

// Scan decodes and copies the Packet data into the fields
func (pk Packet) Scan(fields ...FieldDecoder) error {
	return ScanFields(bytes.NewReader(pk.Data), fields...)
}

// Marshal encodes the packet and all it's fields
func (pk *Packet) Marshal() ([]byte, error) {
	var packedData []byte
	data := []byte{pk.ID}
	data = append(data, pk.Data...)
	packetLength := VarInt(int32(len(data))).Encode()
	packedData = append(packedData, packetLength...)

	return append(packedData, data...), nil
}

// ScanFields decodes a byte stream into fields
func ScanFields(r DecodeReader, fields ...FieldDecoder) error {
	for _, field := range fields {
		if err := field.Decode(r); err != nil {
			return err
		}
	}
	return nil
}

// MarshalPacket transforms an ID and Fields into a Packet
func MarshalPacket(ID byte, fields ...FieldEncoder) Packet {
	var pkt Packet
	pkt.ID = ID

	for _, v := range fields {
		pkt.Data = append(pkt.Data, v.Encode()...)
	}

	return pkt
}

// ReadPacketBytes decodes a byte stream and cuts the first Packet as a byte array out
func ReadPacketBytes(r DecodeReader) ([]byte, error) {
	var packetLength VarInt
	if err := packetLength.Decode(r); err != nil {
		return nil, err
	}

	if packetLength < 1 {
		return nil, fmt.Errorf("packet length too short")
	}

	data := make([]byte, packetLength)
	if _, err := io.ReadFull(r, data); err != nil {
		return nil, fmt.Errorf("reading the content of the packet failed: %v", err)
	}

	return data, nil
}

// ReadPacketOld decodes and decompresses a byte stream and cuts the first Packet out
func ReadPacketOld(r DecodeReader) (Packet, error) {
	data, err := ReadPacketBytes(r)

	if err != nil {
		return Packet{}, err
	}

	return Packet{
		ID:   data[0],
		Data: data[1:],
	}, nil
}

func ReadPacket(r DecodeReader) (Packet, error) {
	packetLength, err := ReadVarInt(r)
	if err != nil {
		return Packet{}, err
	}

	if packetLength < 1 {
		return Packet{}, fmt.Errorf("packet length too short")
	}

	data := make([]byte, packetLength)
	if _, err := io.ReadFull(r, data); err != nil {
		return Packet{}, fmt.Errorf("reading the content of the packet failed: %v", err)
	}

	return Packet{
		ID:   data[0],
		Data: data[1:],
	}, nil
}
