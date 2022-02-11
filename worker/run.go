package worker

import (
	"errors"
	"fmt"
	"log"
	"net"
	"net/http"
	"os"
	"os/signal"
	"path/filepath"
	"runtime"
	"syscall"
	"time"

	"github.com/cloudflare/tableflip"
	"github.com/pires/go-proxyproto"
	"github.com/prometheus/client_golang/prometheus/promhttp"
	"github.com/realDragonium/Ultraviolet/config"
)

var (
	configPath     string
	upg            *tableflip.Upgrader
	pidFileName    = "uv.pid"
	ReqCh          chan net.Conn
	backendManager BackendManager
)

func RunProxy(configPath, version string) error {
	uvReader := config.NewUVConfigFileReader(configPath)
	serverCfgReader := config.NewBackendConfigFileReader(configPath, config.VerifyConfigs)
	cfg, err := uvReader()
	if err != nil {
		return err
	}

	notUseHotSwap := !cfg.UseTableflip || runtime.GOOS == "windows" || version == "docker"
	newListener, err := createListener(cfg, notUseHotSwap)
	if err != nil {
		return err
	}
	
	proxy := NewProxy(uvReader, newListener, serverCfgReader.Read)
	err = proxy.Start()
	if err != nil {
		return err
	}

	if notUseHotSwap {
		select {}
	}

	if err := upg.Ready(); err != nil {
		panic(err)
	}
	<-upg.Exit()

	log.Println("Waiting for all connections to be closed before shutting down")
	for {
		active := backendManager.CheckActiveConnections()
		if !active {
			break
		}
		time.Sleep(time.Minute)
	}
	log.Println("All connections closed, shutting down process")
	return nil
}

func createListener(cfg config.UltravioletConfig, notUseHotSwap bool) (net.Listener, error) {
	var ln net.Listener
	var err error
	if notUseHotSwap {
		ln, err = net.Listen("tcp", cfg.ListenTo)
	} else {
		if cfg.PidFile == "" {
			cfg.PidFile = filepath.Join(configPath, pidFileName)
		}
		if _, err := os.Stat(cfg.PidFile); errors.Is(err, os.ErrNotExist) {
			pid := fmt.Sprint(os.Getpid())
			bb := []byte(pid)
			os.WriteFile(cfg.PidFile, bb, os.ModePerm)
		}
		ln, err = tableflipListener(cfg)
	}

	if err != nil {
		log.Fatalf("Can't listen: %v", err)
	}

	if cfg.AcceptProxyProtocol {
		policyFunc := func(upstream net.Addr) (proxyproto.Policy, error) {
			return proxyproto.REQUIRE, nil
		}
		proxyListener := &proxyproto.Listener{
			Listener: ln,
			Policy:   policyFunc,
		}
		return proxyListener, nil
	}
	return ln, nil
}

func tableflipListener(cfg config.UltravioletConfig) (net.Listener, error) {
	var err error
	upg, err = tableflip.New(tableflip.Options{
		PIDFile: cfg.PidFile,
	})
	if err != nil {
		log.Fatal(err)
	}
	go func() {
		sig := make(chan os.Signal, 1)
		signal.Notify(sig, syscall.SIGHUP)
		for range sig {
			err := upg.Upgrade()
			if err != nil {
				log.Println("upgrade failed:", err)
			}
		}
	}()
	return upg.Listen("tcp", cfg.ListenTo)
}

func NewProxy(uvReader config.UVConfigReader, l net.Listener, cfgReader config.ServerConfigReader) WorkerProxy {
	return WorkerProxy{
		uvReader:  uvReader,
		listener:  l,
		cfgReader: cfgReader,
	}
}

type WorkerProxy struct {
	uvReader  config.UVConfigReader
	listener  net.Listener
	cfgReader config.ServerConfigReader
}

func (p *WorkerProxy) Start() error {
	cfg, err := p.uvReader()
	if err != nil {
		return err
	}
	if ReqCh == nil {
		ReqCh = make(chan net.Conn, 50)
	}
	workerManager := NewWorkerManager(p.uvReader, ReqCh)
	workerManager.Start()
	backendManager, err = NewBackendManager(workerManager, BackendFactory, p.cfgReader)
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
	UsedAPI := NewAPI(backendManager)
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
