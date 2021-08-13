package ultraviolet

import (
	"errors"
	"fmt"
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
	"github.com/realDragonium/Ultraviolet/config"
)

var (
	upg                  tableflip.Upgrader
	registeredConfigPath string

	bWorkerManager workerManager
	backendManager BackendManager

	ReqCh chan net.Conn
)

type CheckOpenConns struct {
	Ch chan bool
}

func RunProxy(configPath string) {
	registeredConfigPath = configPath
	mainCfg, err := config.ReadUltravioletConfig(configPath)
	if err != nil {
		log.Fatalf("Error while reading main config file: %v", err)
	}

	serverCfgs, err := config.ReadServerConfigs(configPath)
	if err != nil {
		log.Fatalf("Something went wrong while reading config files: %v", err)
	}

	StartWorkers(mainCfg, serverCfgs)
	log.Println("Finished starting up proxy")

	log.Println("Now starting api endpoint")
	var promeServer *http.Server
	if mainCfg.UsePrometheus {
		log.Println("Starting prometheus...")
		mux := http.NewServeMux()
		mux.Handle("/metrics", promhttp.Handler())
		promeServer = &http.Server{Addr: mainCfg.PrometheusBind, Handler: mux}
		go func() {
			fmt.Println(promeServer.ListenAndServe())
		}()
	}

	mux := http.NewServeMux()
	mux.HandleFunc("/reload", ReloadHandler)
	apiServer := &http.Server{Addr: mainCfg.APIBind, Handler: mux}
	go func() {
		fmt.Println(apiServer.ListenAndServe())
	}()

	fmt.Println("Finished starting up")

	if runtime.GOOS == "windows" {
		select {}
	}
	if err := upg.Ready(); err != nil {
		panic(err)
	}
	<-upg.Exit()

	if mainCfg.UsePrometheus {
		promeServer.Close()
	}
	apiServer.Close()

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
	verifyError := config.VerifyConfigs(serverCfgs)
	if verifyError.HasErrors() {
		log.Fatal(verifyError.Error())
		return
	}
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

func ReloadHandler(w http.ResponseWriter, r *http.Request) {
	newCfgs, err := config.ReadServerConfigs(registeredConfigPath)
	if err != nil {
		http.Error(w, err.Error(), 500)
		return
	}
	verifyError := config.VerifyConfigs(newCfgs)
	if verifyError.HasErrors() {
		http.Error(w, verifyError.Error(), 500)
		return
	}
	backendManager.LoadAllConfigs(newCfgs)
	log.Printf("%d config files detected and loaded", len(newCfgs))
	w.WriteHeader(200)
	fmt.Fprintln(w, "success")
}
