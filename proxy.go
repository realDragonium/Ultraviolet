package ultraviolet

import (
	"errors"
	"log"
	"net"
	"net/http"
	"os"
	"os/signal"
	"runtime"
	"syscall"
	"time"

	"github.com/cloudflare/tableflip"
	"github.com/pires/go-proxyproto"
	"github.com/prometheus/client_golang/prometheus/promhttp"
	"github.com/realDragonium/Ultraviolet/api"
	"github.com/realDragonium/Ultraviolet/config"
	"github.com/realDragonium/Ultraviolet/server"
)

var (
	upg       tableflip.Upgrader
	ReqCh     chan net.Conn
	uvVersion = "(unknown version)"

	bWorkerManager server.WorkerManager
	backendManager server.BackendManager

	promeServer *http.Server
	API         api.API

	notUseHotSwap = false
)

func RunProxy(configPath string) {
	log.Printf("Starting up Ultraviolet %s", uvVersion)
	uvReader := config.NewUVConfigFileReader(configPath)
	serverCfgReader := config.NewBackendConfigFileReader(configPath, config.VerifyConfigs)
	StartProxy(uvReader, serverCfgReader)

	if notUseHotSwap {
		select {}
	}
	if err := upg.Ready(); err != nil {
		panic(err)
	}
	<-upg.Exit()

	promeServer.Close()
	API.Close()

	log.Println("Waiting for all connections to be closed before shutting down")
	for {
		active := backendManager.CheckActiveConnections()
		if !active {
			break
		}
		time.Sleep(time.Minute)
	}
	log.Println("All connections closed, shutting down process")
}

func createListener(cfg config.UltravioletConfig) net.Listener {
	var ln net.Listener
	var err error
	if notUseHotSwap {
		ln, err = net.Listen("tcp", cfg.ListenTo)
		if err != nil {
			log.Fatalf("Can't listen: %v", err)
		}
	} else {
		upg, err := tableflip.New(tableflip.Options{
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
		ln, err = upg.Listen("tcp", cfg.ListenTo)
		if err != nil {
			log.Fatalf("Can't listen: %v", err)
		}
	}

	if cfg.AcceptProxyProtocol {
		policyFunc := func(upstream net.Addr) (proxyproto.Policy, error) {
			return proxyproto.REQUIRE, nil
		}
		proxyListener := &proxyproto.Listener{
			Listener:          ln,
			ReadHeaderTimeout: cfg.IODeadline,
			Policy:            policyFunc,
		}
		return proxyListener
	}
	return ln
}

func serveListener(listener net.Listener, reqCh chan net.Conn) {
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

func StartProxy(uvReader config.UVConfigReader, serverCfgsReader config.ServerConfigReader) error {
	mainCfg, err := uvReader.Read()
	if err != nil {
		log.Fatalf("Error while reading main config file: %v", err)
	}
	notUseHotSwap = !mainCfg.EnableHotSwap || runtime.GOOS == "windows" || uvVersion == "docker"
	log.SetOutput(mainCfg.LogOutput)

	if ReqCh == nil {
		ReqCh = make(chan net.Conn, 50)
	}

	listener := createListener(mainCfg)
	for i := 0; i < mainCfg.NumberOfListeners; i++ {
		go func(listener net.Listener, reqCh chan net.Conn) {
			serveListener(listener, reqCh)
		}(listener, ReqCh)
	}
	log.Printf("Running %v listener(s)", mainCfg.NumberOfListeners)

	bWorkerManager = server.NewWorkerManager(mainCfg, ReqCh)

	backendManager = server.NewBackendManager(bWorkerManager, server.BackendFactory, serverCfgsReader)
	backendManager.Update()

	if mainCfg.UsePrometheus {
		log.Println("Starting prometheus...")
		mux := http.NewServeMux()
		mux.Handle("/metrics", promhttp.Handler())
		promeServer = &http.Server{Addr: mainCfg.PrometheusBind, Handler: mux}
		go func() {
			log.Println(promeServer.ListenAndServe())
		}()
	}

	log.Println("Now starting api endpoint")
	API = api.NewAPI(backendManager)
	go API.Run(mainCfg.APIBind)
	log.Println("Finished starting up")
	return nil
}
