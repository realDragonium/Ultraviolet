package ultraviolet

type WorkerManager interface {
	AddBackend(domains []string, ch chan<- BackendRequest)
	RemoveBackend(domains []string)
	KnowsDomain(domain string) bool
}

func NewWorkerManager() workerManager {
	return workerManager{
		domains:   make(map[string]chan<- BackendRequest),
		workerChs: []chan<- map[string]chan<- BackendRequest{},
	}
}

type workerManager struct {
	domains   map[string]chan<- BackendRequest
	workerChs []chan<- map[string]chan<- BackendRequest
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
	manager.workerChs = append(manager.workerChs, worker.UpdateCh())
	if update {
		worker.UpdateCh() <- manager.domains
	}
}

func (manager *workerManager) update() {
	for _, ch := range manager.workerChs {
		ch <- manager.domains
	}
}

func (manager *workerManager) KnowsDomain(domain string) bool {
	_, knows := manager.domains[domain]
	return knows
}
