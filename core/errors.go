package core

import "errors"

var (
	ErrClientToSlow      = errors.New("client was to slow with sending its packets")
	ErrClientClosedConn  = errors.New("client closed the connection")
	ErrNoServerFound     = errors.New("could not find server")
	ErrNoServerConn      = errors.New("could not find server")
	ErrNotValidHandshake = errors.New("not a valid handshake state")
	ErrOverConnRateLimit = errors.New("too many request within rate limit time frame")
	ErrStatusPing        = errors.New("something went wrong while pinging")
)
