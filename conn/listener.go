package conn

import (
	"bufio"
	"io"
	"net"

	"github.com/realDragonium/UltraViolet/mc"
)

func NewListener(listener net.Listener) Listener {
	return Listener{
		listener:    listener,
		LoginReqCh:  make(chan LoginRequest),
		LoginAnsCh:  make(chan LoginAnswer),
		StatusReqCh: make(chan StatusRequest),
		StatusAnsCh: make(chan StatusAnswer),
	}
}

type Listener struct {
	listener    net.Listener
	LoginReqCh  chan LoginRequest
	LoginAnsCh  chan LoginAnswer
	StatusReqCh chan StatusRequest
	StatusAnsCh chan StatusAnswer
}

func (l Listener) Serve() {
	for {
		conn, _ := l.listener.Accept()
		go l.ReadConnection(conn)
	}
}

func (l Listener) ReadConnection(c net.Conn) {
	// Rewrite this
	conn := NewHandshakeConn(c, c.RemoteAddr())
	packet, _ := conn.ReadPacket()
	handshake, _ := mc.UnmarshalServerBoundHandshake(packet)
	conn.HandshakePacket = packet
	conn.Handshake = handshake

	switch handshake.NextState {
	case mc.ServerBoundHandshakeStatusState:
		l.DoStatusSequence(conn)
	case mc.ServerBoundHandshakeLoginState:
		l.DoLoginSequence(conn)
	default:
		conn.netConn.Close()
	}
}

type LoginRequest struct {
	ServerAddr string
	Username   string
	Ip         net.Addr
}
type LoginAnswer struct {
	ServerConn    net.Conn
	DisconMessage mc.Packet
	Proxy         bool
}

func (l Listener) DoLoginSequence(conn HandshakeConn) {
	// Rewrite this
	packet, _ := conn.ReadPacket()
	loginStart, _ := mc.UnmarshalServerBoundLoginStart(packet)

	l.LoginReqCh <- LoginRequest{
		ServerAddr: string(conn.Handshake.ServerAddress),
		Username:   string(loginStart.Name),
		Ip:         conn.RemoteAddr(),
	}
	loginAns := <-l.LoginAnsCh
	if loginAns.Proxy {
		go ProxyConnections(conn.netConn, loginAns.ServerConn)
	} else {
		conn.WritePacket(loginAns.DisconMessage)
		conn.netConn.Close()
	}
}

type StatusRequest struct{}
type StatusAnswer struct {
	Proxy      bool
	ServerConn net.Conn
	StatusPk   mc.Packet
}

func (l Listener) DoStatusSequence(conn HandshakeConn) {
	l.StatusReqCh <- StatusRequest{}
	statusAns := <-l.StatusAnsCh
	if statusAns.Proxy {
		go ProxyConnections(conn.netConn, statusAns.ServerConn)
	} else {
		//Rewrite this
		conn.ReadPacket()
		conn.WritePacket(statusAns.StatusPk)
		pingPk, _ := conn.ReadPacket()
		conn.WritePacket(pingPk)
		conn.netConn.Close()
	}
}

func ProxyConnections(client, server net.Conn) {
	go func() {
		io.Copy(server, client)
		client.Close()
	}()
	io.Copy(client, server)
	server.Close()
}

// handshake, err := mc.ReadHandshake(byteReader)
// if err != nil {
// return
// }

// packetBuffer := make([]byte, mc.MaxPacketSize)
// conn.Read(packetBuffer)
// byteReader := bytes.NewReader(packetBuffer)
// packet, err := mc.ReadPacketOld(byteReader)

// func triedSomething (){
// 	packetBuffer := make([]byte, mc.MaxPacketSize)
// 	conn.Read(packetBuffer)

// 	packetSize, sizeLength, err := mc.ReadPacketSize_Bytes(packetBuffer)
// 	if errors.Is(err, mc.ErrVarIntSize) || packetSize > mc.MaxPacketSize {
// 		conn.Close()
// 		return
// 	}
// 	packetBytes := packetBuffer[sizeLength:packetSize]

// 	emptyFunc(packetBytes)
// }
// func emptyFunc(something ...interface{}) {

// }

func NewHandshakeConn(conn net.Conn, remoteAddr net.Addr) HandshakeConn {
	return HandshakeConn{
		netConn: conn,
		reader:  bufio.NewReader(conn),
		addr:    remoteAddr,
	}
}

type HandshakeConn struct {
	netConn net.Conn
	reader  mc.DecodeReader
	addr    net.Addr

	Handshake       mc.ServerBoundHandshake
	HandshakePacket mc.Packet
}

func (hsConn HandshakeConn) RemoteAddr() net.Addr {
	return hsConn.addr
}

func (conn HandshakeConn) ReadPacket() (mc.Packet, error) {
	return mc.ReadPacket(conn.reader)
}

func (conn HandshakeConn) WritePacket(p mc.Packet) error {
	pk, _ := p.Marshal()
	_, err := conn.netConn.Write(pk)
	return err
}
