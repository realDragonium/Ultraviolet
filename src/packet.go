package ultravioletv2

import (
	"bytes"
	"io"
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
