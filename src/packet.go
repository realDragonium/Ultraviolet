package ultravioletv2

import (
	"bytes"
	"encoding/binary"
	"io"
	"log"
)

type ServerBoundHandshakePacket struct {
	PacketId        byte
	ProtocolVersion int
	ServerAddress   string
	ServerPort      int16
	NextState       int
}

func (pk ServerBoundHandshakePacket) WriteTo(w io.Writer) (int64, error) {
	buf := bytes.NewBuffer([]byte{})

	WriteByte(buf, pk.PacketId)
	l := 1 // len of byte
	m, _ := WriteVarInt(buf, pk.ProtocolVersion)
	n, _ := WriteString(buf, pk.ServerAddress)
	WriteShort(buf, pk.ServerPort)
	o := 2 // len of short
	p, _ := WriteVarInt(buf, pk.NextState)

	pkLen := l + m + n + o + p

	q, _ := WriteVarInt(w, pkLen)
	buf.WriteTo(w)

	pkLen += q
	return int64(pkLen), nil
}

func ReadServerBoundHandshake(r io.Reader) (pk ServerBoundHandshakePacket, err error) {
	pk.PacketId, err = ReadByte(r)
	if err != nil {
		return
	}

	pk.ProtocolVersion, err = ReadVarInt(r)
	if err != nil {
		return
	}

	pk.ServerAddress, err = ReadString(r)
	if err != nil {
		return
	}

	pk.ServerPort, err = ReadShort(r)
	if err != nil {
		return
	}

	pk.NextState, err = ReadVarInt(r)
	if err != nil {
		return
	}

	return
}

// UnconnectedMessageSequence is a sequence of bytes which is found in every unconnected message sent in RakNet.
var UnconnectedMessageSequence = [16]byte{0x00, 0xff, 0xff, 0x00, 0xfe, 0xfe, 0xfe, 0xfe, 0xfd, 0xfd, 0xfd, 0xfd, 0x12, 0x34, 0x56, 0x78}

type UnconnectedPing struct {
	SendTimestamp int64
	Magic         [16]byte
	ClientGUID    int64
}

func (pk *UnconnectedPing) Write(w io.Writer) (err error) {
	buf := bytes.NewBuffer([]byte{})
	err = binary.Write(buf, binary.BigEndian, IDUnconnectedPing)
	if err != nil {
		log.Printf("Error writing IDUnconnectedPing: %v", err)
	}
	err = binary.Write(buf, binary.BigEndian, pk.SendTimestamp)
	if err != nil {
		log.Printf("Error writing SendTimestamp: %v", err)
	}
	err = binary.Write(buf, binary.BigEndian, UnconnectedMessageSequence)
	if err != nil {
		log.Printf("Error writing unconnectedMessageSequence: %v", err)
	}
	err = binary.Write(buf, binary.BigEndian, pk.ClientGUID)
	if err != nil {
		log.Printf("Error writing ClientGUID: %v", err)
	}

	w.Write(buf.Bytes())

	return
}

func (pk *UnconnectedPing) Read(r io.Reader) error {
	_ = binary.Read(r, binary.BigEndian, &pk.SendTimestamp)
	_ = binary.Read(r, binary.BigEndian, &pk.Magic)
	return binary.Read(r, binary.BigEndian, &pk.ClientGUID)
}

type UnconnectedPong struct {
	SendTimestamp int64
	Magic         [16]byte
	ServerGUID    int64
	Data          string
}

func (pk *UnconnectedPong) Write(w io.Writer) {
	_ = binary.Write(w, binary.BigEndian, IDUnconnectedPong)
	_ = binary.Write(w, binary.BigEndian, pk.SendTimestamp)
	_ = binary.Write(w, binary.BigEndian, pk.ServerGUID)
	_ = binary.Write(w, binary.BigEndian, UnconnectedMessageSequence)
	_ = binary.Write(w, binary.BigEndian, int16(len(pk.Data)))
	_ = binary.Write(w, binary.BigEndian, []byte(pk.Data))
}

func (pk *UnconnectedPong) Read(r io.Reader) error {
	var l int16
	_ = binary.Read(r, binary.BigEndian, &pk.SendTimestamp)
	_ = binary.Read(r, binary.BigEndian, &pk.ServerGUID)
	_ = binary.Read(r, binary.BigEndian, &pk.Magic)
	_ = binary.Read(r, binary.BigEndian, &l)
	readB := make([]byte, l)
	n, err := r.Read(readB)
	pk.Data = string(readB[:n])
	return err
}
