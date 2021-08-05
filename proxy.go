package ultraviolet

import (
	"errors"
	"flag"
	"log"
	"net"
	"net/http"
	"os"
	"os/signal"
	"path/filepath"
	"syscall"
	"time"

	"github.com/cloudflare/tableflip"
	"github.com/pires/go-proxyproto"
	"github.com/prometheus/client_golang/prometheus/promhttp"
	"github.com/realDragonium/Ultraviolet/config"
)

var (
	defaultCfgPath = "/etc/ultraviolet"
)

func RunProxy() {
	log.Println("Starting up Alpha-v0.10")
	var (
		cfgDir = flag.String("configs", defaultCfgPath, "`Path` to config directory")
	)
	flag.Parse()

	mainCfgPath := filepath.Join(*cfgDir, "ultraviolet.json")
	mainCfg, err := config.ReadUltravioletConfig(mainCfgPath)
	if err != nil {
		log.Fatalf("Read main config file at '%s' - error: %v", mainCfgPath, err)
	}

	serverCfgsPath := filepath.Join(*cfgDir, "config")
	serverCfgs, err := config.ReadServerConfigs(serverCfgsPath)
	if err != nil {
		log.Fatalf("Something went wrong while reading config files: %v", err)
	}

	StartWorkers(mainCfg, serverCfgs)
	log.Println("Finished starting up proxy")

	log.Println("Now starting prometheus endpoint")
	if mainCfg.UsePrometheus {
		http.Handle("/metrics", promhttp.Handler())
		log.Fatal(http.ListenAndServe(mainCfg.PrometheusBind, nil))
	} else {
		select {}
	}
}

func createListener(listenAddr string, useProxyProtocol bool, pidFile string) net.Listener {
	upg, err := tableflip.New(tableflip.Options{
		PIDFile: pidFile,
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

	ln, err := upg.Listen("tcp", listenAddr)
	if err != nil {
		log.Fatalf("Can't listen: %v", err)
	}
	if useProxyProtocol {
		proxyListener := &proxyproto.Listener{
			Listener:          ln,
			ReadHeaderTimeout: 1 * time.Second,
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
	reqCh := make(chan net.Conn, 50)
	listener := createListener(cfg.ListenTo, cfg.UseProxyProtocol, cfg.PidFile)
	for i := 0; i < cfg.NumberOfListeners; i++ {
		go func(listener net.Listener, reqCh chan net.Conn) {
			serveListener(listener, reqCh)
		}(listener, reqCh)
	}
	log.Printf("Running %v listener(s)", cfg.NumberOfListeners)

	workerCfg := config.NewWorkerConfig(cfg)
	worker := NewWorker(workerCfg, reqCh)
	for id, serverCfg := range serverCfgs {
		workerServerCfg, _ := config.FileToWorkerConfig(serverCfg)
		serverWorker := NewBackendWorker(id, workerServerCfg)
		for _, domain := range serverCfg.Domains {
			worker.RegisterBackendWorker(domain, serverWorker)
		}
		go serverWorker.Work()
	}
	log.Printf("Registered %v backend(s)", len(serverCfgs))

	for i := 0; i < cfg.NumberOfWorkers; i++ {
		go func(worker BasicWorker) {
			worker.Work()
		}(worker)
	}
	log.Printf("Running %v worker(s)", cfg.NumberOfWorkers)
}
