package worker

import (
	"net"

	ultraviolet "github.com/realDragonium/Ultraviolet"
	"github.com/realDragonium/Ultraviolet/mc"
)

//go:generate stringer -type=BackendAction
type BackendAction byte

const (
	Error BackendAction = iota
	Proxy
	Disconnect
	SendStatus
	Close
)

//go:generate stringer -type=ProxyAction
type ProxyAction int8

const (
	ProxyOpen ProxyAction = iota
	ProxyClose
)

type BackendRequest struct {
	// Type       mc.HandshakeState
	// Handshake  mc.ServerBoundHandshake
	// ServerAddr string
	// Addr       net.Addr
	// Username   string
	ReqData ultraviolet.RequestData
	Ch      chan<- BackendAnswer
}

type BackendAnswer struct {
	ServerName     string
	serverConnFunc func() (net.Conn, error)
	action         BackendAction
	proxyCh        chan ProxyAction

	firstPacket  mc.Packet
	secondPacket mc.Packet
}

func NewDisconnectAnswer(p mc.Packet) BackendAnswer {
	return BackendAnswer{
		action:      Disconnect,
		firstPacket: p,
	}
}

func NewStatusAnswer(p mc.Packet) BackendAnswer {
	return BackendAnswer{
		action:      SendStatus,
		firstPacket: p,
	}
}

func NewProxyAnswer(p1, p2 mc.Packet, proxyCh chan ProxyAction, connFunc func() (net.Conn, error)) BackendAnswer {
	return BackendAnswer{
		action:         Proxy,
		serverConnFunc: connFunc,
		firstPacket:    p1,
		secondPacket:   p2,
		proxyCh:        proxyCh,
	}
}

func NewCloseAnswer() BackendAnswer {
	return BackendAnswer{
		action: Close,
	}
}

func (ans BackendAnswer) ServerConn() (net.Conn, error) {
	return ans.serverConnFunc()
}
func (ans BackendAnswer) Response() mc.Packet {
	return ans.firstPacket
}
func (ans BackendAnswer) Response2() mc.Packet {
	return ans.secondPacket
}
func (ans BackendAnswer) ProxyCh() chan ProxyAction {
	return ans.proxyCh
}
func (ans BackendAnswer) Action() BackendAction {
	return ans.action
}
