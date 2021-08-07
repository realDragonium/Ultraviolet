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
	"reflect"
	"runtime"
	"syscall"
	"time"

	"github.com/cloudflare/tableflip"
	"github.com/pires/go-proxyproto"
	"github.com/prometheus/client_golang/prometheus"
	"github.com/prometheus/client_golang/prometheus/promauto"
	"github.com/prometheus/client_golang/prometheus/promhttp"
	"github.com/realDragonium/Ultraviolet/config"
)

var (
	upg            tableflip.Upgrader
	proxiesCounter = promauto.NewGauge(prometheus.GaugeOpts{
		Name: "ultraviolet_proxy_total",
		Help: "The total number of registered proxies",
	})
	defaultCfgPath = "/etc/ultraviolet"
	configPath     = defaultCfgPath

	backendWorkers map[string]BackendWorker         = make(map[string]BackendWorker)
	serverChs      map[string]chan<- BackendRequest = make(map[string]chan<- BackendRequest)
	serverCfgs     map[string]config.ServerConfig   = make(map[string]config.ServerConfig)
	basicWorkers   []BasicWorker                    = []BasicWorker{}
	reqCh          chan net.Conn
)

func RegisterBackendWorker(id string, bw BackendWorker) {
	backendWorkers[id] = bw
}

func DeregisterBackendWorker(id string, bw BackendWorker) {
	delete(backendWorkers, id)
}

func RegisterBasicWorker(w BasicWorker) {
	basicWorkers = append(basicWorkers, w)
}

func RegisterServerconfig(cfg config.ServerConfig) {
	serverCfgs[cfg.FilePath] = cfg
}

func DeregisterServerconfig(cfg config.ServerConfig) {
	delete(serverCfgs, cfg.FilePath)
}

func RegisterServerCh(domains []string, ch chan<- BackendRequest) {
	for _, domain := range domains {
		serverChs[domain] = ch
	}
}

func DeregisterServerCh(domains []string) {
	for _, domain := range domains {
		delete(serverChs, domain)
	}
}

type checkOpenConns struct {
	Ch chan bool
}

func RunProxy() {
	log.Println("Starting up Alpha-v0.11.1")
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

	log.Println("Now starting prometheus endpoint")
	if mainCfg.UsePrometheus {
		http.Handle("/metrics", promhttp.Handler())
	}
	http.HandleFunc("/reload", reloadHandler)
	go http.ListenAndServe(mainCfg.PrometheusBind, nil)
	
	if runtime.GOOS == "windows" {
		select {}
	}
	if err := upg.Ready(); err != nil {
		panic(err)
	}
	<-upg.Exit()

	log.Println("Waiting for all connections to be closed before shutting down")
	for {
		active := CheckActiveConnections()
		if !active {
			break
		}
		time.Sleep(time.Minute)
	}
	log.Println("All connections closed, shutting down process")
}

func CheckActiveConnections() bool {
	activeConns := false
	for _, bw := range backendWorkers {
		answerCh := make(chan bool)
		bw.ActiveConnCh() <- checkOpenConns{
			Ch: answerCh,
		}
		answer := <-answerCh
		if answer {
			activeConns = true
			break
		}
	}
	return activeConns
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
	reqCh = make(chan net.Conn, 50)

	listener := createListener(cfg)
	for i := 0; i < cfg.NumberOfListeners; i++ {
		go func(listener net.Listener, reqCh chan net.Conn) {
			serveListener(listener, reqCh)
		}(listener, reqCh)
	}
	log.Printf("Running %v listener(s)", cfg.NumberOfListeners)

	startBackendWorkers(serverCfgs)
	workerCfg := config.NewWorkerConfig(cfg)
	startBasicWorkers(cfg.NumberOfWorkers, workerCfg)

}

func startBackendWorkers(serverCfgs []config.ServerConfig) {
	for _, serverCfg := range serverCfgs {
		startNewBackendWorker(serverCfg)
	}
	log.Printf("Registered %v backend(s)", len(serverCfgs))
}

func startNewBackendWorker(cfg config.ServerConfig) {
	RegisterServerconfig(cfg)
	workerServerCfg, _ := config.FileToWorkerConfig(cfg)
	backendWorker := NewBackendWorker(workerServerCfg)
	RegisterBackendWorker(cfg.FilePath, backendWorker)
	RegisterServerCh(cfg.Domains, backendWorker.ReqCh())
	go backendWorker.Work()
	proxiesCounter.Inc()
}

func startBasicWorkers(num int, cfg config.WorkerConfig) {
	for i := 0; i < num; i++ {
		worker := NewWorker(cfg, reqCh)
		RegisterBasicWorker(worker)
		worker.SetServers(serverChs)
		go worker.Work()
	}
	log.Printf("Running %v worker(s)", num)
}

func reloadHandler(w http.ResponseWriter, r *http.Request) {
	newCfgs, err := config.ReadServerConfigs(configPath)
	if err != nil {
		fmt.Fprintf(w, "failed: %v", err)
		return
	}
	ReloadBackendWorkers(newCfgs)
	fmt.Fprintf(w, "config: %v", configPath)
}

func ReloadBackendWorkers(currentServerCfgs []config.ServerConfig) {
	currentCfgs := make(map[string]config.ServerConfig)
	configStatus := make(map[string]int)
	for _, cfg := range serverCfgs {
		key := cfg.FilePath
		configStatus[key] += 1
	}
	for _, cfg := range currentServerCfgs {
		key := cfg.FilePath
		currentCfgs[key] = cfg
		configStatus[key] += 2
	}

	deteleCount := 0
	keepCount := 0
	newCount := 0
	updateCount := 0
	deleteBackends := []chan<- struct{}{}
	// keepCfgs := []config.ServerConfig{}
	for key, value := range configStatus {
		switch value {
		case 1: // delete
			deteleCount++
			bw := backendWorkers[key]
			closeCh := bw.CloseCh()
			deleteBackends = append(deleteBackends, closeCh)
		case 2: // new
			newCount++
			startNewBackendWorker(currentCfgs[key])
		case 3: // keep
			if reflect.DeepEqual(serverCfgs[key], currentCfgs[key]) {
				keepCount++
				continue
			}
			// updateCount++
			// newCfg := currentCfgs[key]
			// oldCfg := serverCfgs[key]

			// if newCfg.Domains != oldCfg.Domains {

			// }
		}
	}

	for _, worker := range basicWorkers {
		ch := worker.UpdateCh()
		ch <- serverChs
	}

	for _, ch := range deleteBackends {
		ch <- struct{}{}
	}

	log.Printf("%v backend(s) registered", newCount)
	log.Printf("%v backend(s) removed", deteleCount)
	log.Printf("%v backend(s) kept", keepCount)
	log.Printf("%v backend(s) updated", updateCount)
}
