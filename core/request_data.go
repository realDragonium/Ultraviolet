package core

import (
	"net"

	"github.com/realDragonium/Ultraviolet/mc"
)

type RequestData struct {
	Type       mc.HandshakeState
	Handshake  mc.ServerBoundHandshake
	ServerAddr string
	Addr       net.Addr
	Username   string
}
