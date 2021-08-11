package ultraviolet

import (
	"bufio"
	"errors"
	"io"
	"log"
	"net"
	"os"
	"strings"
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

type UpdatableWorker interface {
	Update(data map[string]chan<- BackendRequest)
}

var (
	ErrClientToSlow     = errors.New("client was to slow with sending its packets")
	ErrClientClosedConn = errors.New("client closed the connection")

	ns             = "ultraviolet"
	newConnections = promauto.NewCounterVec(prometheus.CounterOpts{
		Namespace: ns,
		Name:      "request_total",
		Help:      "The number of new connections created since started running",
	}, []string{"type"}) // which can be "unknown", "login" or "status"
	processedConnections = promauto.NewCounterVec(prometheus.CounterOpts{
		Namespace: ns,
		Name:      "processed_connections",
		Help:      "The number actions taken for 'valid' connections",
	}, []string{"action", "server", "type"})

	requestBuckets  = []float64{.0001, .0005, .001, .005, .01, .05, .1, .5, 1, 5}
	processRequests = promauto.NewHistogramVec(prometheus.HistogramOpts{
		Namespace: ns,
		Name:      "request_duration_seconds",
		Help:      "Histogram request processing durations.",
		Buckets:   requestBuckets,
	}, []string{"action", "server", "type"})
)

func NewWorker(cfg config.WorkerConfig, reqCh chan net.Conn) BasicWorker {
	dict := make(map[string]chan<- BackendRequest)
	defaultStatusPk := cfg.DefaultStatus.Marshal()
	return BasicWorker{
		reqCh:         reqCh,
		defaultStatus: defaultStatusPk,
		serverDict:    dict,
		ioTimeout:     cfg.IOTimeout,
		closeCh:       make(chan struct{}),
		updateCh:      make(chan map[string]chan<- BackendRequest),
	}
}

type BasicWorker struct {
	reqCh    chan net.Conn
	closeCh  chan struct{}
	updateCh chan map[string]chan<- BackendRequest

	defaultStatus mc.Packet
	ioTimeout     time.Duration
	serverDict    map[string]chan<- BackendRequest
}

func (w *BasicWorker) IODeadline() time.Time {
	return time.Now().Add(w.ioTimeout)
}

func (w *BasicWorker) CloseCh() chan<- struct{} {
	return w.closeCh
}

func (w *BasicWorker) Update(data map[string]chan<- BackendRequest) {
	w.updateCh <- data
}

func (w *BasicWorker) SetServers(servers map[string]chan<- BackendRequest) {
	w.serverDict = servers
}

func (w *BasicWorker) KnowsDomain(domain string) bool {
	_, ok := w.serverDict[domain]
	return ok
}

// TODO:
// - add more tests with this method
func (bw *BasicWorker) Work() {
	unknownServer := "unknown"

	var err error
	var conn net.Conn
	var req BackendRequest
	var ans ProcessAnswer
	for {
		select {
		case conn = <-bw.reqCh:
			start := time.Now()
			req, err = bw.ProcessConnection(conn)
			if err != nil {
				if errors.Is(err, ErrClientToSlow) {
					log.Printf("client %v was to slow with sending packet to us", conn.RemoteAddr())
				}
				dur := time.Since(start).Seconds()
				labels := prometheus.Labels{"server": unknownServer, "type": req.Type.String(), "action": CLOSE.String()}
				processRequests.With(labels).Observe(dur)
				processedConnections.With(labels).Inc()
				conn.Close()
				continue
			}
			log.Printf("received connection from %v", conn.RemoteAddr())
			ans = bw.ProcessRequest(req)
			log.Printf("%v request from %v will take action: %v", req.Type, conn.RemoteAddr(), ans.action)
			bw.ProcessAnswer(conn, ans)
			dur := time.Since(start).Seconds()
			labels := prometheus.Labels{"server": ans.ServerName, "type": req.Type.String(), "action": ans.action.String()}
			processRequests.With(labels).Observe(dur)
			processedConnections.With(labels).Inc()
		case <-bw.closeCh:
			return
		case serverChs := <-bw.updateCh:
			bw.SetServers(serverChs)
		}
	}
}

func (bw *BasicWorker) NotSafeYet_ProcessConnection(conn net.Conn) (BackendRequest, error) {
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
func (bw *BasicWorker) ProcessConnection(conn net.Conn) (BackendRequest, error) {
	mcConn := mc.NewMcConn(conn)
	conn.SetDeadline(bw.IODeadline())
	handshakePacket, err := mcConn.ReadPacket()
	if errors.Is(err, os.ErrDeadlineExceeded) {
		return BackendRequest{}, ErrClientToSlow
	} else if err != nil {
		// log.Printf("error while reading handshake: %v", err)
		return BackendRequest{}, err
	}

	conn.SetDeadline(bw.IODeadline())
	packet, err := mcConn.ReadPacket()
	if errors.Is(err, os.ErrDeadlineExceeded) {
		return BackendRequest{}, ErrClientToSlow
	} else if err != nil {
		// log.Printf("error while reading second packet: %v", err)
		return BackendRequest{}, err
	}
	conn.SetDeadline(time.Time{})

	handshake, err := mc.UnmarshalServerBoundHandshake(handshakePacket)
	if err != nil {
		log.Printf("error while parsing handshake: %v", err)
	}
	reqType := mc.RequestState(handshake.NextState)
	newConnections.WithLabelValues(reqType.String()).Inc()
	if reqType == mc.UNKNOWN_STATE {
		return BackendRequest{}, ErrNotValidHandshake
	}

	serverAddr := strings.ToLower(handshake.ParseServerAddress())
	request := BackendRequest{
		Type:       reqType,
		ServerAddr: serverAddr,
		Addr:       conn.RemoteAddr(),
		Handshake:  handshake,
	}

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

func (bw *BasicWorker) ProcessRequest(req BackendRequest) ProcessAnswer {
	ch, ok := bw.serverDict[req.ServerAddr]
	if !ok {
		if req.Type == mc.STATUS {
			return ProcessAnswer{
				action:      SEND_STATUS,
				firstPacket: bw.defaultStatus,
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

// TODO:
// - figure out or this need more deadlines
func (bw *BasicWorker) ProcessAnswer(conn net.Conn, ans ProcessAnswer) {
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
		conn.SetDeadline(bw.IODeadline())
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
