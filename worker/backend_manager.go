package worker

import (
	"errors"
	"log"
	"reflect"

	"github.com/realDragonium/Ultraviolet/config"
)

var ErrSameConfig = errors.New("old and new config are the same")

func NewBackendManager(manager WorkerManager, factory BackendFactoryFunc, cfgReader config.ServerConfigReader) (BackendManager, error) {
	bManager := BackendManager{
		backends:       make(map[string]Backend),
		domains:        make(map[string]string),
		cfgs:           make(map[string]config.ServerConfig),
		workerManager:  manager,
		backendFactory: factory,
		configReader:   cfgReader,
	}
	err := bManager.Update()
	return bManager, err
}

type BackendManager struct {
	backends map[string]Backend
	domains  map[string]string
	cfgs     map[string]config.ServerConfig

	backendFactory BackendFactoryFunc
	workerManager  WorkerManager
	configReader   config.ServerConfigReader
}

func (manager *BackendManager) Update() error {
	newCfgs, err := manager.configReader()
	if err != nil {
		return err
	}
	for _, newCfg := range newCfgs {
		_, err := config.ServerToBackendConfig(newCfg)
		if err != nil {
			return err
		}
	}

	//From here on forward its not possible anymore to get an error...?

	if len(manager.cfgs) == 0 {
		for _, cfg := range newCfgs {
			manager.addConfig(cfg)
		}
		return nil
	}
	manager.loadAllConfigs(newCfgs)
	log.Printf("Registered %v backend(s)", len(newCfgs))
	return nil
}

// convert error should be checked before calling this method
func (manager *BackendManager) addConfig(cfg config.ServerConfig) {
	manager.cfgs[cfg.ID()] = cfg
	for _, domain := range cfg.Domains {
		manager.domains[domain] = cfg.ID()
	}
	// Error has already been changed before calling
	bwCfg, _ := config.ServerToBackendConfig(cfg)
	backend := manager.backendFactory(bwCfg)
	manager.backends[cfg.ID()] = backend
	manager.workerManager.AddBackend(cfg.Domains, backend.ReqCh())
}

func (manager *BackendManager) removeConfig(cfg config.ServerConfig) {
	server, ok := manager.backends[cfg.ID()]
	log.Println(ok)
	if !ok {
		return
	}

	delete(manager.cfgs, cfg.ID())
	for _, domain := range cfg.Domains {
		delete(manager.domains, domain)
	}
	manager.workerManager.RemoveBackend(cfg.Domains)

	//so a worker doesnt send a request to a closed backend
	server.Close()
	delete(manager.backends, cfg.ID())
}

func (manager *BackendManager) updateConfig(cfg config.ServerConfig) {
	oldCfg := manager.cfgs[cfg.ID()]
	if reflect.DeepEqual(cfg, oldCfg) {
		return
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
	b := manager.backendByID(cfg.ID())
	manager.workerManager.AddBackend(addedDomains, b.ReqCh())
	manager.workerManager.RemoveBackend(removedDomains)

	backendWorkerCfg, _ := config.ServerToBackendConfig(cfg)
	backendConfig := NewBackendConfig(backendWorkerCfg)
	b.Update(backendConfig)
}

func (manager *BackendManager) backendByID(id string) Backend {
	return manager.backends[id]
}

func (manager *BackendManager) loadAllConfigs(cfgs []config.ServerConfig) {
	newCfgs := make(map[string]config.ServerConfig)
	for _, cfg := range cfgs {
		newCfgs[cfg.ID()] = cfg
	}

	for id, oldCfg := range manager.cfgs {
		if _, ok := newCfgs[id]; !ok {
			manager.removeConfig(oldCfg)
		}
	}

	for id, newCfg := range newCfgs {
		if _, ok := manager.cfgs[id]; !ok {
			manager.addConfig(newCfg)
			continue
		}
		manager.updateConfig(newCfg)
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
