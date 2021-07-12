package proxy

import (
	"bufio"
	"errors"
	"io"
	"log"
	"net"
	"time"

	"github.com/realDragonium/Ultraviolet/mc"
)

type McAction byte
type McRequestType byte

const (
	PROXY McAction = iota
	DISCONNECT
	SEND_STATUS
	SEND_STATUS_CACHE
	CLOSE
	ERROR
)

func (state McAction) String() string {
	var text string
	switch state {
	case PROXY:
		text = "Proxy"
	case DISCONNECT:
		text = "Disconnect"
	case SEND_STATUS:
		text = "Send Status"
	case SEND_STATUS_CACHE:
		text = "Send Status Cache"
	case CLOSE:
		text = "Close"
	case ERROR:
		text = "Error"
	}
	return text
}

const (
	STATUS McRequestType = iota + 1
	LOGIN
)

func (t McRequestType) String() string {
	var text string
	switch t {
	case STATUS:
		text = "Status"
	case LOGIN:
		text = "Login"
	}
	return text
}

type McRequest struct {
	Type       McRequestType
	ServerAddr string
	Username   string
	Addr       net.Addr
	Ch         chan McAnswer
}

type McAnswer struct {
	ServerConnFunc func() (net.Conn, error)
	DisconMessage  mc.Packet
	Action         McAction
	StatusPk       mc.Packet
	ProxyCh        chan ProxyAction
	Latency        time.Duration
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
		c.Close()
		return
	}
	handshake, err := mc.UnmarshalServerBoundHandshake(handshakePacket)
	if err != nil {
		log.Printf("Error while unmarshaling handshake packet: %v", err)
		c.Close()
		return
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
		sConn, err := ans.ServerConnFunc()
		if err != nil {
			log.Printf("Err when creating server connection: %v", err)
			c.Close()
			return
		}
		serverConn := NewMcConn(sConn)
		serverConn.WritePacket(handshakePacket)
		if isLoginReq {
			err = serverConn.WritePacket(loginPacket)
		} else {
			// For some unknown reason if we dont send this here
			//  its goes wrong with proxying status requests
			err = serverConn.WritePacket(mc.Packet{ID: 0x00})
		}
		if err != nil {
			log.Printf("Error in client goroutine: %v", err)
			c.Close()
			sConn.Close()
			return
		}
		go func(client, server net.Conn, proxyCh chan ProxyAction) {
			ProxyConnections(client, server, proxyCh)
		}(c, sConn, ans.ProxyCh)
	case DISCONNECT:
		conn.WritePacket(ans.DisconMessage)
		conn.netConn.Close()
	case SEND_STATUS:
		conn.ReadPacket()
		conn.WritePacket(ans.StatusPk)
		pingPk, _ := conn.ReadPacket()
		conn.WritePacket(pingPk)
		conn.netConn.Close()
	case SEND_STATUS_CACHE:
		conn.ReadPacket()
		conn.WritePacket(ans.StatusPk)
		pingPk, _ := conn.ReadPacket()
		time.Sleep(ans.Latency)
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
	// Close behavior might not work that well
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
	pk, err := mc.ReadPacket(conn.reader)
	return pk, err
}

func (conn McConn) WritePacket(p mc.Packet) error {
	pk, err := p.Marshal()
	if err != nil {
		return errors.New("some idiot (probably me) is sending an invalid packet through here")
	}
	_, err = conn.netConn.Write(pk)
	return err
}
