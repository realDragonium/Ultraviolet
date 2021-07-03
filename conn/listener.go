package conn

import "net"

func ServeListener(listener net.Listener) {
	listener.Accept()
}
