package ultraviolet

import (
	"bufio"
	"errors"
	"io"
	"log"
	"net"
	"os"
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
	ErrClientToSlow     = errors.New("client was to slow with sending its packets")
	ErrClientClosedConn = errors.New("client closed the connection")

	newConnections = promauto.NewCounterVec(prometheus.CounterOpts{
		Name: "ultraviolet_new_connections",
		Help: "The number of new connections created since started running",
	}, []string{"type"}) // which can be "unknown", "login" or "status"
	processedConnections = promauto.NewCounterVec(prometheus.CounterOpts{
		Name: "ultraviolet_processed_connections",
		Help: "The number actions taken for 'valid' connections",
	}, []string{"action"}) // which can be "proxy", "disconnect", "send_status", "close" or "error"
)

func NewWorker(cfg config.WorkerConfig, reqCh chan net.Conn) BasicWorker {
	dict := make(map[string]chan BackendRequest)
	defaultStatusPk := cfg.DefaultStatus.Marshal()
	return BasicWorker{
		reqCh:         reqCh,
		defaultStatus: defaultStatusPk,
		serverDict:    dict,
		ioTimeout:     cfg.IOTimeout,
	}
}

type serverData struct {
	domains []string
	ch chan checkOpenConns
}

type BasicWorker struct {
	reqCh           chan net.Conn
	defaultStatus   mc.Packet
	ioTimeout       time.Duration
	checkOpenConnCh chan checkOpenConns

	servers map[int]serverData
	serverDict map[string]chan BackendRequest
}

func (worker *BasicWorker) IODeadline() time.Time {
	return time.Now().Add(worker.ioTimeout)
}

func (r *BasicWorker) RegisterBackendWorker(key string, worker BackendWorker) {
	r.serverDict[key] = worker.ReqCh
}

func (worker *BasicWorker) AddActiveConnsCh(ch chan checkOpenConns) {
	worker.checkOpenConnCh = ch
}

// TODO:
// - add more tests with this method
func (r *BasicWorker) Work() {
	var err error
	var conn net.Conn
	var req BackendRequest
	var ans ProcessAnswer

	for {
		select {
		case conn = <-r.reqCh:
			req, err = r.ProcessConnection(conn)
			if err != nil {
				if errors.Is(err, ErrClientToSlow) {
					log.Printf("client %v was to slow with sending packet to us", conn.RemoteAddr())
				}
				conn.Close()
				continue
			}
			log.Printf("received connection from %v", conn.RemoteAddr())
			ans = r.ProcessRequest(req)
			log.Printf("%v request from %v will take action: %v", req.Type, conn.RemoteAddr(), ans.action)
			r.ProcessAnswer(conn, ans)
		case req := <-r.checkOpenConnCh:
			activeConns := false
			for _, data := range r.servers {
				ch := data.ch
				answerCh := make(chan bool)
				ch <- checkOpenConns{
					Ch: answerCh,
				}
				answer := <-answerCh
				if answer {
					activeConns = true
					break
				}
			}
			req.Ch <- activeConns
		}
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
// - Adding some more error tests
func (worker *BasicWorker) ProcessConnection(conn net.Conn) (BackendRequest, error) {
	mcConn := mc.NewMcConn(conn)
	conn.SetDeadline(worker.IODeadline())
	handshakePacket, err := mcConn.ReadPacket()
	if errors.Is(err, os.ErrDeadlineExceeded) {
		return BackendRequest{}, ErrClientToSlow
	} else if err != nil {
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

	conn.SetDeadline(worker.IODeadline())
	packet, err := mcConn.ReadPacket()
	if errors.Is(err, os.ErrDeadlineExceeded) {
		return BackendRequest{}, ErrClientToSlow
	} else if err != nil {
		log.Printf("error while reading second packet: %v", err)
		return BackendRequest{}, err
	}

	conn.SetDeadline(time.Time{})
	if reqType == mc.LOGIN {
		loginStart, err := mc.UnmarshalServerBoundLoginStart(packet)
		if err != nil {
			log.Printf("error while parsing login packet: %v", err)
			return BackendRequest{}, err
		}
		request.Username = string(loginStart.Name)
	}

	return request, nil
}

func (w *BasicWorker) ProcessRequest(req BackendRequest) ProcessAnswer {
	ch, ok := w.serverDict[req.ServerAddr]
	if !ok {
		log.Printf("didnt find a server for %v", req.ServerAddr)
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
	log.Printf("Found a matching server for %v", req.ServerAddr)
	req.Ch = make(chan ProcessAnswer)
	ch <- req
	return <-req.Ch
}

// TODO:
// - figure out or this need more deadlines
func (worker *BasicWorker) ProcessAnswer(conn net.Conn, ans ProcessAnswer) {
	processedConnections.WithLabelValues(ans.Action().String()).Inc()
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
		conn.SetDeadline(worker.IODeadline())
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
