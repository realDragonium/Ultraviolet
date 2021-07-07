package proxy

import (
	"errors"
	"log"
	"net"
	"time"

	"github.com/pires/go-proxyproto"
	"github.com/realDragonium/Ultraviolet/mc"
)

var (
	ErrCantWriteToServer     = errors.New("can't write to proxy target")
	ErrCantWriteToClient     = errors.New("can't write to client")
	ErrCantConnectWithServer = errors.New("cant connect with server")

	dialTimeout = 1000 * time.Millisecond
)

type ServerState byte

const (
	// By default every server is online
	ONLINE ServerState = iota
	OFFLINE
	UNKNOWN
)

func newServerConn(cfg WorkerServerConfig) (net.Conn, error) {
	dialer := net.Dialer{
		Timeout: dialTimeout,
		LocalAddr: &net.TCPAddr{
			// TODO: Add something for invalid ProxyBind ips
			IP: net.ParseIP(cfg.ProxyBind),
		},
	}
	serverConn, err := dialer.Dial("tcp", cfg.ProxyTo)
	if err != nil {
		return serverConn, err
	}

	return serverConn, nil
}

// What should the worker know?
// - All domains and their target
// - Default Status
// - some way to cache motd status from a server  (probably separate worker)
// - Some anti bot thing (probably separate worker)
// - Some way to create connections with backend server (probably separate worker)
// -
func SomethingElse() {

}

type WorkerServerConfig struct {
	State ServerState

	OfflineStatus    mc.Packet
	DisconnectPacket mc.Packet

	ProxyTo           string
	ProxyBind         string
	SendProxyProtocol bool

	ConnLimitBackend int
}

func NewWorker(req chan ConnRequest, proxies map[string]WorkerServerConfig, defaultStatus mc.Packet) BasicWorker {
	return BasicWorker{
		reqCh:         req,
		defaultStatus: defaultStatus,
		Servers:       proxies,
	}
}

type BasicWorker struct {
	reqCh         chan ConnRequest
	defaultStatus mc.Packet
	Servers       map[string]WorkerServerConfig
}

func (w BasicWorker) BasicWork() {
	// See or this can be 'efficienter' when the for loop calls the work method
	//  instead of the work method containing the for loop (something with allocation)
	for {
		request := <-w.reqCh
		serverCfg, ok := w.Servers[request.ServerAddr]
		if !ok {
			//Unknown server address
			switch request.Type {
			case STATUS:
				request.Ch <- ConnAnswer{
					Action:   SEND_STATUS,
					StatusPk: w.defaultStatus,
				}
			case LOGIN:
				request.Ch <- ConnAnswer{
					Action: CLOSE,
				}
			}
			return
		}
		switch request.Type {
		case STATUS:
			if serverCfg.State == ONLINE {
				action := PROXY
				serverConn, err := newServerConn(serverCfg)
				if err != nil {
					log.Println(err)
					action = ERROR
				}
				serverMcConn := NewMcConn(serverConn)
				request.Ch <- ConnAnswer{
					Action:     action,
					ServerConn: serverMcConn,
				}
				return
			}

			var statusPk mc.Packet
			if serverCfg.State == OFFLINE {
				statusPk = serverCfg.OfflineStatus
			}
			request.Ch <- ConnAnswer{
				Action:   SEND_STATUS,
				StatusPk: statusPk,
			}
		case LOGIN:
			if serverCfg.State == OFFLINE {
				request.Ch <- ConnAnswer{
					Action:        DISCONNECT,
					DisconMessage: serverCfg.DisconnectPacket,
				}
				return
			}
			serverConn, err := newServerConn(serverCfg)
			if err != nil {
				log.Println(err)
			}

			if serverCfg.SendProxyProtocol {
				header := &proxyproto.Header{
					Version:           2,
					Command:           proxyproto.PROXY,
					TransportProtocol: proxyproto.TCPv4,
					SourceAddr:        request.Addr,
					DestinationAddr:   serverConn.RemoteAddr(),
				}

				if _, err = header.WriteTo(serverConn); err != nil {
					//Handle error
					log.Println(err)
				}
			}
			serverMcConn := NewMcConn(serverConn)
			request.Ch <- ConnAnswer{
				Action:     PROXY,
				ServerConn: serverMcConn,
			}
		}
	}
}

type ConnServerConfig struct {
	State ServerState

	OfflineStatus    mc.Packet
	DisconnectPacket mc.Packet

	ProxyTo           string
	ProxyBind         string
	SendProxyProtocol bool

	ConnLimitBackend int
}

type ConnWorker struct {

}

type StatusServerConfig struct {
	State ServerState

	OnlineStatus     mc.Packet
	OfflineStatus    mc.Packet
	DisconnectPacket mc.Packet
}

type StatusWorker struct {
}
