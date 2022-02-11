package core

import "errors"

var (
	ErrNotValidHandshake = errors.New("not a valid handshake state")
	ErrClientToSlow      = errors.New("client was to slow with sending its packets")
	ErrClientClosedConn  = errors.New("client closed the connection")
	ErrNoServerFound     = errors.New("could not find server")
	ErrNoServerConn      = errors.New("could not find server")
)
