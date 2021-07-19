package router

import (
	"log"
	"net"

	"github.com/realDragonium/Ultraviolet/config"
	"github.com/realDragonium/Ultraviolet/mc"
	"github.com/realDragonium/Ultraviolet/server"
)

type ServerWorkerData struct {
	reqCh chan server.BackendRequest
}

type McRequest struct {
	// Type       McRequestType
	ServerAddr string
	Username   string
	Addr       net.Addr
}

type RouterRequest struct {
}

func NewBasicRouter() BasicRouter {
	return BasicRouter{}
}

type BasicRouter struct {
	reqCh         chan RouterRequest
	defaultStatus mc.Packet

	serverDict map[string]int
	servers    map[int]ServerWorkerData
}

func StartWorkers(cfg config.UltravioletConfig, serverCfgs []config.ServerConfig, reqCh chan RouterRequest) {
	if cfg.LogOutput != nil {
		log.SetOutput(cfg.LogOutput)
	}

	defaultStatus := cfg.DefaultStatus.Marshal()
	servers := make(map[int]ServerWorkerData)
	serverDict := make(map[string]int)
	for id, serverCfg := range serverCfgs {
		workerServerCfg, _ := config.FileToWorkerConfig2(serverCfg)
		serverWorker := server.NewBackendWorker(id, workerServerCfg)

		servers[id] = ServerWorkerData{
			reqCh: serverWorker.ReqCh,
		}
		for _, domain := range serverCfg.Domains {
			serverDict[domain] = id
		}
		go serverWorker.Work()
	}

	router := BasicRouter{
		reqCh:         reqCh,
		defaultStatus: defaultStatus,
		serverDict:    serverDict,
		servers:       servers,
	}

	for i := 0; i < cfg.NumberOfWorkers; i++ {
		go func(worker BasicRouter) {
			worker.Work()
		}(router)
	}
}

func (r *BasicRouter) Work() {

}
