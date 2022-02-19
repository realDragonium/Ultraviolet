package worker_test

import (
	"net"
	"testing"

	"github.com/realDragonium/Ultraviolet/config"
	"github.com/realDragonium/Ultraviolet/core"
	"github.com/realDragonium/Ultraviolet/mc"
	"github.com/realDragonium/Ultraviolet/worker"
)

type testUpdatableWorkerCounter struct {
	updatesReceived int
}

func (worker *testUpdatableWorkerCounter) Update(data core.ServerCatalog) {
	worker.updatesReceived++
}

type testServer struct{}

func (s testServer) ConnAction(req core.RequestData) core.ServerAction {
	return core.CLOSE
}

func (s testServer) CreateConn(req core.RequestData) (c net.Conn, err error) {
	return nil, nil
}

func (s testServer) Status() mc.Packet {
	return mc.Packet{}
}

func TestRegisterServerConfig(t *testing.T) {
	cfg := config.UltravioletConfig{}
	t.Run("add backend", func(t *testing.T) {
		manager := worker.NewWorkerManager(config.NewUVReader(cfg), nil)
		domains := []string{"uv", "uv2"}
		manager.AddBackend(domains, testServer{})
		for _, domain := range domains {
			if !manager.KnowsDomain(domain) {
				t.Error("manager should have known this domain")
			}
		}
	})

	t.Run("remove backend", func(t *testing.T) {
		manager := worker.NewWorkerManager(config.NewUVReader(cfg), nil)
		domain := "uv2"
		domains := []string{"uv", domain}
		manager.AddBackend(domains, testServer{})

		removeDomains := []string{domain}
		manager.RemoveBackend(removeDomains)

		if manager.KnowsDomain(domain) {
			t.Error("manager should NOT have known this domain")
		}
	})

	t.Run("updates workers when registering", func(t *testing.T) {
		manager := worker.NewWorkerManager(config.NewUVReader(cfg), nil)
		wrk := testUpdatableWorkerCounter{}
		manager.Register(&wrk, true)

		if wrk.updatesReceived != 1 {
			t.Fatal("expected to receive an update")
		}
	})

	t.Run("doesnt update workers when registering", func(t *testing.T) {
		manager := worker.NewWorkerManager(config.NewUVReader(cfg), nil)
		wrk := testUpdatableWorkerCounter{}
		manager.Register(&wrk, false)

		if wrk.updatesReceived != 0 {
			t.Fatal("should NOT have received an update")
		}
	})

	t.Run("does updates when adding backend", func(t *testing.T) {
		manager := worker.NewWorkerManager(config.NewUVReader(cfg), nil)
		wrk := testUpdatableWorkerCounter{}
		manager.Register(&wrk, false)

		domain := "uv2"
		domains := []string{"uv", domain}
		manager.AddBackend(domains, testServer{})

		if wrk.updatesReceived != 1 {
			t.Fatal("expected to receive an update")
		}
	})

	t.Run("does updates when removing backend", func(t *testing.T) {
		manager := worker.NewWorkerManager(config.NewUVReader(cfg), nil)
		wrk := testUpdatableWorkerCounter{}
		manager.Register(&wrk, false)

		domain := "uv2"
		domains := []string{"uv", domain}
		manager.RemoveBackend(domains)
		if wrk.updatesReceived != 1 {
			t.Fatal("expected to receive an update")
		}
	})

}
