package proxy

import (
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

	defaultStatus := cfg.DefaultStatus.Marshal()
	workerServerCfgs := make(map[string]WorkerServerConfig)
	for _, serverCfg := range serverCfgs {
		workerServerCfg := FileToWorkerConfig(serverCfg)
		workerServerCfgs[serverCfg.MainDomain] = workerServerCfg
		for _, extraDomains := range serverCfg.ExtraDomains {
			workerServerCfgs[extraDomains] = workerServerCfg
		}
	}

	workerCfg := NewWorkerConfig(reqCh, workerServerCfgs, defaultStatus)
	workerCfg.ProxyCh = proxyCh
	RunBasicWorkers(cfg.NumberOfWorkers, workerCfg, statusCh, connCh)
	RunConnWorkers(cfg.NumberOfConnWorkers, connCh, statusCh, workerServerCfgs)
	RunStatusWorkers(cfg.NumberOfStatusWorkers, statusCh, connCh, workerServerCfgs)
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
	return WorkerServerConfig{
		ProxyTo:           cfg.ProxyTo,
		ProxyBind:         cfg.ProxyBind,
		SendProxyProtocol: cfg.SendProxyProtocol,
		OfflineStatus:     offlineStatusPk,
		DisconnectPacket:  disconPk,
		RateLimit:         cfg.RateLimit,
		RateLimitDuration: duration,
	}
}
