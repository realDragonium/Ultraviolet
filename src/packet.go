package ultravioletv2

import (
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
	WriteByte(w, pk.PacketId)
	l := 1 // len of byte
	m, _ := WriteVarInt(w, pk.ProtocolVersion)
	n, _ := WriteString(w, pk.ServerAddress)
	WriteShort(w, pk.ServerPort)
	o := 2 // len of short
	p, _ := WriteVarInt(w, pk.NextState)

	pkLen := l + m + n + o + p
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
