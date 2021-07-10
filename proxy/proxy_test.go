package proxy_test

import (
	"testing"
	"time"

	"github.com/realDragonium/Ultraviolet/config"
	"github.com/realDragonium/Ultraviolet/mc"
	"github.com/realDragonium/Ultraviolet/proxy"
)

func TestFileToWorkerConfig(t *testing.T) {
	serverCfg := config.ServerConfig{
		Domains:           []string{"Ultraviolet", "Ultraviolet2", "UltraV", "UV"},
		ProxyTo:           "127.0.10.5:25565",
		ProxyBind:         "127.0.0.5",
		DialTimeout:       "1s",
		SendProxyProtocol: true,
		DisconnectMessage: "HelloThereWeAreClosed...Sorry",
		OfflineStatus: mc.AnotherStatusResponse{
			Name:        "Ultraviolet",
			Protocol:    755,
			Description: "Some broken proxy",
		},
		RateLimit:      5,
		RateDuration:   "1m",
		UpdateCooldown: "1m",
	}

	expectedDisconPk := mc.ClientBoundDisconnect{
		Reason: mc.String(serverCfg.DisconnectMessage),
	}.Marshal()
	expectedOfflineStatus := mc.AnotherStatusResponse{
		Name:        "Ultraviolet",
		Protocol:    755,
		Description: "Some broken proxy",
	}.Marshal()
	expectedRateDuration := 1 * time.Minute
	expectedUpdateCooldown := 1 * time.Minute
	expectedDialTimeout := 1 * time.Second

	workerCfg := proxy.FileToWorkerConfig(serverCfg)

	if workerCfg.ProxyTo != serverCfg.ProxyTo {
		t.Errorf("expected: %v - got: %v", serverCfg.ProxyTo, workerCfg.ProxyTo)
	}
	if workerCfg.ProxyBind != serverCfg.ProxyBind {
		t.Errorf("expected: %v - got: %v", serverCfg.ProxyBind, workerCfg.ProxyBind)
	}
	if workerCfg.SendProxyProtocol != serverCfg.SendProxyProtocol {
		t.Errorf("expected: %v - got: %v", serverCfg.SendProxyProtocol, workerCfg.SendProxyProtocol)
	}
	if workerCfg.RateLimit != serverCfg.RateLimit {
		t.Errorf("expected: %v - got: %v", serverCfg.RateLimit, workerCfg.RateLimit)
	}
	if expectedRateDuration != workerCfg.RateLimitDuration {
		t.Errorf("expected: %v - got: %v", expectedRateDuration, workerCfg.RateLimitDuration)
	}
	if expectedUpdateCooldown != workerCfg.StateUpdateCooldown {
		t.Errorf("expected: %v - got: %v", expectedRateDuration, workerCfg.StateUpdateCooldown)
	}
	if expectedDialTimeout != workerCfg.DialTimeout {
		t.Errorf("expected: %v - got: %v", expectedDialTimeout, workerCfg.DialTimeout)
	}
	if !samePk(expectedOfflineStatus, workerCfg.OfflineStatus) {
		offlineStatus, _ := mc.UnmarshalClientBoundResponse(expectedOfflineStatus)
		receivedStatus, _ := mc.UnmarshalClientBoundResponse(workerCfg.OfflineStatus)
		t.Errorf("expcted: %v \ngot: %v", offlineStatus, receivedStatus)
	}

	if !samePk(expectedDisconPk, workerCfg.DisconnectPacket) {
		expectedDiscon, _ := mc.UnmarshalClientDisconnect(expectedDisconPk)
		receivedDiscon, _ := mc.UnmarshalClientDisconnect(workerCfg.DisconnectPacket)
		t.Errorf("expcted: %v \ngot: %v", expectedDiscon, receivedDiscon)
	}
}
