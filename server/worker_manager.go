package server

import (
	"log"
	"net"

	"github.com/realDragonium/Ultraviolet/config"
)

type WorkerManager interface {
	AddBackend(domains []string, ch chan<- BackendRequest)
	RemoveBackend(domains []string)
	KnowsDomain(domain string) bool
	Register(worker UpdatableWorker, update bool)
}

func NewWorkerManager(cfg config.UltravioletConfig, reqCh <-chan net.Conn) WorkerManager {
	manager := workerManager{
		domains: make(map[string]chan<- BackendRequest),
		workers: []UpdatableWorker{},
	}
	workerCfg := config.NewWorkerConfig(cfg)
	for i := 0; i < cfg.NumberOfWorkers; i++ {
		worker := NewWorker(workerCfg, reqCh)
		go func(bw BasicWorker) {
			bw.Work()
		}(worker)
		manager.Register(&worker, true)
	}
	log.Printf("Running %v worker(s)", cfg.NumberOfWorkers)
	return &manager
}

type workerManager struct {
	domains map[string]chan<- BackendRequest
	workers []UpdatableWorker
}

func (manager *workerManager) AddBackend(domains []string, ch chan<- BackendRequest) {
	for _, domain := range domains {
		manager.domains[domain] = ch
	}
	manager.update()
}

func (manager *workerManager) RemoveBackend(domains []string) {
	for _, domain := range domains {
		delete(manager.domains, domain)
	}
	manager.update()
}

func (manager *workerManager) Register(worker UpdatableWorker, update bool) {
	manager.workers = append(manager.workers, worker)
	if update {
		worker.Update(manager.domains)
	}
}

func (manager *workerManager) update() {
	for _, worker := range manager.workers {
		worker.Update(manager.domains)
	}
}

func (manager *workerManager) KnowsDomain(domain string) bool {
	_, knows := manager.domains[domain]
	return knows
}
