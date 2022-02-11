package ultraviolet

import (
	"errors"
	"log"
	"net"
	"net/http"

	"github.com/prometheus/client_golang/prometheus/promhttp"
	"github.com/realDragonium/Ultraviolet/config"
)

var (
	maxConnParallelProcessing = make(chan struct{}, 100_000)
)

func NewProxy(uvReader config.UVConfigReader, l net.Listener, cfgReader config.ServerConfigReader) Proxy {
	return Proxy{
		uvReader:  uvReader,
		listener:  l,
		cfgReader: cfgReader,
	}
}

type Proxy struct {
	uvReader      config.UVConfigReader
	listener      net.Listener
	cfgReader     config.ServerConfigReader
	serverCatalog ServerCatalog
}

func (p *Proxy) Start() error {
	cfg, err := p.uvReader()
	if err != nil {
		return err
	}

	p.ReloadServerCatalog()

	if cfg.UsePrometheus {
		log.Println("Starting prometheus...")
		mux := http.NewServeMux()
		mux.Handle("/metrics", promhttp.Handler())
		promeServer := &http.Server{Addr: cfg.PrometheusBind, Handler: mux}
		go func() {
			log.Println(promeServer.ListenAndServe())
		}()
	}

	for {
		conn, err := p.listener.Accept()
		if errors.Is(err, net.ErrClosed) {
			log.Printf("net.Listener was closed, stopping with accepting calls")
			break
		} else if err != nil {
			log.Printf("got error: %v", err)
			continue
		}

		claimParallelRun()
		go func(conn net.Conn, serverCatalog ServerCatalog) {
			defer unclaimParallelRun()
			FullRun(conn, serverCatalog)
		}(conn, p.serverCatalog)
	}

	return nil
}

func claimParallelRun() {
	maxConnParallelProcessing <- struct{}{}
}

func unclaimParallelRun() {
	<-maxConnParallelProcessing
}

func (p *Proxy) ReloadServerCatalog() error {
	cfg, err := p.uvReader()
	if err != nil {
		return err
	}

	serverCatalog := NewBasicServerCatalog(cfg.DefaultStatusPk(), cfg.VerifyConnectionPk())

	newCfgs, err := p.cfgReader()
	if err != nil {
		return err
	}

	for _, newCfg := range newCfgs {
		apiCfg, err := config.ServerToAPIConfig(newCfg)
		if err != nil {
			return err
		}

		server := NewConfigServer(apiCfg)
		for _, domain := range newCfg.Domains {
			serverCatalog.ServerDict[domain] = server
		}
	}

	p.serverCatalog = &serverCatalog
	return nil
}
