package proxy

import (
	"log"
	"sync"
	"time"

	"github.com/realDragonium/Ultraviolet/config"
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
		serverWorkers: make(map[int]chan gatewayRequest),
		proxyCh:       make(chan ProxyAction),
		wg:            &sync.WaitGroup{},
	}
}

type gatewayRequest struct {
	ch chan bool
}

type Gateway struct {
	serverWorkers map[int]chan gatewayRequest
	proxyCh       chan ProxyAction
	wg            *sync.WaitGroup
}

func (gw *Gateway) Shutdown() {
	for {
		activeConns := false
		for _, ch := range gw.serverWorkers {
			answerCh := make(chan bool)
			ch <- gatewayRequest{
				ch: answerCh,
			}
			answer := <-answerCh
			if answer {
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
	for id, serverCfg := range serverCfgs {
		workerServerCfg := config.FileToWorkerConfig(serverCfg)
		privateWorker := NewPrivateWorker(id, workerServerCfg)
		gw.registerPrivateWorker(id, &privateWorker)

		workerRequestCh := make(chan McRequest)
		privateWorker.reqCh = workerRequestCh
		servers[id] = ServerWorkerData{
			connReqCh: workerRequestCh,
		}
		for _, domain := range serverCfg.Domains {
			serverDict[domain] = id
		}
		go privateWorker.Work()
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

func (gw *Gateway) registerPrivateWorker(id int, worker *PrivateWorker) {
	gatewayCh := make(chan gatewayRequest)
	gw.serverWorkers[id] = gatewayCh
	worker.gatewayCh = gatewayCh
}
