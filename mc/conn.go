package mc

import (
	"bufio"
	"net"
)

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

func DialMCServer(addr string) (mcConn, error) {
	conn, err := net.Dial("tcp", addr)
	if err != nil {
		return mcConn{}, err
	}
	return NewMcConn(conn), err
}
