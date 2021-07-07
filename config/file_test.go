package config_test

import (
	"bytes"
	"testing"

	"github.com/realDragonium/Ultraviolet/config"
	"github.com/realDragonium/Ultraviolet/mc"
)

func samePk(expected, received mc.Packet) bool {
	sameID := expected.ID == received.ID
	sameData := bytes.Equal(expected.Data, received.Data)

	return sameID && sameData
}

func TestFileToWorkerConfig(t *testing.T) {
	serverCfg := config.ServerConfig{
		MainDomain:        "Ultraviolet",
		ExtraDomains:      []string{"Ultraviolet2", "UltraV", "UV"},
		ProxyTo:           "127.0.10.5:25565",
		ProxyBind:         "127.0.0.5",
		SendProxyProtocol: true,
		DisconnectMessage: "HelloThereWeAreClosed...Sorry",
		OfflineStatus: mc.AnotherStatusResponse{
			Name:        "Ultraviolet",
			Protocol:    755,
			Description: "Some broken proxy",
		},
		ConnLimitBackend: 5,
	}

	expectedDisconPk := mc.ClientBoundDisconnect{
		Reason: mc.String(serverCfg.DisconnectMessage),
	}.Marshal()
	expectedOfflineStatus := mc.AnotherStatusResponse{
		Name:        "Ultraviolet",
		Protocol:    755,
		Description: "Some broken proxy",
	}.Marshal()

	workerCfg := config.FileToWorkerConfig(serverCfg)

	if workerCfg.ProxyTo != serverCfg.ProxyTo {
		t.Errorf("expected: %v - got: %v", serverCfg.ProxyTo, workerCfg.ProxyTo)
	}
	if workerCfg.ProxyBind != serverCfg.ProxyBind {
		t.Errorf("expected: %v - got: %v", serverCfg.ProxyBind, workerCfg.ProxyBind)
	}
	if workerCfg.SendProxyProtocol != serverCfg.SendProxyProtocol {
		t.Errorf("expected: %v - got: %v", serverCfg.SendProxyProtocol, workerCfg.SendProxyProtocol)
	}
	if workerCfg.ConnLimitBackend != serverCfg.ConnLimitBackend {
		t.Errorf("expected: %v - got: %v", serverCfg.ConnLimitBackend, workerCfg.ConnLimitBackend)
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
