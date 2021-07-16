package proxy_test

import (
	"testing"
	"time"

	"github.com/realDragonium/Ultraviolet/config"
	"github.com/realDragonium/Ultraviolet/proxy"
)

var (
	defaultChTimeout = 10 * time.Millisecond
	longerChTimeout  = 100 * time.Millisecond
)

func TestProxy_StartCorrectAmountOfWorkers_PublicPrivate(t *testing.T) {
	reqCh := make(chan proxy.McRequest)
	cfg := config.UltravioletConfig{
		NumberOfWorkers: 1,
	}
	gateway := proxy.NewGateway()
	gateway.StartWorkers(cfg, nil, reqCh)
	answerCh := make(chan proxy.McAnswer)
	req := proxy.McRequest{
		Ch: answerCh,
	}
	reqCh <- req
	select {
	case reqCh <- proxy.McRequest{}:
		t.Error("worker has received request")
	case <-time.After(defaultChTimeout):
		t.Log("timed out")
	}
}

func TestShutdown_ReturnsWhenThereAreNoOpenConnections(t *testing.T) {
	createConfigs := func(addr string) (config.UltravioletConfig, []config.ServerConfig) {
		cfg := config.UltravioletConfig{
			NumberOfWorkers: 1,
		}
		serverCfgs := []config.ServerConfig{
			{
				Domains: []string{"uv"},
				ProxyTo: addr,
			},
			{
				Domains: []string{"uv1"},
				ProxyTo: addr,
			},
		}
		return cfg, serverCfgs
	}
	testShutdown_DoesReturn := func(t *testing.T, gw proxy.Gateway) {
		finishedCh := make(chan struct{})
		go func() {
			gw.Shutdown()
			finishedCh <- struct{}{}
		}()
		select {
		case <-finishedCh:
			t.Log("method call has returned")
		case <-time.After(defaultChTimeout):
			t.Error("timed out")
		}
	}
	testShutdown_DoesntReturn := func(t *testing.T, gw proxy.Gateway) {
		finishedCh := make(chan struct{})
		go func() {
			gw.Shutdown()
			finishedCh <- struct{}{}
		}()
		select {
		case <-finishedCh:
			t.Error("method call has returned")
		case <-time.After(defaultChTimeout):
			t.Log("timed out")
		}
	}

	startWorker := func(addr string) (proxy.Gateway, chan proxy.McRequest) {
		gw := proxy.NewGateway()
		cfg, serverCfgs := createConfigs(addr)
		reqCh := make(chan proxy.McRequest)
		gw.StartWorkers(cfg, serverCfgs, reqCh)
		return gw, reqCh
	}

	t.Run("when a fresh proxy has been made", func(t *testing.T) {
		p := proxy.NewGateway()
		testShutdown_DoesReturn(t, p)
	})

	t.Run("With workers active", func(t *testing.T) {
		gw, _ := startWorker("")
		testShutdown_DoesReturn(t, gw)
	})

	t.Run("With active connections", func(t *testing.T) {
		targetAddr := testAddr()
		gw, reqCh := startWorker(targetAddr)

		acceptAllConnsListener(t, targetAddr)
		answerCh := make(chan proxy.McAnswer)
		reqCh <- proxy.McRequest{
			Type:       proxy.STATUS,
			ServerAddr: "uv",
			Ch:         answerCh,
		}
		answer := <-answerCh
		answer.ProxyCh() <- proxy.PROXY_OPEN
		time.Sleep(defaultChTimeout)
		testShutdown_DoesntReturn(t, gw)
	})

	t.Run("When active connection is closed", func(t *testing.T) {
		targetAddr := testAddr()
		gw, reqCh := startWorker(targetAddr)

		acceptAllConnsListener(t, targetAddr)
		answerCh := make(chan proxy.McAnswer)
		reqCh <- proxy.McRequest{
			Type:       proxy.STATUS,
			ServerAddr: "uv",
			Ch:         answerCh,
		}
		answer := <-answerCh
		answer.ProxyCh() <- proxy.PROXY_OPEN
		time.Sleep(defaultChTimeout)
		answer.ProxyCh() <- proxy.PROXY_CLOSE
		testShutdown_DoesReturn(t, gw)
	})

	t.Run("With 2 open close 1 and still doesnt return", func(t *testing.T) {
		targetAddr := testAddr()
		gw, reqCh := startWorker(targetAddr)

		acceptAllConnsListener(t, targetAddr)
		answerCh := make(chan proxy.McAnswer)
		reqCh <- proxy.McRequest{
			Type:       proxy.STATUS,
			ServerAddr: "uv",
			Ch:         answerCh,
		}
		answer := <-answerCh
		answer.ProxyCh() <- proxy.PROXY_OPEN
		answer.ProxyCh() <- proxy.PROXY_OPEN
		finishedCh := make(chan struct{})
		go func() {
			gw.Shutdown()
			finishedCh <- struct{}{}
		}()
		answer.ProxyCh() <- proxy.PROXY_CLOSE
		select {
		case <-finishedCh:
			t.Error("method call has returned")
		case <-time.After(defaultChTimeout):
			t.Log("timed out")
		}
	})

	t.Run("With 2 different server connections close 1 and still doesnt return", func(t *testing.T) {
		targetAddr := testAddr()
		gw, reqCh := startWorker(targetAddr)

		acceptAllConnsListener(t, targetAddr)
		answerCh := make(chan proxy.McAnswer)
		reqCh <- proxy.McRequest{
			Type:       proxy.STATUS,
			ServerAddr: "uv",
			Ch:         answerCh,
		}
		answer1 := <-answerCh
		answer1.ProxyCh() <- proxy.PROXY_OPEN

		answerCh2 := make(chan proxy.McAnswer)
		reqCh <- proxy.McRequest{
			Type:       proxy.STATUS,
			ServerAddr: "uv1",
			Ch:         answerCh2,
		}
		answer2 := <-answerCh2
		answer2.ProxyCh() <- proxy.PROXY_OPEN

		finishedCh := make(chan struct{})
		go func() {
			gw.Shutdown()
			finishedCh <- struct{}{}
		}()
		answer1.ProxyCh() <- proxy.PROXY_CLOSE
		select {
		case <-finishedCh:
			t.Error("method call has returned")
		case <-time.After(defaultChTimeout):
			t.Log("timed out")
		}
	})

}
