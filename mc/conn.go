package mc

import (
	"bufio"
	"net"

	
)


type McConn interface{
	ReadPacket() (Packet, error)
	WritePacket(p Packet) error
}

func NewMcConn(conn net.Conn) mcConn {
	return mcConn{
		netConn: conn,
		reader:  bufio.NewReader(conn),
	}
}

type mcConn struct {
	netConn net.Conn
	reader  DecodeReader
}

func (conn mcConn) ReadPacket() (Packet, error) {
	pk, err := ReadPacket(conn.reader)
	return pk, err
}

func (conn mcConn) WritePacket(p Packet) error {
	bytes := p.Marshal()
	_, err := conn.netConn.Write(bytes)
	return err
}

func (conn mcConn) WriteMcPacket(s McPacket) error {
	p := s.MarshalPacket()
	bytes := p.Marshal()
	_, err := conn.netConn.Write(bytes)
	return err
}
