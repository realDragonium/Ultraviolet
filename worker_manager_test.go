package ultraviolet_test

import (
	"testing"
	"time"

	ultraviolet "github.com/realDragonium/Ultraviolet"
)

func newTestUpdatableWorker() testUpdatableWorker {
	return testUpdatableWorker{
		ch: make(chan map[string]chan<- ultraviolet.BackendRequest),
	}
}

type testUpdatableWorker struct {
	ch chan map[string]chan<- ultraviolet.BackendRequest
}

func (worker *testUpdatableWorker) UpdateCh() chan<- map[string]chan<- ultraviolet.BackendRequest {
	return worker.ch
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
		worker := newTestUpdatableWorker()
		go manager.Register(&worker, true)

		select {
		case <-worker.ch:
			t.Log("received update")
		case <-time.After(defaultChTimeout):
			t.Fatal("expected to receive an update")
		}
	})

	t.Run("doesnt update workers when registering", func(t *testing.T) {
		manager := ultraviolet.NewWorkerManager()
		worker := newTestUpdatableWorker()
		go manager.Register(&worker, false)

		select {
		case <-worker.ch:
			t.Fatal("received update")
		case <-time.After(defaultChTimeout):
			t.Log("as expected no update")
		}
	})

	t.Run("updates workers adding backend", func(t *testing.T) {
		manager := ultraviolet.NewWorkerManager()
		worker := newTestUpdatableWorker()
		manager.Register(&worker, false)

		domain := "uv2"
		domains := []string{"uv", domain}
		ch := make(chan ultraviolet.BackendRequest)
		go manager.AddBackend(domains, ch)

		select {
		case <-worker.ch:
			t.Log("received update")
		case <-time.After(defaultChTimeout):
			t.Fatal("expected to receive an update")
		}
	})

	t.Run("updates workers removing backend", func(t *testing.T) {
		manager := ultraviolet.NewWorkerManager()
		worker := newTestUpdatableWorker()
		manager.Register(&worker, false)

		domain := "uv2"
		domains := []string{"uv", domain}
		go manager.RemoveBackend(domains)

		select {
		case <-worker.ch:
			t.Log("received update")
		case <-time.After(defaultChTimeout):
			t.Fatal("expected to receive an update")
		}
	})

}
