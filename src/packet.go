package ultravioletv2

import (
	"bytes"
	"encoding/binary"
	"fmt"
	"io"
	"log"
	"net"
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

// unconnectedMessageSequence is a sequence of bytes which is found in every unconnected message sent in RakNet.
var unconnectedMessageSequence = [16]byte{0x00, 0xff, 0xff, 0x00, 0xfe, 0xfe, 0xfe, 0xfe, 0xfd, 0xfd, 0xfd, 0xfd, 0x12, 0x34, 0x56, 0x78}

type UnconnectedPing struct {
	SendTimestamp int64
	Magic         [16]byte
	ClientGUID    int64
}

func (pk *UnconnectedPing) Write(w io.Writer) (err error) {
	err = binary.Write(w, binary.BigEndian, IDUnconnectedPing)
	if err != nil {
		log.Printf("Error writing IDUnconnectedPing: %v", err)
	}
	err = binary.Write(w, binary.BigEndian, pk.SendTimestamp)
	if err != nil {
		log.Printf("Error writing SendTimestamp: %v", err)
	}
	err = binary.Write(w, binary.BigEndian, unconnectedMessageSequence)
	if err != nil {
		log.Printf("Error writing unconnectedMessageSequence: %v", err)
	}
	err = binary.Write(w, binary.BigEndian, pk.ClientGUID)
	if err != nil {
		log.Printf("Error writing ClientGUID: %v", err)
	}

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
	_ = binary.Write(w, binary.BigEndian, unconnectedMessageSequence)
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

// TODO: remove bytes.Buffer usage in some way
type OpenConnectionRequest1 struct {
	Magic                 [16]byte
	Protocol              byte
	MaximumSizeNotDropped uint16
}

func (pk *OpenConnectionRequest1) Write(buf *bytes.Buffer) {
	_ = binary.Write(buf, binary.BigEndian, IDOpenConnectionRequest1)
	_ = binary.Write(buf, binary.BigEndian, unconnectedMessageSequence)
	_ = binary.Write(buf, binary.BigEndian, pk.Protocol)
	_, _ = buf.Write(make([]byte, pk.MaximumSizeNotDropped-uint16(buf.Len()+28)))
}

func (pk *OpenConnectionRequest1) Read(buf *bytes.Buffer) error {
	pk.MaximumSizeNotDropped = uint16(buf.Len()+1) + 28
	_ = binary.Read(buf, binary.BigEndian, &pk.Magic)
	return binary.Read(buf, binary.BigEndian, &pk.Protocol)
}

type OpenConnectionReply1 struct {
	Magic                  [16]byte
	ServerGUID             int64
	Secure                 bool
	ServerPreferredMTUSize uint16
}

func (pk *OpenConnectionReply1) Write(w io.Writer) {
	_ = binary.Write(w, binary.BigEndian, IDOpenConnectionReply1)
	_ = binary.Write(w, binary.BigEndian, unconnectedMessageSequence)
	_ = binary.Write(w, binary.BigEndian, pk.ServerGUID)
	_ = binary.Write(w, binary.BigEndian, pk.Secure)
	_ = binary.Write(w, binary.BigEndian, pk.ServerPreferredMTUSize)
}

func (pk *OpenConnectionReply1) Read(r io.Reader) error {
	_ = binary.Read(r, binary.BigEndian, &pk.Magic)
	_ = binary.Read(r, binary.BigEndian, &pk.ServerGUID)
	_ = binary.Read(r, binary.BigEndian, &pk.Secure)
	return binary.Read(r, binary.BigEndian, &pk.ServerPreferredMTUSize)
}

type OpenConnectionRequest2 struct {
	Magic                  [16]byte
	ServerAddress          net.UDPAddr
	ClientPreferredMTUSize uint16
	ClientGUID             int64
}

func (pk *OpenConnectionRequest2) Write(buf *bytes.Buffer) {
	_ = binary.Write(buf, binary.BigEndian, IDOpenConnectionRequest2)
	_ = binary.Write(buf, binary.BigEndian, unconnectedMessageSequence)
	writeAddr(buf, pk.ServerAddress)
	_ = binary.Write(buf, binary.BigEndian, pk.ClientPreferredMTUSize)
	_ = binary.Write(buf, binary.BigEndian, pk.ClientGUID)
}

func (pk *OpenConnectionRequest2) Read(buf *bytes.Buffer) error {
	_ = binary.Read(buf, binary.BigEndian, &pk.Magic)
	_ = readAddr(buf, &pk.ServerAddress)
	_ = binary.Read(buf, binary.BigEndian, &pk.ClientPreferredMTUSize)
	return binary.Read(buf, binary.BigEndian, &pk.ClientGUID)
}

type OpenConnectionReply2 struct {
	Magic         [16]byte
	ServerGUID    int64
	ClientAddress net.UDPAddr
	MTUSize       uint16
	Secure        bool
}

func (pk *OpenConnectionReply2) Write(buf *bytes.Buffer) {
	_ = binary.Write(buf, binary.BigEndian, IDOpenConnectionReply2)
	_ = binary.Write(buf, binary.BigEndian, unconnectedMessageSequence)
	_ = binary.Write(buf, binary.BigEndian, pk.ServerGUID)
	writeAddr(buf, pk.ClientAddress)
	_ = binary.Write(buf, binary.BigEndian, pk.MTUSize)
	_ = binary.Write(buf, binary.BigEndian, pk.Secure)
}

func (pk *OpenConnectionReply2) Read(buf *bytes.Buffer) error {
	_ = binary.Read(buf, binary.BigEndian, &pk.Magic)
	_ = binary.Read(buf, binary.BigEndian, &pk.ServerGUID)
	_ = readAddr(buf, &pk.ClientAddress)
	_ = binary.Read(buf, binary.BigEndian, &pk.MTUSize)
	return binary.Read(buf, binary.BigEndian, &pk.Secure)
}

// writeAddr writes a UDP address to the buffer passed.
func writeAddr(buffer *bytes.Buffer, addr net.UDPAddr) {
	var ver byte = 6
	if addr.IP.To4() != nil {
		ver = 4
	}
	if addr.IP == nil {
		addr.IP = make([]byte, 16)
	}
	_ = buffer.WriteByte(ver)
	if ver == 4 {
		ipBytes := addr.IP.To4()

		_ = buffer.WriteByte(^ipBytes[0])
		_ = buffer.WriteByte(^ipBytes[1])
		_ = buffer.WriteByte(^ipBytes[2])
		_ = buffer.WriteByte(^ipBytes[3])
		_ = binary.Write(buffer, binary.BigEndian, uint16(addr.Port))
	} else {
		_ = binary.Write(buffer, binary.LittleEndian, int16(23)) // syscall.AF_INET6 on Windows.
		_ = binary.Write(buffer, binary.BigEndian, uint16(addr.Port))
		// The IPv6 address is enclosed in two 0 integers.
		_ = binary.Write(buffer, binary.BigEndian, int32(0))
		_, _ = buffer.Write(addr.IP.To16())
		_ = binary.Write(buffer, binary.BigEndian, int32(0))
	}
}

// readAddr decodes a RakNet address from the buffer passed. If not successful, an error is returned.
func readAddr(buffer *bytes.Buffer, addr *net.UDPAddr) error {
	ver, err := buffer.ReadByte()
	if err != nil {
		return err
	}
	if ver == 4 {
		ipBytes := make([]byte, 4)
		if _, err := buffer.Read(ipBytes); err != nil {
			return fmt.Errorf("error reading raknet address ipv4 bytes: %v", err)
		}
		// Construct an IPv4 out of the 4 bytes we just read.
		addr.IP = net.IPv4((-ipBytes[0]-1)&0xff, (-ipBytes[1]-1)&0xff, (-ipBytes[2]-1)&0xff, (-ipBytes[3]-1)&0xff)
		var port uint16
		if err := binary.Read(buffer, binary.BigEndian, &port); err != nil {
			return fmt.Errorf("error reading raknet address port: %v", err)
		}
		addr.Port = int(port)
	} else {
		buffer.Next(2)
		var port uint16
		if err := binary.Read(buffer, binary.LittleEndian, &port); err != nil {
			return fmt.Errorf("error reading raknet address port: %v", err)
		}
		addr.Port = int(port)
		buffer.Next(4)
		addr.IP = make([]byte, 16)
		if _, err := buffer.Read(addr.IP); err != nil {
			return fmt.Errorf("error reading raknet address ipv6 bytes: %v", err)
		}
		buffer.Next(4)
	}
	return nil
}
