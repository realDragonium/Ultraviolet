package worker_test

import (
	"io/ioutil"
	"math/rand"
	"net"
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"

	"github.com/realDragonium/Ultraviolet/config"
	"github.com/realDragonium/Ultraviolet/core"
	"github.com/realDragonium/Ultraviolet/mc"
	"github.com/realDragonium/Ultraviolet/worker"
)

type testConfigReader struct {
	cfgs []config.ServerConfig
}

func (reader testConfigReader) Read() ([]config.ServerConfig, error) {
	return reader.cfgs, nil
}

func newTestWorkerManager() testWorkerManager {
	return testWorkerManager{
		something: make(map[string]chan<- worker.BackendRequest),
	}
}

type testWorkerManager struct {
	timesAddCalled    int
	timesRemoveCalled int
	something         map[string]chan<- worker.BackendRequest
}

func (man *testWorkerManager) AddBackend(domains []string, ch chan<- worker.BackendRequest) {
	man.timesAddCalled++
	for _, domain := range domains {
		man.something[domain] = ch
	}
}
func (man *testWorkerManager) RemoveBackend(domains []string) {
	man.timesRemoveCalled++
	for _, domain := range domains {
		delete(man.something, domain)
	}
}
func (man *testWorkerManager) KnowsDomain(domain string) bool {
	_, knows := man.something[domain]
	return knows
}
func (man *testWorkerManager) Register(wrk worker.UpdatableWorker, update bool) {
}

func (man *testWorkerManager) Start() error {
	return nil
}
func (man *testWorkerManager) SetReqChan(reqCh <-chan net.Conn) {
}

func newTestBackend() testBackend {
	return testBackend{
		reqCh: make(chan worker.BackendRequest),
	}
}

type testBackend struct {
	reqCh        chan worker.BackendRequest
	calledClose  bool
	calledUpdate bool
	timesClosed  int
	timesUpdated int
}

func (backend *testBackend) ReqCh() chan<- worker.BackendRequest {
	return backend.reqCh
}
func (backend *testBackend) HasActiveConn() bool {
	return false
}
func (backend *testBackend) Update(cfg worker.BackendConfig) error {
	backend.calledUpdate = true
	backend.timesUpdated++
	return nil
}
func (backend *testBackend) Close() {
	backend.calledClose = true
	backend.timesClosed++
}

func newSimpleBackendManager() worker.BackendManager {
	workerManager := newTestWorkerManager()
	factory := func(cfg config.BackendWorkerConfig) worker.Backend {
		backend := newTestBackend()
		return &backend
	}
	reader := newReaderWithDefaultConfig()
	backend, _ := worker.NewBackendManager(&workerManager, factory, reader.Read)
	return backend
}

func newBackendManagerWithReader(reader config.ServerConfigReader) worker.BackendManager {
	factory := func(cfg config.BackendWorkerConfig) worker.Backend {
		backend := newTestBackend()
		return &backend
	}
	workerManager := newTestWorkerManager()
	backend, _ := worker.NewBackendManager(&workerManager, factory, reader)
	return backend
}

func newReaderWithConfig(cfg config.ServerConfig) *testConfigReader {
	return &testConfigReader{
		cfgs: []config.ServerConfig{
			cfg,
		},
	}
}

func newReaderWithDefaultConfig() *testConfigReader {
	cfg := config.ServerConfig{
		FilePath: "123",
		Domains:  []string{"uv"},
		ProxyTo:  "1",
	}
	return newReaderWithConfig(cfg)
}

func TestBackendManager(t *testing.T) {
	t.Run("adding new config calls factory to create new backend", func(t *testing.T) {
		ch := make(chan struct{})
		factory := func(cfg config.BackendWorkerConfig) worker.Backend {
			ch <- struct{}{}
			backend := newTestBackend()
			return &backend
		}
		workerManager := newTestWorkerManager()
		reader := newReaderWithDefaultConfig()
		go worker.NewBackendManager(&workerManager, factory, reader.Read)
		select {
		case <-ch:
			t.Log("factory has been called")
		case <-time.After(defaultChTimeout):
			t.Error("expected factory to be called but wasnt")
		}
	})

	t.Run("fault config cause and error without any update", func(t *testing.T) {
		factory := func(cfg config.BackendWorkerConfig) worker.Backend {
			backend := newTestBackend()
			return &backend
		}
		workerManager := newTestWorkerManager()
		tmpDir, _ := ioutil.TempDir("", "backend-manager")
		defer os.Remove(tmpDir)
		filePath := filepath.Join(tmpDir, "keyFile")
		os.WriteFile(filePath, []byte{}, os.ModePerm)
		reader := &testConfigReader{
			cfgs: []config.ServerConfig{
				{
					FilePath: "123",
					Domains:  []string{"uv"},
					ProxyTo:  "1",
				},
				{
					FilePath:  "123",
					Domains:   []string{"uv"},
					ProxyTo:   "1",
					NewRealIP: true,
					RealIPKey: filePath,
				},
			},
		}
		_, err := worker.NewBackendManager(&workerManager, factory, reader.Read)
		if !strings.HasPrefix(err.Error(), "x509") {
			t.Errorf("expected an privat key error but got: %v", err)
		}
		if workerManager.timesAddCalled != 0 {
			t.Errorf("expected to be called zero times but was called %d", workerManager.timesAddCalled)
		}
		if workerManager.timesRemoveCalled != 0 {
			t.Errorf("expected to be called zero times but was called %d", workerManager.timesRemoveCalled)
		}
	})

	t.Run("adding new config calls worker manager to update", func(t *testing.T) {
		factory := func(cfg config.BackendWorkerConfig) worker.Backend {
			backend := newTestBackend()
			return &backend
		}
		workerManager := newTestWorkerManager()
		reader := newReaderWithDefaultConfig()
		worker.NewBackendManager(&workerManager, factory, reader.Read)
		if workerManager.timesAddCalled != 1 {
			t.Errorf("expected to be called once but was called %d", workerManager.timesAddCalled)
		}
	})

	t.Run("adding already registered configs doesnt register them again", func(t *testing.T) {
		factory := func(cfg config.BackendWorkerConfig) worker.Backend {
			backend := newTestBackend()
			return &backend
		}
		workerManager := newTestWorkerManager()
		reader := newReaderWithDefaultConfig()
		manager, _ := worker.NewBackendManager(&workerManager, factory, reader.Read)
		manager.Update()
		if workerManager.timesAddCalled != 1 {
			t.Errorf("expected to be called once but was called %d", workerManager.timesAddCalled)
		}
	})

	t.Run("removing a config should update worker manager", func(t *testing.T) {
		factory := func(cfg config.BackendWorkerConfig) worker.Backend {
			backend := newTestBackend()
			return &backend
		}
		workerManager := newTestWorkerManager()
		reader := newReaderWithDefaultConfig()
		read := func() ([]config.ServerConfig, error) {
			return reader.Read()
		}
		manager, err := worker.NewBackendManager(&workerManager, factory, read)
		if err != nil {
			t.Fatalf("got error: %v", err)
		}
		reader.cfgs = []config.ServerConfig{}
		err = manager.Update()
		if err != nil {
			t.Fatalf("got error: %v", err)
		}
		if workerManager.timesRemoveCalled != 1 {
			t.Errorf("expected to be called once but was called %d", workerManager.timesRemoveCalled)
		}
	})

	t.Run("removing a config should update backend", func(t *testing.T) {
		b := newTestBackend()
		factory := func(cfg config.BackendWorkerConfig) worker.Backend {
			return &b
		}
		workerManager := newTestWorkerManager()
		reader := newReaderWithDefaultConfig()
		read := func() ([]config.ServerConfig, error) {
			return reader.Read()
		}
		manager, _ := worker.NewBackendManager(&workerManager, factory, read)
		reader.cfgs = []config.ServerConfig{}
		manager.Update()
		if !b.calledClose {
			t.Error("expected close to be called")
		}
	})

	t.Run("updates backend if values have changed", func(t *testing.T) {
		b := newTestBackend()
		factory := func(cfg config.BackendWorkerConfig) worker.Backend {
			return &b
		}
		workerManager := newTestWorkerManager()
		reader := newReaderWithDefaultConfig()
		manager, _ := worker.NewBackendManager(&workerManager, factory, reader.Read)
		reader.cfgs[0].ProxyTo = "2"
		manager.Update()
		if !b.calledUpdate {
			t.Error("expected to be updated")
		}
	})

	t.Run("does NOT update backend if values have NOT changed", func(t *testing.T) {
		b := newTestBackend()
		factory := func(cfg config.BackendWorkerConfig) worker.Backend {
			return &b
		}
		workerManager := newTestWorkerManager()
		reader := newReaderWithDefaultConfig()
		manager, _ := worker.NewBackendManager(&workerManager, factory, reader.Read)
		manager.Update()
		if b.calledUpdate {
			t.Error("should not have been updated")
		}
	})

	t.Run("adding new config calls worker manager to register new domains", func(t *testing.T) {
		newDomain := "uv1"
		factory := func(cfg config.BackendWorkerConfig) worker.Backend {
			backend := newTestBackend()
			return &backend
		}
		workerManager := newTestWorkerManager()
		reader := newReaderWithDefaultConfig()
		manager, _ := worker.NewBackendManager(&workerManager, factory, reader.Read)
		workerManager.timesAddCalled = 0
		reader.cfgs[0].Domains = append(reader.cfgs[0].Domains, newDomain)
		manager.Update()
		if workerManager.timesAddCalled != 1 {
			t.Errorf("expected to be called once but was called %d", workerManager.timesAddCalled)
		}
	})

	t.Run("removing new config calls worker manager to remove domain", func(t *testing.T) {
		factory := func(cfg config.BackendWorkerConfig) worker.Backend {
			backend := newTestBackend()
			return &backend
		}
		workerManager := newTestWorkerManager()
		cfg := config.ServerConfig{
			FilePath: "123",
			Domains:  []string{"uv", "uv1"},
			ProxyTo:  "1",
		}
		reader := newReaderWithConfig(cfg)
		manager, _ := worker.NewBackendManager(&workerManager, factory, reader.Read)
		reader.cfgs[0].Domains = []string{"uv"}
		manager.Update()
		if workerManager.timesRemoveCalled != 1 {
			t.Errorf("expected to be called once but was called %d", workerManager.timesAddCalled)
		}
	})

	t.Run("update domain by worker when old config is removed and new config is added with same domain", func(t *testing.T) {
		domain := "uv"
		factory := func(cfg config.BackendWorkerConfig) worker.Backend {
			backend := newTestBackend()
			return &backend
		}
		workerManager := newTestWorkerManager()
		cfg := config.ServerConfig{
			FilePath: "123",
			Domains:  []string{domain},
			ProxyTo:  "1",
		}
		reader := newReaderWithConfig(cfg)
		read := func() ([]config.ServerConfig, error) {
			return reader.Read()
		}
		manager, _ := worker.NewBackendManager(&workerManager, factory, read)
		backendCh1 := workerManager.something[domain]
		reader.cfgs = []config.ServerConfig{
			{
				FilePath: "124",
				Domains:  []string{domain},
				ProxyTo:  "1",
			},
		}
		manager.Update()
		backendCh2, ok := workerManager.something[domain]
		if !ok {
			t.Error("expected worker manager to have channel to backend")
		}
		if workerManager.timesRemoveCalled != 1 {
			t.Errorf("expected to be called once but was called %d", workerManager.timesRemoveCalled)
		}
		if workerManager.timesAddCalled != 2 {
			t.Errorf("expected to be called twice but was called %d", workerManager.timesAddCalled)
		}
		if backendCh1 == backendCh2 {
			t.Error("backend channels shouldnt be the same")
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
		reader := newReaderWithDefaultConfig()
		manager := newBackendManagerWithReader(reader.Read)
		manager.Update()
		active := manager.CheckActiveConnections()
		if active {
			t.Error("expected no active connections")
		}
	})

	t.Run("active connection", func(t *testing.T) {
		domain := "uv"
		factory := func(cfg config.BackendWorkerConfig) worker.Backend {
			backend := worker.BackendFactory(cfg)
			return backend
		}
		workerManager := newTestWorkerManager()
		cfg := config.ServerConfig{
			FilePath:         "123",
			Domains:          []string{domain},
			ProxyTo:          "1",
			CheckStateOption: "online",
		}
		reader := newReaderWithConfig(cfg)
		manager, _ := worker.NewBackendManager(&workerManager, factory, reader.Read)

		ch := workerManager.something[domain]
		ansCh := make(chan worker.BackendAnswer)
		req := worker.BackendRequest{
			ReqData: core.RequestData{
				Type: mc.Login,
			},
			Ch: ansCh,
		}
		ch <- req
		ans := <-ansCh
		ans.ProxyCh() <- worker.ProxyOpen
		time.Sleep(defaultChTimeout)

		active := manager.CheckActiveConnections()
		if !active {
			t.Error("expected there to be active connection")
		}
	})

	t.Run("multiple active connections", func(t *testing.T) {
		domain := "uv"
		cfg := config.ServerConfig{
			FilePath:         "123",
			Domains:          []string{domain},
			ProxyTo:          "1",
			CheckStateOption: "online",
		}
		factory := func(cfg config.BackendWorkerConfig) worker.Backend {
			backend := worker.BackendFactory(cfg)
			return backend
		}
		workerManager := newTestWorkerManager()
		reader := newReaderWithConfig(cfg)
		manager, _ := worker.NewBackendManager(&workerManager, factory, reader.Read)

		for _, ch := range workerManager.something {
			count := rand.Intn(3)
			for i := 0; i < count; i++ {
				ansCh := make(chan worker.BackendAnswer)
				req := worker.BackendRequest{
					ReqData: core.RequestData{
						Type: mc.Login,
					},
					Ch: ansCh,
				}
				ch <- req
				ans := <-ansCh
				ans.ProxyCh() <- worker.ProxyOpen
			}
		}
		time.Sleep(defaultChTimeout)

		active := manager.CheckActiveConnections()
		if !active {
			t.Error("expected there to be active connection")
		}
	})
}
