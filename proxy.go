package ultraviolet

import (
	"errors"
	"flag"
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
	upg            tableflip.Upgrader
	defaultCfgPath = "/etc/ultraviolet"
	configPath     = defaultCfgPath

	bWorkerManager workerManager
	backendManager BackendManager

	ReqCh chan net.Conn
)

type CheckOpenConns struct {
	Ch chan bool
}

func RunProxy() {
	log.Println("Starting up Alpha-v0.11.2")
	var (
		cfgDir = flag.String("configs", defaultCfgPath, "`Path` to config directory")
	)
	flag.Parse()
	configPath = *cfgDir

	mainCfgPath := filepath.Join(*cfgDir, "ultraviolet.json")
	mainCfg, err := config.ReadUltravioletConfig(mainCfgPath)
	if err != nil {
		log.Fatalf("Read main config file at '%s' - error: %v", mainCfgPath, err)
	}

	serverCfgs, err := config.ReadServerConfigs(*cfgDir)
	if err != nil {
		log.Fatalf("Something went wrong while reading config files: %v", err)
	}

	StartWorkers(mainCfg, serverCfgs)
	log.Println("Finished starting up proxy")

	log.Println("Now starting api endpoint")
	if mainCfg.UsePrometheus {
		log.Println("while at it, also adding prometheus to it")
		http.Handle("/metrics", promhttp.Handler())
	}
	http.HandleFunc("/reload", reloadHandler)
	server := &http.Server{Addr: mainCfg.PrometheusBind, Handler: nil}
	go server.ListenAndServe()
	// go http.ListenAndServe(mainCfg.PrometheusBind, nil)

	if runtime.GOOS == "windows" {
		select {}
	}
	if err := upg.Ready(); err != nil {
		panic(err)
	}
	<-upg.Exit()

	server.Close()
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
	if runtime.GOOS == "windows" || !cfg.EnableHotSwap {
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
		proxyListener := &proxyproto.Listener{
			Listener: ln,
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

func StartWorkers(cfg config.UltravioletConfig, serverCfgs []config.ServerConfig) {
	if ReqCh == nil {
		ReqCh = make(chan net.Conn, 50)
	}

	listener := createListener(cfg)
	for i := 0; i < cfg.NumberOfListeners; i++ {
		go func(listener net.Listener, reqCh chan net.Conn) {
			serveListener(listener, reqCh)
		}(listener, ReqCh)
	}
	log.Printf("Running %v listener(s)", cfg.NumberOfListeners)

	bWorkerManager = NewWorkerManager()
	backendManager = NewBackendManager(&bWorkerManager, BackendFactory)
	backendManager.LoadAllConfigs(serverCfgs)
	log.Printf("Registered %v backend(s)", len(serverCfgs))

	workerCfg := config.NewWorkerConfig(cfg)
	for i := 0; i < cfg.NumberOfWorkers; i++ {
		worker := NewWorker(workerCfg, ReqCh)
		go func(bw BasicWorker) {
			bw.Work()
		}(worker)
		bWorkerManager.Register(&worker, true)
	}
	log.Printf("Running %v worker(s)", cfg.NumberOfWorkers)

}

func reloadHandler(w http.ResponseWriter, r *http.Request) {
	newCfgs, err := config.ReadServerConfigs(configPath)
	if err != nil {
		fmt.Fprintf(w, "failed: %v", err)
		return
	}
	backendManager.LoadAllConfigs(newCfgs)
	fmt.Fprintf(w, "config: %v", configPath)
}
