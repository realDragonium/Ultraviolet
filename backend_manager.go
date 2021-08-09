package ultraviolet

import (
	"errors"
	"reflect"

	"github.com/realDragonium/Ultraviolet/config"
)

var (
	ErrSameConfig = errors.New("old and new config are the same")
)

func NewBackendManager(manager WorkerManager, factory BackendFactoryFunc) BackendManager {
	return BackendManager{
		backends:       make(map[string]Backend),
		domains:        make(map[string]string),
		cfgs:           make(map[string]config.ServerConfig),
		workerManager:  manager,
		backendFactory: factory,
	}
}

type BackendManager struct {
	backends map[string]Backend
	domains  map[string]string
	cfgs     map[string]config.ServerConfig

	backendFactory BackendFactoryFunc
	workerManager  WorkerManager
}

func (manager *BackendManager) AddConfig(cfg config.ServerConfig) {
	if _, ok := manager.cfgs[cfg.ID()]; ok {
		return
	}
	manager.cfgs[cfg.ID()] = cfg
	for _, domain := range cfg.Domains {
		manager.domains[domain] = cfg.ID()
	}
	backend, _ := manager.backendFactory(cfg)
	manager.backends[cfg.ID()] = backend
	manager.workerManager.AddBackend(cfg.Domains, backend.ReqCh())
}

func (manager *BackendManager) RemoveConfig(cfg config.ServerConfig) {
	backend, ok := manager.backends[cfg.ID()]
	if !ok {
		return
	}

	delete(manager.cfgs, cfg.ID())
	for _, domain := range cfg.Domains {
		delete(manager.domains, domain)
	}
	manager.workerManager.RemoveBackend(cfg.Domains)

	//Call this after updating the workers, so a worker doesnt send a request to a closed backend
	backend.Close()
	delete(manager.backends, cfg.ID())
}

func (manager *BackendManager) UpdateConfig(cfg config.ServerConfig) error {
	oldCfg := manager.cfgs[cfg.ID()]
	if reflect.DeepEqual(cfg, oldCfg) {
		return ErrSameConfig
	}

	domainStatus := make(map[string]int)
	for _, domain := range cfg.Domains {
		domainStatus[domain] += 1
	}

	for _, domain := range oldCfg.Domains {
		domainStatus[domain] += 2
	}

	removedDomains := []string{}
	addedDomains := []string{}
	for key, value := range domainStatus {
		switch value {
		case 1: // new
			manager.domains[key] = cfg.ID()
			addedDomains = append(addedDomains, key)
		case 2: // old
			removedDomains = append(removedDomains, key)
			delete(manager.domains, key)
		case 3: // both have it, so keep
		}
	}
	backend := manager.backendByID(cfg.ID())
	manager.workerManager.AddBackend(addedDomains, backend.ReqCh())
	manager.workerManager.RemoveBackend(removedDomains)

	return nil
}

func (manager *BackendManager) backendByDomain(domain string) Backend {
	id := manager.domains[domain]
	return manager.backends[id]
}

func (manager *BackendManager) backendByID(id string) Backend {
	return manager.backends[id]
}

func (manager *BackendManager) LoadAllConfigs(cfgs []config.ServerConfig) {
	for _, cfg := range cfgs {
		manager.AddConfig(cfg)
	}
}

func (manager *BackendManager) CheckActiveConnections() bool {
	activeConns := false
	for _, bw := range manager.backends {
		answer := bw.HasActiveConn()
		if answer {
			activeConns = true
			break
		}
	}
	return activeConns
}

func (manager *BackendManager) KnowsDomain(domain string) bool {
	_, ok := manager.domains[domain]
	return ok
}
