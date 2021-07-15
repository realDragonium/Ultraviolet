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

func NewMcAnswerProxy(proxyCh chan ProxyAction, serverConnFunc func() (net.Conn, error)) McAnswerBasic {
	return McAnswerBasic{
		action:         PROXY,
		proxyCh:        proxyCh,
		ServerConnFunc: serverConnFunc,
	}
}

func NewMcAnswerRealIPProxy(proxyCh chan ProxyAction, serverConnFunc func() (net.Conn, error)) McAnswerBasic {
	return McAnswerBasic{
		action:         PROXY,
		proxyCh:        proxyCh,
		ServerConnFunc: serverConnFunc,
		upgradeRealIp:  true,
	}
}

func NewMcAnswerClose() McAnswer {
	return McAnswerBasic{
		action: CLOSE,
	}
}

func NewMcAnswerDisonncet(pk1 mc.Packet) McAnswer {
	return McAnswerBasic{
		action:      DISCONNECT,
		firstPacket: pk1,
	}
}
func NewMcAnswerStatus(pk1 mc.Packet, latency time.Duration) McAnswer {
	return McAnswerBasic{
		action:      SEND_STATUS,
		firstPacket: pk1,
		latency:     latency,
	}
}

type McAnswer interface {
	FirstPacket() mc.Packet
	ServerConn() (net.Conn, error)
	ProxyCh() chan ProxyAction
	Latency() time.Duration
	Action() McAction
	UpdateToRealIp() bool
}

type McAnswerBasic struct {
	ServerConnFunc func() (net.Conn, error)
	action         McAction
	proxyCh        chan ProxyAction
	latency        time.Duration
	upgradeRealIp  bool

	firstPacket mc.Packet
}

func (ans McAnswerBasic) ServerConn() (net.Conn, error) {
	return ans.ServerConnFunc()
}
func (ans McAnswerBasic) FirstPacket() mc.Packet {
	return ans.firstPacket
}
func (ans McAnswerBasic) ProxyCh() chan ProxyAction {
	return ans.proxyCh
}
func (ans McAnswerBasic) Latency() time.Duration {
	return ans.latency
}
func (ans McAnswerBasic) Action() McAction {
	return ans.action
}
func (ans McAnswerBasic) UpdateToRealIp() bool {
	return ans.upgradeRealIp
}

func ServeListener(listener net.Listener, reqCh chan McRequest) {
	for {
		conn, err := listener.Accept()
		if err != nil {
			if errors.Is(err, net.ErrClosed) {
				log.Printf("net.Listener was closed, stopping with accepting calls")
				break
			}
			log.Println(err)
			continue
		}
		go ReadConnection(conn, reqCh)
	}
}

func ReadConnection(c net.Conn, reqCh chan McRequest) {
	conn := NewMcConn(c)
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
		c.Close()
		return
	}
	serverAddr := string(handshake.ServerAddress)
	isLoginReq := false
	requestType := STATUS
	if handshake.NextState == mc.HandshakeLoginState {
		isLoginReq = true
		requestType = LOGIN
	}

	//Write tests for this
	if handshake.IsForgeAddress() || handshake.IsRealIPAddress() {
		serverAddr = handshake.ParseServerAddress()
	}

	ansCh := make(chan McAnswer)
	req := McRequest{
		Type:       requestType,
		Ch:         ansCh,
		Addr:       c.RemoteAddr(),
		ServerAddr: serverAddr,
	}

	var loginPacket mc.Packet
	if isLoginReq {
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

	switch ans.Action() {
	case PROXY:
		sConn, err := ans.ServerConn()
		if err != nil {
			log.Printf("Err when creating server connection: %v", err)
			c.Close()
			return
		}
		serverConn := NewMcConn(sConn)
		if ans.UpdateToRealIp() {
			handshake.UpgradeToRealIP(c.RemoteAddr().String())
			handshakePacket = handshake.Marshal()
		}
		serverConn.WritePacket(handshakePacket)
		switch requestType {
		case LOGIN:
			serverConn.WritePacket(loginPacket)
			go func(client, server net.Conn, proxyCh chan ProxyAction) {
				ProxyLogin(client, server, proxyCh)
			}(c, sConn, ans.ProxyCh())
		case STATUS:
			// For some unknown reason if we dont send this here
			//  its goes wrong with proxying status requests
			serverConn.WritePacket(mc.Packet{ID: 0x00})
			go func(client, server net.Conn, proxyCh chan ProxyAction) {
				ProxyStatus(client, server, proxyCh)
			}(c, sConn, ans.ProxyCh())
		}
	case DISCONNECT:
		conn.WritePacket(ans.FirstPacket())
		c.Close()
	case SEND_STATUS:
		conn.ReadPacket()
		conn.WritePacket(ans.FirstPacket())
		pingPk, _ := conn.ReadPacket()
		if ans.Latency() != 0 {
			time.Sleep(ans.Latency())
		}
		conn.WritePacket(pingPk)
		c.Close()
	case CLOSE:
		c.Close()
	}

}

func ProxyLogin(client, server net.Conn, proxyCh chan ProxyAction) {
	proxyCh <- PROXY_OPEN
	// Close behavior doesnt seem to work that well
	go func() {
		io.Copy(server, client)
		client.Close()
	}()
	io.Copy(client, server)
	server.Close()
	proxyCh <- PROXY_CLOSE
}

func ProxyStatus(client, server net.Conn, proxyCh chan ProxyAction) {
	proxyCh <- PROXY_OPEN
	go func() {
		pipe(server, client)
		client.Close()
	}()
	pipe(client, server)
	server.Close()
	proxyCh <- PROXY_CLOSE
}

func pipe(c1, c2 net.Conn) {
	buffer := make([]byte, 0xffff)
	for {
		n, err := c1.Read(buffer)
		if err != nil {
			return
		}
		_, err = c2.Write(buffer[:n])
		if err != nil {
			return
		}
	}
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
