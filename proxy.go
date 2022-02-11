package ultraviolet

import (
	"errors"
	"log"
	"net"

	"github.com/realDragonium/Ultraviolet/config"
	"github.com/realDragonium/Ultraviolet/core"
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
	serverCatalog core.ServerCatalog
}

func (p *Proxy) Start() error {
	p.ReloadServerCatalog()

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
		go func(conn net.Conn, serverCatalog core.ServerCatalog) {
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

	servers := make(map[string]core.Server)

	newCfgs, err := p.cfgReader()
	if err != nil {
		return err
	}

	for _, newCfg := range newCfgs {
		apiCfg, err := config.ServerToAPIConfig(newCfg)
		if err != nil {
			return err
		}

		server := NewAPIServer(apiCfg)
		for _, domain := range newCfg.Domains {
			servers[domain] = server
		}
	}

	p.serverCatalog = core.NewServerCatalog(servers, cfg.DefaultStatusPk(), cfg.VerifyConnectionPk())
	return nil
}
