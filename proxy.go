package ultraviolet

import (
	"errors"
	"log"
	"net"
	"net/http"

	"github.com/prometheus/client_golang/prometheus/promhttp"
	"github.com/realDragonium/Ultraviolet/config"
	"github.com/realDragonium/Ultraviolet/server"
)

var (
	ReqCh          chan net.Conn
	BackendManager server.BackendManager
)

func NewProxy(uvReader config.UVConfigReader, l net.Listener, cfgReader config.ServerConfigReader) Proxy {
	return Proxy{
		uvReader:  uvReader,
		listener:  l,
		cfgReader: cfgReader,
	}
}

type Proxy struct {
	uvReader  config.UVConfigReader
	listener  net.Listener
	cfgReader config.ServerConfigReader
}

func (p Proxy) Start() error {
	cfg, err := p.uvReader()
	if err != nil {
		return err
	}
	if ReqCh == nil {
		ReqCh = make(chan net.Conn, 50)
	}
	workerManager := server.NewWorkerManager(p.uvReader, ReqCh)
	workerManager.Start()
	BackendManager, err = server.NewBackendManager(workerManager, server.BackendFactory, p.cfgReader)
	if err != nil {
		return err
	}

	for i := 0; i < cfg.NumberOfListeners; i++ {
		go func(listener net.Listener, reqCh chan<- net.Conn) {
			serveListener(listener, reqCh)
		}(p.listener, ReqCh)
	}
	log.Printf("Running %v listener(s)", cfg.NumberOfListeners)

	if cfg.UsePrometheus {
		log.Println("Starting prometheus...")
		mux := http.NewServeMux()
		mux.Handle("/metrics", promhttp.Handler())
		promeServer := &http.Server{Addr: cfg.PrometheusBind, Handler: mux}
		go func() {
			log.Println(promeServer.ListenAndServe())
		}()
	}

	log.Println("Now starting api endpoint")
	UsedAPI := NewAPI(BackendManager)
	go UsedAPI.Run(cfg.APIBind)
	log.Println("Finished starting up")

	return nil
}

func serveListener(listener net.Listener, reqCh chan<- net.Conn) {
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
