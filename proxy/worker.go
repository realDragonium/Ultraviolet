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

type connectionData struct {
	ProxyTo           string
	ProxyBind         string
	SendProxyProtocol bool
}

func newServerConn(cfg connectionData) (net.Conn, error) {
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

func NewWorker(req chan McRequest, proxies map[string]WorkerServerConfig, defaultStatus mc.Packet) Worker {
	return BasicWorker{
		reqCh:         req,
		defaultStatus: defaultStatus,
		servers:       proxies,
	}
}

type BasicWorker struct {
	reqCh         chan McRequest
	defaultStatus mc.Packet
	servers       map[string]WorkerServerConfig
}

func (w BasicWorker) Work() {
	// See or this can be 'efficienter' when the for loop calls the work method
	//  instead of the work method containing the for loop (something with allocation)
	for {
		request := <-w.reqCh
		cfg, ok := w.servers[request.ServerAddr]
		if !ok {
			//Unknown server address
			switch request.Type {
			case STATUS:
				request.Ch <- McAnswer{
					Action:   SEND_STATUS,
					StatusPk: w.defaultStatus,
				}
			case LOGIN:
				request.Ch <- McAnswer{
					Action: CLOSE,
				}
			}
			return
		}
		switch request.Type {
		case STATUS:
			if cfg.State == ONLINE {
				action := PROXY
				serverConn, err := newServerConn(connectionData{
					ProxyTo:           cfg.ProxyTo,
					ProxyBind:         cfg.ProxyBind,
					SendProxyProtocol: cfg.SendProxyProtocol,
				})
				if err != nil {
					log.Println(err)
					action = ERROR
				}
				serverMcConn := NewMcConn(serverConn)
				request.Ch <- McAnswer{
					Action:     action,
					ServerConn: serverMcConn,
				}
				return
			}

			var statusPk mc.Packet
			if cfg.State == OFFLINE {
				statusPk = cfg.OfflineStatus
			}
			request.Ch <- McAnswer{
				Action:   SEND_STATUS,
				StatusPk: statusPk,
			}
		case LOGIN:
			if cfg.State == OFFLINE {
				request.Ch <- McAnswer{
					Action:        DISCONNECT,
					DisconMessage: cfg.DisconnectPacket,
				}
				return
			}
			serverConn, err := newServerConn(connectionData{
				ProxyTo:           cfg.ProxyTo,
				ProxyBind:         cfg.ProxyBind,
				SendProxyProtocol: cfg.SendProxyProtocol,
			})
			if err != nil {
				log.Println(err)
			}

			if cfg.SendProxyProtocol {
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
			request.Ch <- McAnswer{
				Action:     PROXY,
				ServerConn: serverMcConn,
			}
		}
	}
}

func NewConnWorker(reqCh chan ConnRequest, proxies map[string]WorkerServerConfig) Worker {
	servers := make(map[string]connectionData)
	for id, proxy := range proxies {
		servers[id] = connectionData{
			ProxyTo:           proxy.ProxyTo,
			ProxyBind:         proxy.ProxyBind,
			SendProxyProtocol: proxy.SendProxyProtocol,
		}

	}
	return ConnWorker{
		reqCh:   reqCh,
		servers: servers,
	}
}

type ConnRequest struct {
	answerCh          chan ConnAnswer
	clientAddr        net.Addr
	serverId          string
	sendProxyProtocol bool
}

type ConnAnswer struct {
	ServerConn McConn
}

// Needs conn rate limiter later
type ConnWorker struct {
	reqCh   chan ConnRequest
	servers map[string]connectionData
}

// If it got here it should already be checked or the proxy has been registered
func (w ConnWorker) Work() {
	for {
		request := <-w.reqCh
		cfg := w.servers[request.serverId]
		serverConn, _ := newServerConn(cfg)
		if request.sendProxyProtocol {
			header := &proxyproto.Header{
				Version:           2,
				Command:           proxyproto.PROXY,
				TransportProtocol: proxyproto.TCPv4,
				SourceAddr:        request.clientAddr,
				DestinationAddr:   serverConn.RemoteAddr(),
			}
			header.WriteTo(serverConn)
		}
		serverMcConn := NewMcConn(serverConn)
		request.answerCh <- ConnAnswer{
			ServerConn: serverMcConn,
		}
	}
}

func NewStatusWorker(reqCh chan StatusRequest, proxies map[string]WorkerServerConfig) Worker {
	servers := make(map[string]StatusServerData)
	for id, proxy := range proxies {
		servers[id] = StatusServerData{
			OfflineStatus: proxy.OfflineStatus,
			OnlineStatus:  mc.Packet{},
		}

	}
	return StatusWorker{
		reqCh:   reqCh,
		servers: servers,
	}
}

type StatusRequest struct {
	serverId string
	state    ServerState
	answerCh chan mc.Packet
}

type StatusServerData struct {
	OnlineStatus  mc.Packet
	OfflineStatus mc.Packet
}

// TODO:
// - add online status caching when doesnt proxy client requests to actual server
type StatusWorker struct {
	reqCh   chan StatusRequest
	servers map[string]StatusServerData
}

func (w StatusWorker) Work() {
	for {
		request := <-w.reqCh
		cfg := w.servers[request.serverId]
		var statusPk mc.Packet
		switch request.state {
		case ONLINE:
			statusPk = cfg.OnlineStatus
		case OFFLINE:
			statusPk = cfg.OfflineStatus
		}
		request.answerCh <- statusPk
	}
}

func NewStateWorker(reqCh chan StateRequest, proxies map[string]WorkerServerConfig) Worker {
	servers := make(map[string]StateServerData)
	for id, proxy := range proxies {
		servers[id] = StateServerData{
			state: proxy.State,
		}

	}
	return StateWorker{
		reqCh:   reqCh,
		servers: servers,
	}
}

type StateRequest struct {
	serverId string
	answerCh chan StateServerData
}

type StateServerData struct {
	state ServerState
}

// TODO:
// - automatically update state every x amount of time
// - check or state is still valid or expired before replying
type StateWorker struct {
	reqCh   chan StateRequest
	servers map[string]StateServerData
}

func (w StateWorker) Work() {
	for {
		request := <-w.reqCh
		cfg := w.servers[request.serverId]
		request.answerCh <- cfg
	}
}
