package conn

import (
	"bufio"
	"errors"
	"io"
	"log"
	"net"

	"github.com/realDragonium/Ultraviolet/mc"
)

type ConnAction byte
type ConnType byte

const (
	PROXY ConnAction = iota
	DISCONNECT
	SEND_STATUS
	CLOSE
)

const (
	STATUS ConnType = iota + 1
	LOGIN
)

type ConnRequest struct {
	Type       ConnType
	ServerAddr string
	Username   string
	Ip         net.Addr
	Ch         chan ConnAnswer
}

type ConnAnswer struct {
	ServerConn    McConn
	DisconMessage mc.Packet
	Action        ConnAction
	StatusPk      mc.Packet
	NotifyClosed  chan struct{}
}

func Serve(listener net.Listener, reqCh chan ConnRequest) {
	for {
		conn, err := listener.Accept()
		if err != nil {
			if errors.Is(err, net.ErrClosed) {
				log.Printf("net.Listener was closed, shutting down listener")
				break
			}
			log.Println(err)
			continue
		}
		go ReadConnection(conn, reqCh)
	}
}

func ReadConnection(c net.Conn, reqCh chan ConnRequest) {
	// Rewrite connection code?
	conn := NewMcConn(c)
	// Add better error handling
	handshakePacket, err := conn.ReadPacket()
	if err != nil {
		log.Printf("Error while reading handshake packet: %v", err)
	}
	handshake, err := mc.UnmarshalServerBoundHandshake(handshakePacket)
	if err != nil {
		log.Printf("Error while unmarshaling handshake packet: %v", err)
	}

	if handshake.NextState != mc.HandshakeLoginState && handshake.NextState != mc.HandshakeStatusState {
		conn.netConn.Close()
		return
	}

	isLoginReq := false
	if handshake.NextState == mc.HandshakeLoginState {
		isLoginReq = true
	}

	ansCh := make(chan ConnAnswer)
	req := ConnRequest{
		Ch:         ansCh,
		Ip:         c.RemoteAddr(),
		ServerAddr: string(handshake.ServerAddress),
	}
	var loginPacket mc.Packet
	if isLoginReq {
		// Add better error handling
		loginPacket, err = conn.ReadPacket()
		if err != nil {
			log.Printf("Error while reading login start packet: %v", err)
		}
		loginStart, err := mc.UnmarshalServerBoundLoginStart(loginPacket)
		if err != nil {
			log.Printf("Error while unmarshaling login start packet: %v", err)
		}
		req.Username = string(loginStart.Name)
	}
	reqCh <- req
	ans := <-ansCh

	switch ans.Action {
	case PROXY:
		serverConn := ans.ServerConn
		serverConn.WritePacket(handshakePacket)
		if isLoginReq {
			serverConn.WritePacket(loginPacket)
		}
		go func(client, server net.Conn, notfiyClosedCh chan struct{}) {
			ProxyConnections(client, server, notfiyClosedCh)
		}(conn.netConn, serverConn.netConn, ans.NotifyClosed)
	case DISCONNECT:
		conn.WritePacket(ans.DisconMessage)
		conn.netConn.Close()
	case SEND_STATUS:
		// Notchian servers will wait for ping packet before sending response...? 
		// source: https://wiki.vg/Server_List_Ping#Response (first line -> second sentence)
		conn.ReadPacket()
		conn.WritePacket(ans.StatusPk)
		pingPk, _ := conn.ReadPacket()
		conn.WritePacket(pingPk)
		conn.netConn.Close()
	case CLOSE:
		conn.netConn.Close()
	}

}

// Check or doing this in a separate method has some adventages like:
// - having a different stack so the other data can be collected
// - doesnt give too much trouble with copy the connections
func ProxyConnections(client, server net.Conn, notifyClosedCh chan struct{}) {
	go func() {
		io.Copy(server, client)
		client.Close()
	}()
	io.Copy(client, server)
	server.Close()
	notifyClosedCh <- struct{}{}
}

func NewMcConn(conn net.Conn) McConn {
	return McConn{
		netConn: conn,
		reader:  bufio.NewReader(conn),
	}
}

type McConn struct {
	netConn net.Conn
	reader  mc.DecodeReader
}

func (conn McConn) ReadPacket() (mc.Packet, error) {
	return mc.ReadPacket(conn.reader)
}

func (conn McConn) WritePacket(p mc.Packet) error {
	pk, err := p.Marshal()
	if err != nil {
		return errors.New("some idiot (probably me) is sending an invalid packet through here")
	}
	_, err = conn.netConn.Write(pk)
	return err
}
