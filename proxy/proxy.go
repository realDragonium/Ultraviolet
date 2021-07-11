package proxy

import (
	"log"
	"sync"
	"time"

	"github.com/realDragonium/Ultraviolet/config"
	"github.com/realDragonium/Ultraviolet/mc"
)

type ProxyAction int8

const (
	PROXY_OPEN ProxyAction = iota
	PROXY_CLOSE
)

func NewProxy() Proxy {
	return Proxy{
		NotifyCh:       make(chan struct{}),
		ShouldNotifyCh: make(chan struct{}),

		ProxyCh: make(chan ProxyAction),
		wg:      &sync.WaitGroup{},
	}
}

type Proxy struct {
	NotifyCh       chan struct{}
	ShouldNotifyCh chan struct{}

	ProxyCh chan ProxyAction
	wg      *sync.WaitGroup
}

func Serve(cfg config.UltravioletConfig, serverCfgs []config.ServerConfig, reqCh chan McRequest) (chan struct{}, chan struct{}) {
	p := NewProxy()
	go p.manageConnections()
	SetupWorkers(cfg, serverCfgs, reqCh, p.ProxyCh)
	return p.ShouldNotifyCh, p.NotifyCh
}

func SetupWorkers(cfg config.UltravioletConfig, serverCfgs []config.ServerConfig, reqCh chan McRequest, proxyCh chan ProxyAction) {
	connCh := make(chan ConnRequest)
	statusCh := make(chan StatusRequest)

	if cfg.LogOutput != nil {
		log.SetOutput(cfg.LogOutput)
	}

	defaultStatus := cfg.DefaultStatus.Marshal()
	workerServerCfgs := make(map[int]WorkerServerConfig)
	serverDict := make(map[string]int)
	for i, serverCfg := range serverCfgs {
		workerServerCfg := FileToWorkerConfig(serverCfg)
		workerServerCfgs[i] = workerServerCfg
		for _, domain := range serverCfg.Domains {
			serverDict[domain] = i
		}
	}

	workerCfg := NewWorkerConfig(reqCh, serverDict, workerServerCfgs, defaultStatus)
	workerCfg.ProxyCh = proxyCh
	RunBasicWorkers(cfg.NumberOfWorkers, workerCfg, statusCh, connCh)
	RunConnWorkers(cfg.NumberOfConnWorkers, connCh, statusCh, workerServerCfgs)
	RunStatusWorkers(cfg.NumberOfStatusWorkers, statusCh, connCh, workerServerCfgs)
}

func SetupNewWorkers(cfg config.UltravioletConfig, serverCfgs []config.ServerConfig, reqCh chan McRequest) {
	if cfg.LogOutput != nil {
		log.SetOutput(cfg.LogOutput)
	}

	defaultStatus := cfg.DefaultStatus.Marshal()
	servers := make(map[int]ServerWorkerData)
	serverDict := make(map[string]int)
	for i, serverCfg := range serverCfgs {
		workerRequestCh := make(chan McRequest)
		workerServerCfg := FileToWorkerConfig(serverCfg)
		privateWorker := NewPrivateWorker(i, workerServerCfg)
		privateWorker.reqCh = workerRequestCh
		go privateWorker.Work()
		for _, domain := range serverCfg.Domains {
			serverDict[domain] = i
			servers[i] = ServerWorkerData{
				connReqCh: workerRequestCh,
			}
		}
	}

	publicWorker := PublicWorker{
		reqCh:         reqCh,
		defaultStatus: defaultStatus,
		serverDict:    serverDict,
		servers:       servers,
	}
	for i := 0; i < cfg.NumberOfWorkers; i++ {
		go func(worker PublicWorker){
			worker.Work()
		}(publicWorker)
	}

}

func (p *Proxy) manageConnections() {
	go func() {
		<-p.ShouldNotifyCh
		p.wg.Wait()
		p.NotifyCh <- struct{}{}
	}()

	for {
		action := <-p.ProxyCh
		switch action {
		case PROXY_OPEN:
			p.wg.Add(1)
		case PROXY_CLOSE:
			p.wg.Done()
		}
	}
}

func FileToWorkerConfig(cfg config.ServerConfig) WorkerServerConfig {
	disconPk := mc.ClientBoundDisconnect{
		Reason: mc.Chat(cfg.DisconnectMessage),
	}.Marshal()
	offlineStatusPk := cfg.OfflineStatus.Marshal()
	duration, _ := time.ParseDuration(cfg.RateDuration)
	cooldown, _ := time.ParseDuration(cfg.UpdateCooldown)
	dialTimeout, _ := time.ParseDuration(cfg.DialTimeout)
	return WorkerServerConfig{
		ProxyTo:             cfg.ProxyTo,
		ProxyBind:           cfg.ProxyBind,
		DialTimeout:         dialTimeout,
		SendProxyProtocol:   cfg.SendProxyProtocol,
		OfflineStatus:       offlineStatusPk,
		DisconnectPacket:    disconPk,
		RateLimit:           cfg.RateLimit,
		RateLimitDuration:   duration,
		StateUpdateCooldown: cooldown,
	}
}
