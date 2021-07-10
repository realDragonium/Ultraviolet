package proxy

import (
	"bufio"
	"errors"
	"io"
	"log"
	"net"

	"github.com/realDragonium/Ultraviolet/mc"
)

type McAction byte
type McRequestType byte

const (
	PROXY McAction = iota
	DISCONNECT
	SEND_STATUS
	CLOSE
	ERROR
)

const (
	STATUS McRequestType = iota + 1
	LOGIN
)

type McRequest struct {
	Type       McRequestType
	ServerAddr string
	Username   string
	Addr       net.Addr
	Ch         chan McAnswer
}

type McAnswer struct {
	// ServerConn     McConn
	ServerConnFunc func(net.Addr) (net.Conn, error)
	DisconMessage  mc.Packet
	Action         McAction
	StatusPk       mc.Packet
	ProxyCh        chan ProxyAction
}

func ServeListener(listener net.Listener, reqCh chan McRequest) {
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

func ReadConnection(c net.Conn, reqCh chan McRequest) {
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

	ansCh := make(chan McAnswer)
	req := McRequest{
		Type:       STATUS,
		Ch:         ansCh,
		Addr:       c.RemoteAddr(),
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
		req.Type = LOGIN
		req.Username = string(loginStart.Name)
	}
	reqCh <- req
	ans := <-ansCh

	switch ans.Action {
	case PROXY:
		sConn, _ := ans.ServerConnFunc(c.RemoteAddr())
		serverConn := NewMcConn(sConn)
		serverConn.WritePacket(handshakePacket)
		if isLoginReq {
			serverConn.WritePacket(loginPacket)
		}
		go func(client, server net.Conn, proxyCh chan ProxyAction) {
			ProxyConnections(client, server, proxyCh)
		}(conn.netConn, serverConn.netConn, ans.ProxyCh)
	case DISCONNECT:
		conn.WritePacket(ans.DisconMessage)
		conn.netConn.Close()
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

// Check or doing this in a separate method has some adventages like:
// - having a different stack so the other data can be collected
// - doesnt give too much trouble with copy the connections
func ProxyConnections(client, server net.Conn, proxyCh chan ProxyAction) {
	proxyCh <- PROXY_OPEN
	go func() {
		io.Copy(server, client)
		client.Close()
	}()
	io.Copy(client, server)
	server.Close()
	proxyCh <- PROXY_CLOSE
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
