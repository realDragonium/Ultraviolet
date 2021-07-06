package proxy

import (
	"errors"
	"net"
	"time"

	"github.com/pires/go-proxyproto"
	"github.com/realDragonium/Ultraviolet/config"
)

var (
	ErrCantWriteToServer     = errors.New("can't write to proxy target")
	ErrCantWriteToClient     = errors.New("can't write to client")
	ErrCantConnectWithServer = errors.New("cant connect with server")

	dialTimeout = time.Duration(1000) * time.Millisecond
)

func Something(cfg config.BackendConnConfig) (net.Conn, error) {
	var targetIp net.Addr
	dialer := net.Dialer{
		Timeout: dialTimeout,
		LocalAddr: &net.TCPAddr{
			IP: net.ParseIP(cfg.ProxyBind),
		},
	}
	serverConn, err := dialer.Dial("tcp", cfg.ProxyTo)
	if err != nil {
		return serverConn, ErrCantConnectWithServer
	}

	if cfg.SendProxyProtocol {
		header := &proxyproto.Header{
			Version:           2,
			Command:           proxyproto.PROXY,
			TransportProtocol: proxyproto.TCPv4,
			SourceAddr:        targetIp,
			DestinationAddr:   serverConn.RemoteAddr(),
		}

		if _, err = header.WriteTo(serverConn); err != nil {
			return serverConn, ErrCantWriteToServer
		}
	}

	return serverConn, nil
}


func Worker(){
	
}