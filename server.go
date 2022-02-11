package ultraviolet

import (
	"errors"
	"net"
	"time"

	"github.com/pires/go-proxyproto"
	"github.com/realDragonium/Ultraviolet/config"
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
	ConnAction(req RequestData) (ServerAction, error)
	CreateConn(req RequestData) (net.Conn, error)
	CachedStatus() mc.Packet
}

type ProxyAllServer struct {
}

func (s ProxyAllServer) ConnAction(req RequestData) (ServerAction, error) {
	return PROXY, nil
}

func (s ProxyAllServer) CreateConn(req RequestData) (net.Conn, error) {
	return &net.TCPConn{}, nil
}

func (s ProxyAllServer) CachedStatus() mc.Packet {
	return mc.Packet{}
}

func NewConfigServer(cfg config.APIServerConfig) configServer {
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

	serverState := Offline
	if cfg.IsOnline {
		serverState = Online
	}

	return configServer{
		sendProxyProtocol: cfg.SendProxyProtocol,
		disconnectPacket:  disconnectPacket,
		dialer:            dialer,
		useStatusCache:    cfg.UseStatusCache,
		serverStatusPk:    cachedStatusPk,
		serverStatus:      serverState,
	}
}

type configServer struct {
	sendProxyProtocol bool
	disconnectPacket  mc.Packet

	dialer  net.Dialer
	proxyTo string

	useStatusCache bool
	serverStatusPk mc.Packet
	serverStatus   ServerState
}

func (cfgServer configServer) ConnAction(req RequestData) (ServerAction, error) {
	switch cfgServer.serverStatus {
	case Offline:
		return cfgServer.serverOffline(req)
	case Online:
		return cfgServer.serverOnline(req)
	default:
		return DISCONNECT, nil
	}
}

func (cfgServer configServer) serverOffline(req RequestData) (ServerAction, error) {
	if req.Type == mc.Status {
		return STATUS_CACHED, nil
	}

	return DISCONNECT, nil
}

func (cfgServer configServer) serverOnline(req RequestData) (ServerAction, error) {
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
		if cfgServer.useStatusCache {
			return STATUS_CACHED, nil
		}

		return PROXY, nil
	}

	return DISCONNECT, nil
}

func (cfgServer configServer) CreateConn(req RequestData) (conn net.Conn, err error) {
	conn, err = cfgServer.dialer.Dial("tcp", cfgServer.proxyTo)
	if err != nil {
		return
	}

	if cfgServer.sendProxyProtocol {
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

func (cfgServer configServer) CachedStatus() mc.Packet {
	return cfgServer.serverStatusPk
}
