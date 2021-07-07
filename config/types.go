package config

import "github.com/realDragonium/Ultraviolet/mc"

type ServerConfig struct {
	MainDomain        string                   `json:"mainDomain"`
	ExtraDomains      []string                 `json:"extraDomains"`
	ProxyTo           string                   `json:"proxyTo"`
	ProxyBind         string                   `json:"proxyBind"`
	SendProxyProtocol bool                     `json:"sendProxyProtocol"`
	DisconnectMessage string                   `json:"disconnectMessage"`
	OfflineStatus     mc.AnotherStatusResponse `json:"offlineStatus"`
	ConnLimitBackend  int                      `json:"connPerSec"`
}

func DefaultServerConfig() ServerConfig {
	return ServerConfig{
		ProxyBind:         "0.0.0.0",
		ProxyTo:           ":25566",
		MainDomain:        "localhost",
		DisconnectMessage: "Sorry {{username}}, but the server is offline.",
		OfflineStatus: mc.AnotherStatusResponse{
			Name:        "Ultraviolet",
			Protocol:    755,
			Description: "Some broken proxy",
		},
		ConnLimitBackend: 5,
	}
}

type UltravioletConfig struct {
	ListenTo             string                   `json:"listenTo"`
	NumberOfWorkers      int                      `json:"numberOfWorkers"`
	ReceiveProxyProtocol bool                     `json:"receiveProxyProtocol"`
	DefaultStatus        mc.AnotherStatusResponse `json:"defaultStatus"`
}

func DefaultUltravioletConfig() UltravioletConfig {
	return UltravioletConfig{
		ListenTo:             ":25565",
		NumberOfWorkers:      5,
		ReceiveProxyProtocol: false,
		DefaultStatus: mc.AnotherStatusResponse{
			Name:        "Ultraviolet",
			Protocol:    755,
			Description: "Some broken proxy",
		},
	}
}
