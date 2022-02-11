package ultraviolet

import (
	"errors"
	"net"
	"time"

	"github.com/pires/go-proxyproto"
	"github.com/realDragonium/Ultraviolet/config"
	"github.com/realDragonium/Ultraviolet/core"
	"github.com/realDragonium/Ultraviolet/mc"
)

var ErrNoServerFound = errors.New("could not find server")

type ServerAction byte

const (
	DISCONNECT ServerAction = iota
	DEFAULT_STATUS
	STATUS_CACHED
	VERIFY_CONN
	PROXY
	PROXY_REALIP_2_4
	PROXY_REALIP_2_5
)

type Server interface {
	ConnAction(req core.RequestData) (ServerAction, error)
	CreateConn(req core.RequestData) (net.Conn, error)
	CachedStatus() mc.Packet
}

type ProxyAllServer struct {
}

func (s ProxyAllServer) ConnAction(req core.RequestData) (ServerAction, error) {
	return PROXY, nil
}

func (s ProxyAllServer) CreateConn(req core.RequestData) (net.Conn, error) {
	return &net.TCPConn{}, nil
}

func (s ProxyAllServer) CachedStatus() mc.Packet {
	return mc.Packet{}
}

func NewConfigServer(cfg config.APIServerConfig) APIServer {
	dialTimeout, err := time.ParseDuration(cfg.DialTimeout)
	if err != nil {
		dialTimeout = time.Second
	}

	dialer := net.Dialer{
		Timeout: dialTimeout,
		LocalAddr: &net.TCPAddr{
			IP: net.ParseIP(cfg.ProxyBind),
		},
	}

	disconnectPacket := mc.ClientBoundDisconnect{
		Reason: mc.String(cfg.DisconnectMessage),
	}.Marshal()

	cachedStatusPk := cfg.CachedStatus.Marshal()

	serverState := core.Offline
	if cfg.IsOnline {
		serverState = core.Online
	}

	return APIServer{
		sendProxyProtocol: cfg.SendProxyProtocol,
		disconnectPacket:  disconnectPacket,
		dialer:            dialer,
		useStatusCache:    cfg.UseStatusCache,
		serverStatusPk:    cachedStatusPk,
		serverStatus:      serverState,
	}
}

type APIServer struct {
	sendProxyProtocol bool
	disconnectPacket  mc.Packet

	dialer  net.Dialer
	proxyTo string

	useStatusCache bool
	serverStatusPk mc.Packet
	serverStatus   core.ServerState
}

func (server APIServer) ConnAction(req core.RequestData) (ServerAction, error) {
	switch server.serverStatus {
	case core.Offline:
		return server.serverOffline(req)
	case core.Online:
		return server.serverOnline(req)
	default:
		return DISCONNECT, nil
	}
}

func (server APIServer) serverOffline(req core.RequestData) (ServerAction, error) {
	if req.Type == mc.Status {
		return STATUS_CACHED, nil
	}

	return DISCONNECT, nil
}

func (server APIServer) serverOnline(req core.RequestData) (ServerAction, error) {
	if req.Type == mc.Login {
		// if cfgServer.useRealipv2_4 {
		// 	return PROXY_REALIP_2_4, nil
		// }
		// if cfgServer.useRealipv2_5 {
		// 	return PROXY_REALIP_2_5, nil
		// }
		return PROXY, nil
	}

	if req.Type == mc.Status {
		if server.useStatusCache {
			return STATUS_CACHED, nil
		}

		return PROXY, nil
	}

	return DISCONNECT, nil
}

func (server APIServer) CreateConn(req core.RequestData) (conn net.Conn, err error) {
	conn, err = server.dialer.Dial("tcp", server.proxyTo)
	if err != nil {
		return
	}

	if server.sendProxyProtocol {
		header := &proxyproto.Header{
			Version:           2,
			Command:           proxyproto.PROXY,
			TransportProtocol: proxyproto.TCPv4,
			SourceAddr:        conn.LocalAddr(),
			DestinationAddr:   conn.RemoteAddr(),
		}

		_, err = header.WriteTo(conn)
	}

	return
}

func (server APIServer) CachedStatus() mc.Packet {
	return server.serverStatusPk
}
