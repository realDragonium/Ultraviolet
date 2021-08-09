package ultraviolet

type WorkerManager interface {
	AddBackend(domains []string, ch chan<- BackendRequest)
	RemoveBackend(domains []string)
	KnowsDomain(domain string) bool
}

func NewWorkerManager() workerManager {
	return workerManager{
		domains: make(map[string]chan<- BackendRequest),
		workers: []UpdatableWorker{},
	}
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
