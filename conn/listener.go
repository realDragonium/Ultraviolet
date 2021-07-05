package conn

import (
	"bufio"
	"errors"
	"io"
	"log"
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
		conn, err := l.listener.Accept()
		if err != nil {
			if errors.Is(err, net.ErrClosed) {
				log.Printf("net.Listener was closed, shutting down listener")
				break
			}
			log.Println(err)
			continue
		}
		go l.ReadConnection(conn)
	}
}

func (l Listener) ReadConnection(c net.Conn) {
	// Rewrite this
	conn := NewHandshakeConn(c, c.RemoteAddr())
	packet, err := conn.ReadPacket()
	if err != nil {
		log.Printf("Error while reading handshake packet: %v", err)
	}
	handshake, err := mc.UnmarshalServerBoundHandshake(packet)
	if err != nil {
		log.Printf("Error while unmarshaling handshake packet: %v", err)
	}
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
	Action        ConnAction
	NotifyClosed  chan struct{}
}
type ConnAction byte

const (
	PROXY ConnAction = iota
	DISCONNECT
	SEND_STATUS
	CLOSE
)

func (l Listener) DoLoginSequence(conn HandshakeConn) {
	// Rewrite this
	packet, err := conn.ReadPacket()
	if err != nil {
		log.Printf("Error while reading login start packet: %v", err)
	}
	loginStart, err := mc.UnmarshalServerBoundLoginStart(packet)
	if err != nil {
		log.Printf("Error while unmarshaling login start packet: %v", err)
	}
	l.LoginReqCh <- LoginRequest{
		ServerAddr: string(conn.Handshake.ServerAddress),
		Username:   string(loginStart.Name),
		Ip:         conn.RemoteAddr(),
	}
	ans := <-l.LoginAnsCh
	switch ans.Action {
	case PROXY:
		bytes, _ := conn.HandshakePacket.Marshal()
		ans.ServerConn.Write(bytes)
		bytes, _ = packet.Marshal()
		ans.ServerConn.Write(bytes)
		go ProxyConnections(conn.netConn, ans.ServerConn, ans.NotifyClosed)
	case DISCONNECT:
		conn.WritePacket(ans.DisconMessage)
		conn.netConn.Close()
	case CLOSE:
		conn.netConn.Close()
	}
}

type StatusRequest struct{}
type StatusAnswer struct {
	Action       ConnAction
	ServerConn   net.Conn
	StatusPk     mc.Packet
	NotifyClosed chan struct{}
}

func (l Listener) DoStatusSequence(conn HandshakeConn) {
	l.StatusReqCh <- StatusRequest{}
	ans := <-l.StatusAnsCh
	switch ans.Action {
	case PROXY:
		bytes, _ := conn.HandshakePacket.Marshal()
		ans.ServerConn.Write(bytes)
		go ProxyConnections(conn.netConn, ans.ServerConn, ans.NotifyClosed)
	case SEND_STATUS:
		conn.ReadPacket()
		conn.WritePacket(ans.StatusPk)
		pingPk, _ := conn.ReadPacket()
		conn.WritePacket(pingPk)
		conn.netConn.Close()
	case CLOSE:
		conn.netConn.Close()
	}
}

func ProxyConnections(client, server net.Conn, closeCh chan struct{}) {
	go func() {
		io.Copy(server, client)
		client.Close()
	}()
	io.Copy(client, server)
	server.Close()
	closeCh <- struct{}{}
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
