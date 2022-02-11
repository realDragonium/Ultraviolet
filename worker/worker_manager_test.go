package worker_test

import (
	"testing"

	"github.com/realDragonium/Ultraviolet/config"
	"github.com/realDragonium/Ultraviolet/worker"
)

type testUpdatableWorkerCounter struct {
	updatesReceived int
}

func (worker *testUpdatableWorkerCounter) Update(data map[string]chan<- worker.BackendRequest) {
	worker.updatesReceived++
}

func TestRegisterServerConfig(t *testing.T) {
	cfg := config.UltravioletConfig{}
	t.Run("add backend", func(t *testing.T) {
		manager := worker.NewWorkerManager(config.NewUVReader(cfg), nil)
		domains := []string{"uv", "uv2"}
		ch := make(chan worker.BackendRequest)
		manager.AddBackend(domains, ch)
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
		ch := make(chan worker.BackendRequest)
		manager.AddBackend(domains, ch)

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
		ch := make(chan worker.BackendRequest)
		manager.AddBackend(domains, ch)

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
