package server

import (
	"bufio"
	"errors"
	"io"
	"log"
	"net"
	"time"

	"github.com/realDragonium/Ultraviolet/config"
	"github.com/realDragonium/Ultraviolet/mc"
)

const (
	maxHandshakeLength int = 264 // 264 -> 'max handshake packet length' + 1
	// packetLength:2 + packet ID: 1 + protocol version:2 + max string length:255 + port:2 + state: 1 -> 2+1+2+255+2+1 = 263
)

func ServeListener(listener net.Listener, reqCh chan net.Conn) {
	for {
		conn, err := listener.Accept()
		if err != nil {
			if errors.Is(err, net.ErrClosed) {
				log.Printf("net.Listener was closed, stopping with accepting calls")
				break
			}
			log.Println(err)
			continue
		}
		reqCh <- conn
	}
}

func StartWorkers(cfg config.UltravioletConfig, serverCfgs []config.ServerConfig) {
	reqCh := make(chan net.Conn, 1000)
	if cfg.LogOutput != nil {
		log.SetOutput(cfg.LogOutput)
	}

	ln, err := net.Listen("tcp", cfg.ListenTo)
	if err != nil {
		log.Fatalf("Can't listen: %v", err)
	}

	for i := 0; i < cfg.NumberOfListeners; i++ {
		go func(listener net.Listener, reqCh chan net.Conn) {
			ServeListener(listener, reqCh)
		}(ln, reqCh)
	}

	statusPk := cfg.DefaultStatus.Marshal()
	defaultStatus, err := statusPk.Marshal()
	if err != nil {
		log.Printf("error during marshaling default status: %v", err)
	}
	serverDict := make(map[string]chan BackendRequest)
	for id, serverCfg := range serverCfgs {
		workerServerCfg, _ := config.FileToWorkerConfig2(serverCfg)
		serverWorker := NewBasicBackendWorker(id, workerServerCfg)

		for _, domain := range serverCfg.Domains {
			serverDict[domain] = serverWorker.ReqCh
		}
		go serverWorker.Work()
	}

	worker := BasicWorker{
		ReqCh:         reqCh,
		defaultStatus: defaultStatus,
		serverDict:    serverDict,
	}

	for i := 0; i < cfg.NumberOfWorkers; i++ {
		go func(worker BasicWorker) {
			worker.Work()
		}(worker)
	}
}

func NewBasicWorker() BasicWorker {
	return BasicWorker{}
}

func NewWorker(cfg config.WorkerConfig) BasicWorker {
	dict := make(map[string]chan BackendRequest)
	statusPk := cfg.DefaultStatus.Marshal()
	defaultStatus, err := statusPk.Marshal()
	if err != nil {
		log.Printf("error during marshaling default status: %v", err)
	}
	reqCh := make(chan net.Conn)
	return BasicWorker{
		ReqCh:         reqCh,
		defaultStatus: defaultStatus,
		serverDict:    dict,
	}
}

// Need a better name for this, router doesnt fit right...
type BasicWorker struct {
	ReqCh         chan net.Conn
	defaultStatus []byte

	serverDict map[string]chan BackendRequest
}

func (r *BasicWorker) RegisterWorker(key string, worker BackendWorker) {
	r.serverDict[key] = worker.ReqCh
}

func (r *BasicWorker) Work() {
	var err error
	var conn net.Conn
	var req BackendRequest
	var ans ProcessAnswer

	for {
		conn = <-r.ReqCh
		req, err = r.ProcessConnection(conn)
		if err != nil {
			return
		}
		ans = r.ProcessRequest(req)
		log.Printf("%v - with type %v will take action: %v", conn.RemoteAddr(), req.Type, ans.action)
		r.ProcessAnswer(conn, ans)
	}
}

func (r *BasicWorker) ProcessConnection(conn net.Conn) (BackendRequest, error) {
	//  TODO: When handshake gets too long stuff goes wrong, prevent is from crashing when that happens
	b := bufio.NewReaderSize(conn, maxHandshakeLength)
	handshake, _ := mc.ReadPacket3_Handshake(b)
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
	switch ans.action {
	case PROXY:
		sConn, err := ans.serverConnFunc()
		if err != nil {
			log.Printf("Err when creating server connection: %v", err)
			conn.Close()
			return
		}
		sConn.Write(ans.Response())
		sConn.Write(ans.Response2())
		go func(client, server net.Conn, proxyCh chan ProxyAction) {
			proxyCh <- PROXY_OPEN
			ProxyConnection(client, server)
			proxyCh <- PROXY_CLOSE
		}(conn, sConn, ans.ProxyCh())
	case DISCONNECT:
		conn.Write(ans.Response())
		conn.Close()
	case SEND_STATUS:
		conn.Write(ans.Response())
		readBuf := make([]byte, 128)
		_, err := conn.Read(readBuf)
		if err != nil {
			conn.Close()
			return
		}
		if ans.Latency() != 0 {
			time.Sleep(ans.Latency())
		}
		conn.Write(readBuf[:])
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
