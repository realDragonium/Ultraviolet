package proxy

import (
	"errors"
	"log"
	"net"
	"time"

	"github.com/pires/go-proxyproto"
	"github.com/realDragonium/Ultraviolet/conn"
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

type UnknownServer struct {
	Status mc.Packet
}

type WorkerServerConfig struct {
	//Adding domains temporarily for testing until better structure
	MainDomain   string
	ExtraDomains []string

	OnlineStatus  mc.Packet
	OfflineStatus mc.Packet
	State         ServerState

	ProxyTo           string
	ProxyBind         string
	SendProxyProtocol bool
	DisconnectMessage string

	ConnLimitBackend int
}

func NewWorker(req chan conn.ConnRequest) Worker {
	return Worker{
		ReqCh: req,
	}

}

type Worker struct {
	ReqCh      chan conn.ConnRequest
	DefaultCfg UnknownServer
	Servers    map[string]WorkerServerConfig
}

func (w Worker) Work() {
	for {
		request := <-w.ReqCh
		serverCfg, ok := w.Servers[request.ServerAddr]
		if !ok {
			//Unknown server address
			switch request.Type {
			case conn.STATUS:
				request.Ch <- conn.ConnAnswer{
					Action:   conn.SEND_STATUS,
					StatusPk: w.DefaultCfg.Status,
				}
			case conn.LOGIN:
				request.Ch <- conn.ConnAnswer{
					Action: conn.CLOSE,
				}
			}
			return
		}
		switch request.Type {
		case conn.STATUS:
			statusPk := serverCfg.OnlineStatus
			if serverCfg.State == OFFLINE {
				statusPk = serverCfg.OfflineStatus
			}
			request.Ch <- conn.ConnAnswer{
				Action:   conn.SEND_STATUS,
				StatusPk: statusPk,
			}
		case conn.LOGIN:
			if serverCfg.State == OFFLINE {
				request.Ch <- conn.ConnAnswer{
					Action: conn.CLOSE,
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
			serverMcConn := conn.NewMcConn(serverConn)
			request.Ch <- conn.ConnAnswer{
				Action:     conn.PROXY,
				ServerConn: serverMcConn,
			}
		}
	}
}
