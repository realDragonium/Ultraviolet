package core

import (
	"net"

	"github.com/realDragonium/Ultraviolet/mc"
)

type Server interface {
	ConnAction(req RequestData) ServerAction
	CreateConn(req RequestData) (c net.Conn, err error)
	Status() mc.Packet
}

type ServerAction byte

const (
	DISCONNECT ServerAction = iota
	DEFAULT_STATUS
	STATUS
	VERIFY_CONN
	PROXY
	PROXY_REALIP_2_4
	PROXY_REALIP_2_5
)

//go:generate stringer -type=ServerState
type ServerState byte

const (
	Unknown ServerState = iota
	Online
	Offline
)
