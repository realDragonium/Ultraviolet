package worker

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
	ultraviolet "github.com/realDragonium/Ultraviolet"
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
	unknownServerAddr = "unknown"

	requestBuckets  = []float64{.0001, .0005, .001, .005, .01, .05, .1, .5, 1, 5}
	processRequests = promauto.NewHistogramVec(prometheus.HistogramOpts{
		Namespace: "ultraviolet",
		Name:      "request_duration_seconds",
		Help:      "Histogram request processing durations.",
		Buckets:   requestBuckets,
	}, []string{"action", "server", "type"})
)

func NewWorker(cfg config.WorkerConfig, reqCh <-chan net.Conn) BasicWorker {
	dict := make(map[string]chan<- BackendRequest)
	defaultStatusPk := cfg.DefaultStatus.Marshal()
	statusAnswer := NewStatusAnswer(defaultStatusPk)
	statusAnswer.ServerName = unknownServerAddr
	closeAnswer := NewCloseAnswer()
	closeAnswer.ServerName = unknownServerAddr
	return BasicWorker{
		reqCh:               reqCh,
		defaultStatusAnswer: statusAnswer,
		closeAnswer:         closeAnswer,
		serverDict:          dict,
		ioTimeout:           cfg.IOTimeout,
		closeCh:             make(chan struct{}),
		updateCh:            make(chan map[string]chan<- BackendRequest),
	}
}

type BasicWorker struct {
	reqCh    <-chan net.Conn
	closeCh  chan struct{}
	updateCh chan map[string]chan<- BackendRequest

	defaultStatusAnswer BackendAnswer
	closeAnswer         BackendAnswer

	ioTimeout  time.Duration
	serverDict map[string]chan<- BackendRequest
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
	for {
		select {
		case conn := <-bw.reqCh:
			start := time.Now()
			req, ans, err := bw.ProcessConnection(conn)
			if err != nil {
				conn.Close()
				if errors.Is(err, ultraviolet.ErrClientToSlow) {
					log.Printf("client %v was to slow with sending packet to us", conn.RemoteAddr())
				} else {
					log.Printf("error while trying to read: %v", err)
				}
			}
			dur := time.Since(start).Seconds()
			labels := prometheus.Labels{"server": ans.ServerName, "type": req.Type.String(), "action": ans.action.String()}
			processRequests.With(labels).Observe(dur)
		case <-bw.closeCh:
			return
		case serverChs := <-bw.updateCh:
			bw.SetServers(serverChs)
		}
	}
}

func (bw *BasicWorker) NotSafeYet_ProcessConnection(conn net.Conn) (ultraviolet.RequestData, error) {
	//  TODO: When handshake gets too long stuff goes wrong, prevent is from crashing when that happens
	b := bufio.NewReaderSize(conn, maxHandshakeLength)
	handshake, err := mc.ReadPacket3_Handshake(b)
	if err != nil {
		log.Printf("error parsing handshake from %v - error: %v", conn.RemoteAddr(), err)
	}
	t := mc.RequestState(handshake.NextState)
	if t == mc.UnknownState {
		return ultraviolet.RequestData{}, ultraviolet.ErrNotValidHandshake
	}
	request := ultraviolet.RequestData{
		Type:       t,
		ServerAddr: handshake.ParseServerAddress(),
		Addr:       conn.RemoteAddr(),
		Handshake:  handshake,
	}

	packet, _ := mc.ReadPacket3(b)
	if t == mc.Login {
		loginStart, _ := mc.UnmarshalServerBoundLoginStart(packet)
		request.Username = string(loginStart.Name)
	}
	return request, nil
}

func (bw *BasicWorker) ProcessConnection(conn net.Conn) (ultraviolet.RequestData, BackendAnswer, error) {
	req, err := bw.ReadConnection(conn)
	if err != nil {
		return req, bw.closeAnswer, err
	}
	// log.Printf("received connection from %v with addr: %s", conn.RemoteAddr(), req.ServerAddr)
	ans := bw.ProcessRequest(req)
	// log.Printf("%v request from %v - %v will take action: %v", req.Type, conn.RemoteAddr(), req.Username, ans.Action())
	bw.ProcessAnswer(conn, ans)
	return req, ans, nil
}

// TODO:
// - Adding some more error tests
func (bw *BasicWorker) ReadConnection(conn net.Conn) (reqData ultraviolet.RequestData, err error) {
	mcConn := mc.NewMcConn(conn)
	conn.SetDeadline(bw.IODeadline())

	handshakePacket, err := mcConn.ReadPacket()
	if errors.Is(err, os.ErrDeadlineExceeded) {
		return reqData, ultraviolet.ErrClientToSlow
	} else if err != nil {
		return
	}

	handshake, err := mc.UnmarshalServerBoundHandshake(handshakePacket)
	if err != nil {
		log.Printf("error while parsing handshake: %v", err)
	}
	reqType := mc.RequestState(handshake.NextState)
	if reqType == mc.UnknownState {
		return reqData, ultraviolet.ErrNotValidHandshake
	}

	packet, err := mcConn.ReadPacket()
	if errors.Is(err, os.ErrDeadlineExceeded) {
		return reqData, ultraviolet.ErrClientToSlow
	} else if err != nil {
		return
	}
	conn.SetDeadline(time.Time{})

	serverAddr := strings.ToLower(handshake.ParseServerAddress())
	reqData = ultraviolet.RequestData{
		Type:       reqType,
		ServerAddr: serverAddr,
		Addr:       conn.RemoteAddr(),
		Handshake:  handshake,
	}

	if reqType == mc.Login {
		loginStart, err := mc.UnmarshalServerBoundLoginStart(packet)
		if err != nil {
			log.Printf("error while parsing login packet: %v", err)
			return reqData, err
		}
		reqData.Username = string(loginStart.Name)
	}

	return reqData, nil
}

func (bw *BasicWorker) ProcessRequest(reqData ultraviolet.RequestData) BackendAnswer {
	ch, ok := bw.serverDict[reqData.ServerAddr]
	if !ok {
		if reqData.Type == mc.Status {
			return bw.defaultStatusAnswer
		}
		return bw.closeAnswer
	}

	rCh := make(chan BackendAnswer)
	backendReq := BackendRequest{
		ReqData: reqData,
		Ch:      rCh,
	}

	ch <- backendReq
	return <-rCh
}

// TODO:
// - figure out or this need more deadlines
func (bw *BasicWorker) ProcessAnswer(conn net.Conn, ans BackendAnswer) {
	clientMcConn := mc.NewMcConn(conn)
	switch ans.Action() {
	case Proxy:
		sConn, err := ans.ServerConn()
		if err != nil {
			log.Printf("Err when creating server connection: %v", err)
			conn.Close()
			return
		}
		mcServerConn := mc.NewMcConn(sConn)
		mcServerConn.WritePacket(ans.Response())
		mcServerConn.WritePacket(ans.Response2())
		go func(client, serverConn net.Conn, proxyCh chan ProxyAction) {
			proxyCh <- ProxyOpen
			ProxyConnection(client, serverConn)
			proxyCh <- ProxyClose
		}(conn, sConn, ans.ProxyCh())
	case Disconnect:
		clientMcConn.WritePacket(ans.Response())
		conn.Close()
	case SendStatus:
		clientMcConn.WritePacket(ans.Response())
		conn.SetDeadline(bw.IODeadline())
		pingPacket, err := clientMcConn.ReadPacket()
		if err != nil {
			conn.Close()
			return
		}
		clientMcConn.WritePacket(pingPacket)
		conn.Close()
	case Close:
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
