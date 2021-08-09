package ultraviolet_test

import (
	"errors"
	"math/rand"
	"testing"
	"time"

	ultraviolet "github.com/realDragonium/Ultraviolet"
	"github.com/realDragonium/Ultraviolet/config"
	"github.com/realDragonium/Ultraviolet/mc"
)

func newTestWorkerManager() testWorkerManager {
	return testWorkerManager{
		something: make(map[string]chan<- ultraviolet.BackendRequest),
	}
}

type testWorkerManager struct {
	something map[string]chan<- ultraviolet.BackendRequest
}

func (man *testWorkerManager) AddBackend(domains []string, ch chan<- ultraviolet.BackendRequest) {
	for _, domain := range domains {
		man.something[domain] = ch
	}
}
func (man *testWorkerManager) RemoveBackend(domains []string) {
	for _, domain := range domains {
		delete(man.something, domain)
	}
}
func (man *testWorkerManager) KnowsDomain(domain string) bool {
	_, knows := man.something[domain]
	return knows
}

func newTestBackend() testBackend {
	return testBackend{
		reqCh: make(chan ultraviolet.BackendRequest),
	}
}

type testBackend struct {
	reqCh        chan ultraviolet.BackendRequest
	calledClose  bool
	calledUpdate bool
	timesClosed  int
	timesUpdated int
}

func (backend *testBackend) ReqCh() chan<- ultraviolet.BackendRequest {
	return backend.reqCh
}
func (backend *testBackend) HasActiveConn() bool {
	return false
}
func (backend *testBackend) Update(cfg ultraviolet.BackendConfig) error {
	backend.calledUpdate = true
	backend.timesUpdated++
	return nil
}
func (backend *testBackend) Close() {
	backend.calledClose = true
	backend.timesClosed++
}

var testBackendFactory ultraviolet.BackendFactoryFunc = func(cfg config.BackendWorkerConfig) (ultraviolet.Backend, error) {
	backend := newTestBackend()
	return &backend, nil
}

func newBackendManager(factory ultraviolet.BackendFactoryFunc) (ultraviolet.BackendManager, testWorkerManager) {
	workerManager := newTestWorkerManager()
	return ultraviolet.NewBackendManager(&workerManager, factory), workerManager
}

func newSimpleBackendManager() ultraviolet.BackendManager {
	workerManager := newTestWorkerManager()
	factory := testBackendFactory
	return ultraviolet.NewBackendManager(&workerManager, factory)
}

func TestBackendManager_Add(t *testing.T) {
	t.Run("adding config", func(t *testing.T) {
		manager := newSimpleBackendManager()
		domain := "uv"
		cfg := config.ServerConfig{
			Domains: []string{domain},
			ProxyTo: "1",
		}
		manager.AddConfig(cfg)
		if !manager.KnowsDomain(domain) {
			t.Error("manager should have known this domain")
		}
	})

	t.Run("adding config gives changes to worker", func(t *testing.T) {
		manager, workerManager := newBackendManager(testBackendFactory)
		domain := "uv"
		cfg := config.ServerConfig{
			Domains: []string{domain},
		}
		manager.AddConfig(cfg)
		if !workerManager.KnowsDomain(domain) {
			t.Error("manager should have known this domain")
		}
	})

	t.Run("adding config gives changes to worker", func(t *testing.T) {
		manager, workerManager := newBackendManager(testBackendFactory)
		domain := "uv"
		cfg := config.ServerConfig{
			Domains: []string{domain},
		}
		manager.AddConfig(cfg)
		if !workerManager.KnowsDomain(domain) {
			t.Error("manager should have known this domain")
		}
	})

	t.Run("adding config creates new backend", func(t *testing.T) {
		ch := make(chan struct{})
		factory := func(cfg config.BackendWorkerConfig) (ultraviolet.Backend, error) {
			ch <- struct{}{}
			backend := newTestBackend()
			return &backend, nil
		}
		manager, _ := newBackendManager(factory)
		cfg := config.ServerConfig{
			Domains: []string{"uv"},
			ProxyTo: "1",
		}
		go manager.AddConfig(cfg)

		select {
		case <-ch:
			t.Log("factory has been called")
		case <-time.After(defaultChTimeout):
			t.Error("expected factory to be called but wasnt")
		}
	})

	t.Run("adding already registered configs doesnt register them again", func(t *testing.T) {
		manager, workerManager := newBackendManager(testBackendFactory)
		domain := "uv"
		cfg := config.ServerConfig{
			Domains: []string{domain},
			ProxyTo: "1",
		}
		manager.AddConfig(cfg)
		ch := workerManager.something[domain]
		manager.AddConfig(cfg)
		if workerManager.something[domain] != ch {
			t.Error("channels should have been the same")
			t.Logf("expected:%v", ch)
			t.Logf("got:%v", workerManager.something[domain])
		}
		if !workerManager.KnowsDomain(domain) {
			t.Error("manager should have known this domain")
		}
	})

}

func TestBackendManager_Remove(t *testing.T) {
	t.Run("remove known config", func(t *testing.T) {
		manager := newSimpleBackendManager()
		domain := "uv"
		cfg := config.ServerConfig{
			Domains: []string{domain},
			ProxyTo: "1",
		}
		manager.AddConfig(cfg)
		manager.RemoveConfig(cfg)
		if manager.KnowsDomain(domain) {
			t.Error("manager should NOT have known this domain")
		}
	})

	t.Run("remove unknown config", func(t *testing.T) {
		manager := newSimpleBackendManager()
		domain := "uv"
		cfg := config.ServerConfig{
			Domains: []string{domain},
			ProxyTo: "1",
		}
		manager.RemoveConfig(cfg)
		if manager.KnowsDomain(domain) {
			t.Error("manager should NOT have known this domain")
		}
	})

	t.Run("removing config gives changes to worker", func(t *testing.T) {
		manager, workerManager := newBackendManager(testBackendFactory)
		domain := "uv"
		cfg := config.ServerConfig{
			Domains: []string{domain},
			ProxyTo: "1",
		}
		manager.AddConfig(cfg)
		manager.RemoveConfig(cfg)
		if workerManager.KnowsDomain(domain) {
			t.Error("manager should NOT have known this domain")
		}
	})

	t.Run("removing backend config closese backend", func(t *testing.T) {
		backend := newTestBackend()
		factory := func(cfg config.BackendWorkerConfig) (ultraviolet.Backend, error) {
			return &backend, nil
		}
		manager, _ := newBackendManager(factory)
		cfg := config.ServerConfig{
			Domains: []string{"uv"},
			ProxyTo: "1",
		}
		manager.AddConfig(cfg)
		manager.RemoveConfig(cfg)
		if !backend.calledClose {
			t.Error("expected close to be called")
		}
	})
}

func TestBackendManager_Update(t *testing.T) {
	t.Run("update config registers new domains and remove not used ones", func(t *testing.T) {
		manager := newSimpleBackendManager()
		filePath := "123"
		domain := "uv"
		newDomain := "uv1"
		cfg := config.ServerConfig{
			FilePath: filePath,
			Domains:  []string{domain},
			ProxyTo:  "1",
		}
		manager.AddConfig(cfg)
		newCfg := config.ServerConfig{
			FilePath: filePath,
			Domains:  []string{newDomain},
			ProxyTo:  "1",
		}
		err := manager.UpdateConfig(newCfg)
		if err != nil {
			t.Fatalf("failed to update config: %v", err)
		}
		if manager.KnowsDomain(domain) {
			t.Error("manager should NOT have known this domain")
		}
		if !manager.KnowsDomain(newDomain) {
			t.Error("manager should have known this domain")
		}
	})

	t.Run("doesnt update config if config the same", func(t *testing.T) {
		manager := newSimpleBackendManager()
		cfg := config.ServerConfig{
			FilePath: "123",
			Domains:  []string{"uv"},
			ProxyTo:  "1",
		}
		manager.AddConfig(cfg)
		err := manager.UpdateConfig(cfg)
		if !errors.Is(err, ultraviolet.ErrSameConfig) {
			t.Fatalf("didnt receive correct error: %v", err)
		}
	})

	t.Run("update config which removes domain should also affects worker ", func(t *testing.T) {
		manager, workerManager := newBackendManager(testBackendFactory)
		domain := "uv"
		domain2 := "uv3"
		cfg := config.ServerConfig{
			FilePath: "123",
			Domains:  []string{domain, domain2},
			ProxyTo:  "1",
		}
		newCfg := config.ServerConfig{
			FilePath: "123",
			Domains:  []string{domain2},
			ProxyTo:  "1",
		}
		manager.AddConfig(cfg)
		manager.UpdateConfig(newCfg)

		if workerManager.KnowsDomain(domain) {
			t.Error("manager should NOT have known this domain")
		}
	})

	t.Run("update config which adds domain should also affects worker ", func(t *testing.T) {
		manager, workerManager := newBackendManager(testBackendFactory)
		domain := "uv"
		domain2 := "uv3"
		cfg := config.ServerConfig{
			FilePath: "123",
			Domains:  []string{domain},
			ProxyTo:  "1",
		}
		newCfg := config.ServerConfig{
			FilePath: "123",
			Domains:  []string{domain, domain2},
			ProxyTo:  "1",
		}
		manager.AddConfig(cfg)
		manager.UpdateConfig(newCfg)

		if !workerManager.KnowsDomain(domain2) {
			t.Error("manager should have known this domain")
		}
		if workerManager.something[domain] == nil {
			t.Error("Channel is nil")
		}

		if workerManager.something[domain] != workerManager.something[domain2] {
			t.Error("both domains should have the same channel")
			t.Logf("domain1: %v ", workerManager.something[domain])
			t.Logf("domain2: %v", workerManager.something[domain2])
		}
	})

	t.Run("updating config also changes backend", func(t *testing.T) {
		backend := newTestBackend()
		factory := func(cfg config.BackendWorkerConfig) (ultraviolet.Backend, error) {
			return &backend, nil
		}
		manager, _ := newBackendManager(factory)
		cfg := config.ServerConfig{
			FilePath: "123",
			Domains:  []string{"uv"},
			ProxyTo:  "1",
		}
		manager.AddConfig(cfg)
		newCfg := cfg
		newCfg.ProxyTo = "2"
		err := manager.UpdateConfig(newCfg)
		if err != nil {
			t.Fatalf("got error: %v", err)
		}
		if !backend.calledUpdate {
			t.Error("expected update to be called")
		}
	})
}

func TestBackendManager_LoadAllConfigs(t *testing.T) {
	t.Run("load all configs without any loaded before", func(t *testing.T) {
		manager := newSimpleBackendManager()
		domains := []string{"uv1", "uv2", "uv3", "uv4"}
		cfgs := []config.ServerConfig{
			{
				FilePath: "uv1",
				Domains:  []string{domains[0], domains[1]},
				ProxyTo:  "1",
			},
			{
				FilePath: "uv2",
				Domains:  []string{domains[2]},
				ProxyTo:  "1",
			},
			{
				FilePath: "uv3",
				Domains:  []string{domains[3]},
				ProxyTo:  "1",
			},
		}
		manager.LoadAllConfigs(cfgs)
		for _, domain := range domains {
			if !manager.KnowsDomain(domain) {
				t.Error("manager should have known this domain")
			}
		}
	})

	t.Run("loading new configs will update if config file has changed", func(t *testing.T) {
		backend := newTestBackend()
		factory := func(cfg config.BackendWorkerConfig) (ultraviolet.Backend, error) {
			return &backend, nil
		}
		manager, _ := newBackendManager(factory)
		cfgs := []config.ServerConfig{
			{
				FilePath: "uv1",
				Domains:  []string{"uv", "uv1"},
				ProxyTo:  "1",
			},
		}
		manager.LoadAllConfigs(cfgs)
		cfgs[0].ProxyTo = "2"
		manager.LoadAllConfigs(cfgs)
		if !backend.calledUpdate {
			t.Error("expected update to be called")
		}
	})

	t.Run("remove domains of config files which werent included", func(t *testing.T) {
		manager, _ := newBackendManager(testBackendFactory)
		domain := "uv"
		cfgs := []config.ServerConfig{
			{
				FilePath: "uv2",
				Domains:  []string{domain},
				ProxyTo:  "1",
			},
		}
		manager.LoadAllConfigs(cfgs)
		manager.LoadAllConfigs([]config.ServerConfig{})
		if manager.KnowsDomain(domain) {
			t.Error("manager should NOT have known this domain")
		}
	})

	t.Run("make sure it doesnt remove a domain which also has been added by an other config", func(t *testing.T) {
		manager, _ := newBackendManager(testBackendFactory)
		domain := "uv"
		cfgs := []config.ServerConfig{
			{
				FilePath: "uv2",
				Domains:  []string{domain},
				ProxyTo:  "1",
			},
		}
		manager.LoadAllConfigs(cfgs)
		newCfgs := []config.ServerConfig{
			{
				FilePath: "uv3",
				Domains:  []string{domain},
				ProxyTo:  "1",
			},
		}
		manager.LoadAllConfigs(newCfgs)
		if !manager.KnowsDomain(domain) {
			t.Error("manager should have known this domain")
		}
	})
}

func TestCheckActiveConnections(t *testing.T) {
	t.Run("empty map", func(t *testing.T) {
		manager := newSimpleBackendManager()
		active := manager.CheckActiveConnections()
		if active {
			t.Error("expected no active connections")
		}
	})

	t.Run("no processed connections", func(t *testing.T) {
		manager, _ := newBackendManager(ultraviolet.BackendFactory)
		cfg := config.ServerConfig{
			FilePath: "123",
			Domains:  []string{"uv"},
			ProxyTo:  "1",
		}
		manager.AddConfig(cfg)
		active := manager.CheckActiveConnections()
		if active {
			t.Error("expected no active connections")
		}
	})

	t.Run("active connection", func(t *testing.T) {
		domain := "uv"
		manager, workerManager := newBackendManager(ultraviolet.BackendFactory)
		cfg := config.ServerConfig{
			FilePath:         "123",
			Domains:          []string{domain},
			ProxyTo:          "1",
			CheckStateOption: "online",
		}
		manager.AddConfig(cfg)

		ch := workerManager.something[domain]
		ansCh := make(chan ultraviolet.ProcessAnswer)
		req := ultraviolet.BackendRequest{
			Type: mc.LOGIN,
			Ch:   ansCh,
		}
		ch <- req
		ans := <-ansCh
		ans.ProxyCh() <- ultraviolet.PROXY_OPEN
		time.Sleep(defaultChTimeout)

		active := manager.CheckActiveConnections()
		if !active {
			t.Error("expected there to be active connection")
		}
	})

	t.Run("multiple active connections", func(t *testing.T) {
		domain := "uv"
		manager, workerManager := newBackendManager(ultraviolet.BackendFactory)
		cfg := config.ServerConfig{
			FilePath:         "123",
			Domains:          []string{domain},
			ProxyTo:          "1",
			CheckStateOption: "online",
		}
		manager.AddConfig(cfg)

		for _, ch := range workerManager.something {
			count := rand.Intn(3)
			for i := 0; i < count; i++ {
				ansCh := make(chan ultraviolet.ProcessAnswer)
				req := ultraviolet.BackendRequest{
					Type: mc.LOGIN,
					Ch:   ansCh,
				}
				ch <- req
				ans := <-ansCh
				ans.ProxyCh() <- ultraviolet.PROXY_OPEN
			}
		}
		time.Sleep(defaultChTimeout)

		active := manager.CheckActiveConnections()
		if !active {
			t.Error("expected there to be active connection")
		}
	})
}
