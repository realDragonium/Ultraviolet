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
	"github.com/realDragonium/Ultraviolet/core"
	"github.com/realDragonium/Ultraviolet/mc"
)

const (
	maxHandshakeLength int = 264 // 264 -> 'max handshake packet length' + 1
	// packetLength:2 + packet ID: 1 + protocol version:2 + max string length:255 + port:2 + state: 1 -> 2+1+2+255+2+1 = 263
)

type UpdatableWorker interface {
	Update(data core.ServerCatalog)
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
	defaultStatusPk := cfg.DefaultStatus.Marshal()
	closeAnswer := NewCloseAnswer()
	closeAnswer.ServerName = unknownServerAddr
	dict := core.NewEmptyServerCatalog(defaultStatusPk, mc.Packet{})
	return BasicWorker{
		reqCh:       reqCh,
		closeAnswer: closeAnswer,
		serverDict:  dict,
		ioTimeout:   cfg.IOTimeout,
		closeCh:     make(chan struct{}),
		updateCh:    make(chan core.ServerCatalog),
	}
}

type BasicWorker struct {
	reqCh    <-chan net.Conn
	closeCh  chan struct{}
	updateCh chan core.ServerCatalog

	closeAnswer BackendAnswer

	ioTimeout  time.Duration
	serverDict core.ServerCatalog
}

func (w *BasicWorker) IODeadline() time.Time {
	return time.Now().Add(w.ioTimeout)
}

func (w *BasicWorker) CloseCh() chan<- struct{} {
	return w.closeCh
}

func (w *BasicWorker) Update(data core.ServerCatalog) {
	w.updateCh <- data
}

func (w *BasicWorker) SetServers(servers core.ServerCatalog) {
	w.serverDict = servers
}

func (bw *BasicWorker) Work() {
	for {
		select {
		case conn := <-bw.reqCh:
			if err := bw.ProcessConnection(conn); err != nil {
				if errors.Is(err, core.ErrClientToSlow) {
					log.Printf("client %v was to slow with sending packet to us", conn.RemoteAddr())
				} else {
					log.Printf("error while trying to read: %v", err)
				}
			}
		case <-bw.closeCh:
			return
		case serverChs := <-bw.updateCh:
			bw.SetServers(serverChs)
		}
	}
}

func (bw *BasicWorker) NotSafeYet_ProcessConnection(conn net.Conn) (core.RequestData, error) {
	//  TODO: When handshake gets too long stuff goes wrong, prevent is from crashing when that happens
	b := bufio.NewReaderSize(conn, maxHandshakeLength)
	handshake, err := mc.ReadPacket3_Handshake(b)
	if err != nil {
		log.Printf("error parsing handshake from %v - error: %v", conn.RemoteAddr(), err)
	}
	t := mc.RequestState(handshake.NextState)
	if t == mc.UnknownState {
		return core.RequestData{}, core.ErrNotValidHandshake
	}
	request := core.RequestData{
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

func (bw *BasicWorker) ProcessConnection(conn net.Conn) (err error) {
	req, err := bw.ReadConnection(conn)
	if err != nil {
		return
	}

	server, err := bw.serverDict.Find(req.ServerAddr)
	if err != nil {
		return
	}
	return ultraviolet.ProcessServer(conn, server, req)
}

// TODO:
// - Adding some more error tests
func (bw *BasicWorker) ReadConnection(conn net.Conn) (reqData core.RequestData, err error) {
	mcConn := mc.NewMcConn(conn)
	conn.SetDeadline(bw.IODeadline())

	handshakePacket, err := mcConn.ReadPacket()
	if errors.Is(err, os.ErrDeadlineExceeded) {
		return reqData, core.ErrClientToSlow
	} else if err != nil {
		return
	}

	handshake, err := mc.UnmarshalServerBoundHandshake(handshakePacket)
	if err != nil {
		log.Printf("error while parsing handshake: %v", err)
	}
	reqType := mc.RequestState(handshake.NextState)
	if reqType == mc.UnknownState {
		return reqData, core.ErrNotValidHandshake
	}

	packet, err := mcConn.ReadPacket()
	if errors.Is(err, os.ErrDeadlineExceeded) {
		return reqData, core.ErrClientToSlow
	} else if err != nil {
		return
	}
	conn.SetDeadline(time.Time{})

	serverAddr := strings.ToLower(handshake.ParseServerAddress())
	reqData = core.RequestData{
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
