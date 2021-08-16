package server

import "net"

type ConnectionCreator interface {
	Conn() func() (net.Conn, error)
}

type ConnectionCreatorFunc func() (net.Conn, error)

func (creator ConnectionCreatorFunc) Conn() func() (net.Conn, error) {
	return creator
}

func BasicConnCreator(proxyTo string, dialer net.Dialer) ConnectionCreatorFunc {
	return func() (net.Conn, error) {
		return dialer.Dial("tcp", proxyTo)
	}
}
