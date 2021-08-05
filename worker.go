package ultraviolet

import (
	"bufio"
	"io"
	"log"
	"net"
	"time"

	"github.com/prometheus/client_golang/prometheus"
	"github.com/prometheus/client_golang/prometheus/promauto"
	"github.com/realDragonium/Ultraviolet/config"
	"github.com/realDragonium/Ultraviolet/mc"
)

const (
	maxHandshakeLength int = 264 // 264 -> 'max handshake packet length' + 1
	// packetLength:2 + packet ID: 1 + protocol version:2 + max string length:255 + port:2 + state: 1 -> 2+1+2+255+2+1 = 263
)

var (
	newConnections     = promauto.NewCounterVec(prometheus.CounterOpts{}, []string{"unknown", "login", "status"})
	processConnections = promauto.NewCounterVec(prometheus.CounterOpts{}, []string{"proxy", "disconnect", "send_status", "close", "error"})
)

func NewWorker(cfg config.WorkerConfig, reqCh chan net.Conn) BasicWorker {
	dict := make(map[string]chan BackendRequest)
	defaultStatusPk := cfg.DefaultStatus.Marshal()
	return BasicWorker{
		ReqCh:         reqCh,
		defaultStatus: defaultStatusPk,
		serverDict:    dict,
	}
}

type BasicWorker struct {
	ReqCh         chan net.Conn
	defaultStatus mc.Packet

	serverDict map[string]chan BackendRequest
}

func (r *BasicWorker) RegisterBackendWorker(key string, worker BackendWorker) {
	r.serverDict[key] = worker.ReqCh
}

// TODO:
// - add more tests with this method
func (r *BasicWorker) Work() {
	var err error
	var conn net.Conn
	var req BackendRequest
	var ans ProcessAnswer

	for {
		conn = <-r.ReqCh
		req, err = r.ProcessConnection(conn)
		if err != nil {
			conn.Close()
			return
		}
		ans = r.ProcessRequest(req)
		log.Printf("%v request from %v will take action: %v", req.Type, conn.RemoteAddr(), ans.action)
		r.ProcessAnswer(conn, ans)
	}
}

func (r *BasicWorker) NotSafeYet_ProcessConnection(conn net.Conn) (BackendRequest, error) {
	//  TODO: When handshake gets too long stuff goes wrong, prevent is from crashing when that happens
	b := bufio.NewReaderSize(conn, maxHandshakeLength)
	handshake, err := mc.ReadPacket3_Handshake(b)
	if err != nil {
		log.Printf("error parsing handshake from %v - error: %v", conn.RemoteAddr(), err)
	}
	t := mc.RequestState(handshake.NextState)
	if t == mc.UNKNOWN_STATE {
		return BackendRequest{}, ErrNotValidHandshake
	}
	request := BackendRequest{
		Type:       t,
		ServerAddr: handshake.ParseServerAddress(),
		Addr:       conn.RemoteAddr(),
		Handshake:  handshake,
	}

	packet, _ := mc.ReadPacket3(b)
	if t == mc.LOGIN {
		loginStart, _ := mc.UnmarshalServerBoundLoginStart(packet)
		request.Username = string(loginStart.Name)
	}
	return request, nil
}

// TODO:
// - Add IO deadlines
// - Adding some more error tests
func (r *BasicWorker) ProcessConnection(conn net.Conn) (BackendRequest, error) {
	mcConn := mc.NewMcConn(conn)
	handshakePacket, err := mcConn.ReadPacket()
	if err != nil {
		log.Printf("error while reading handshake: %v", err)
	}
	handshake, err := mc.UnmarshalServerBoundHandshake(handshakePacket)
	if err != nil {
		log.Printf("error while parsing handshake: %v", err)
	}
	reqType := mc.RequestState(handshake.NextState)
	newConnections.WithLabelValues(reqType.String()).Inc()
	if reqType == mc.UNKNOWN_STATE {
		return BackendRequest{}, ErrNotValidHandshake
	}
	request := BackendRequest{
		Type:       reqType,
		ServerAddr: handshake.ParseServerAddress(),
		Addr:       conn.RemoteAddr(),
		Handshake:  handshake,
	}
	packet, err := mcConn.ReadPacket()
	if err != nil {
		log.Printf("error while reading second packet: %v", err)
	}
	if reqType == mc.LOGIN {
		loginStart, err := mc.UnmarshalServerBoundLoginStart(packet)
		if err != nil {
			log.Printf("error while parsing login packet: %v", err)
		}
		request.Username = string(loginStart.Name)
	}

	return request, nil
}

func (w *BasicWorker) ProcessRequest(req BackendRequest) ProcessAnswer {
	ch, ok := w.serverDict[req.ServerAddr]
	if !ok {
		if req.Type == mc.STATUS {
			return ProcessAnswer{
				action:      SEND_STATUS,
				firstPacket: w.defaultStatus,
			}
		}
		return ProcessAnswer{
			action: CLOSE,
		}
	}

	req.Ch = make(chan ProcessAnswer)
	ch <- req
	return <-req.Ch
}

func (w *BasicWorker) ProcessAnswer(conn net.Conn, ans ProcessAnswer) {
	processConnections.WithLabelValues(ans.Action().String()).Inc()
	clientMcConn := mc.NewMcConn(conn)
	switch ans.action {
	case PROXY:
		sConn, err := ans.serverConnFunc()
		if err != nil {
			log.Printf("Err when creating server connection: %v", err)
			conn.Close()
			return
		}
		mcServerConn := mc.NewMcConn(sConn)
		mcServerConn.WritePacket(ans.Response())
		mcServerConn.WritePacket(ans.Response2())
		go func(client, server net.Conn, proxyCh chan ProxyAction) {
			proxyCh <- PROXY_OPEN
			ProxyConnection(client, server)
			proxyCh <- PROXY_CLOSE
		}(conn, sConn, ans.ProxyCh())
	case DISCONNECT:
		clientMcConn.WritePacket(ans.Response())
		conn.Close()
	case SEND_STATUS:
		clientMcConn.WritePacket(ans.Response())
		pingPacket, err := clientMcConn.ReadPacket()
		if err != nil {
			conn.Close()
			return
		}
		if ans.Latency() != 0 {
			time.Sleep(ans.Latency())
		}
		clientMcConn.WritePacket(pingPacket)
		conn.Close()
	case CLOSE:
		conn.Close()
	}

}

func Proxy_IOCopy(client, server net.Conn) {
	// Close behavior doesnt seem to work that well
	go func() {
		io.Copy(server, client)
		client.Close()
	}()
	io.Copy(client, server)
	server.Close()
}

// TODO:
// - check or servers close the connection when they disconnect players if not add something to prevent abuse
func ProxyConnection(client, server net.Conn) {
	go func() {
		pipe(server, client)
		client.Close()
	}()
	pipe(client, server)
	server.Close()
}

func pipe(c1, c2 net.Conn) {
	buffer := make([]byte, 0xffff)
	for {
		n, err := c1.Read(buffer)
		if err != nil {
			return
		}
		_, err = c2.Write(buffer[:n])
		if err != nil {
			return
		}
	}
}
