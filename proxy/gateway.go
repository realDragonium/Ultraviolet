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

func (action ProxyAction) String() string {
	var text string
	switch action {
	case PROXY_CLOSE:
		text = "Proxy Close"
	case PROXY_OPEN:
		text = "Proxy Open"
	}
	return text
}

func NewGateway() Gateway {
	return Gateway{
		NotifyCh:       make(chan struct{}),
		ShouldNotifyCh: make(chan struct{}),

		serverWorkers: make(map[int]chan gatewayRequest),

		proxyCh: make(chan ProxyAction),
		wg:      &sync.WaitGroup{},
	}
}

type gatewayRequest struct {
	ch chan gatewayAnswer
}

type gatewayAnswer struct {
	hasOpenConnections bool
}

type Gateway struct {
	NotifyCh       chan struct{}
	ShouldNotifyCh chan struct{}

	serverWorkers map[int]chan gatewayRequest

	proxyCh chan ProxyAction
	wg      *sync.WaitGroup
}

func (gw *Gateway) Shutdown() {
	for {
		activeConns := false
		for _, ch := range gw.serverWorkers {
			answerCh := make(chan gatewayAnswer)
			ch <- gatewayRequest{
				ch: answerCh,
			}
			answer := <-answerCh
			if answer.hasOpenConnections {
				activeConns = true
			}
		}
		if !activeConns {
			return
		}
		time.Sleep(time.Minute)
	}
}

func (gw *Gateway) StartWorkers(cfg config.UltravioletConfig, serverCfgs []config.ServerConfig, reqCh chan McRequest) {
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
		gw.registerPrivateWorker(&privateWorker)
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
		go func(worker PublicWorker) {
			worker.Work()
		}(publicWorker)
	}
}

func (gw *Gateway) registerPrivateWorker(worker *PrivateWorker) {
	gatewayCh := make(chan gatewayRequest)
	gw.serverWorkers[worker.serverId] = gatewayCh
	worker.gatewayCh = gatewayCh
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
