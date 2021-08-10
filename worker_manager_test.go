package ultraviolet_test

import (
	"testing"

	ultraviolet "github.com/realDragonium/Ultraviolet"
)

type testUpdatableWorkerCounter struct {
	updatesReceived int
}

func (worker *testUpdatableWorkerCounter) Update(data map[string]chan<- ultraviolet.BackendRequest) {
	worker.updatesReceived++
}

func TestRegisterServerConfig(t *testing.T) {
	t.Run("add backend", func(t *testing.T) {
		manager := ultraviolet.NewWorkerManager()
		domains := []string{"uv", "uv2"}
		ch := make(chan ultraviolet.BackendRequest)
		manager.AddBackend(domains, ch)
		for _, domain := range domains {
			if !manager.KnowsDomain(domain) {
				t.Error("manager should have known this domain")
			}
		}
	})

	t.Run("remove backend", func(t *testing.T) {
		manager := ultraviolet.NewWorkerManager()
		domain := "uv2"
		domains := []string{"uv", domain}
		ch := make(chan ultraviolet.BackendRequest)
		manager.AddBackend(domains, ch)

		removeDomains := []string{domain}
		manager.RemoveBackend(removeDomains)

		if manager.KnowsDomain(domain) {
			t.Error("manager should NOT have known this domain")
		}
	})

	t.Run("updates workers when registering", func(t *testing.T) {
		manager := ultraviolet.NewWorkerManager()
		worker := testUpdatableWorkerCounter{}
		manager.Register(&worker, true)

		if worker.updatesReceived != 1 {
			t.Fatal("expected to receive an update")
		}
	})

	t.Run("doesnt update workers when registering", func(t *testing.T) {
		manager := ultraviolet.NewWorkerManager()
		worker := testUpdatableWorkerCounter{}
		manager.Register(&worker, false)

		if worker.updatesReceived != 0 {
			t.Fatal("should NOT have received an update")
		}
	})

	t.Run("does updates when adding backend", func(t *testing.T) {
		manager := ultraviolet.NewWorkerManager()
		worker := testUpdatableWorkerCounter{}
		manager.Register(&worker, false)

		domain := "uv2"
		domains := []string{"uv", domain}
		ch := make(chan ultraviolet.BackendRequest)
		manager.AddBackend(domains, ch)

		if worker.updatesReceived != 1 {
			t.Fatal("expected to receive an update")
		}
	})

	t.Run("does updates when removing backend", func(t *testing.T) {
		manager := ultraviolet.NewWorkerManager()
		worker := testUpdatableWorkerCounter{}
		manager.Register(&worker, false)

		domain := "uv2"
		domains := []string{"uv", domain}
		manager.RemoveBackend(domains)
		if worker.updatesReceived != 1 {
			t.Fatal("expected to receive an update")
		}
	})

}
