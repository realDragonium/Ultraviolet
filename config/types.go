package config

import "github.com/realDragonium/Ultraviolet/mc"

type ServerConfig struct {
	BackendCfg  BackendConnConfig `json:"backend"`
	ListenCfg   ListenConfig      `json:"listen"`
	FallOverCfg FallOverConfig    `json:"fallOver"`

	ConnLimitBackend int `json:"connPerSec"`
}

type BackendConnConfig struct {
	ProxyBind         string `json:"proxyBind"`
	ProxyTo           string `json:"proxyTo"`
	SendProxyProtocol bool   `json:"sendProxyProtocol"`
}

type ListenConfig struct {
	MainDomain   string   `json:"mainDomain"`
	ExtraDomains []string `json:"extraDomains"`
}

type FallOverConfig struct {
	DisconnectMessage string                   `json:"disconnectMessage"`
	OfflineStatus     mc.AnotherStatusResponse `json:"offlineStatus"`
}

func DefaultServerConfig() ServerConfig {
	return ServerConfig{
		BackendCfg: BackendConnConfig{
			ProxyBind: "0.0.0.0",
			ProxyTo:   ":25566",
		},
		ListenCfg: ListenConfig{
			MainDomain: "localhost",
		},
		FallOverCfg: FallOverConfig{
			DisconnectMessage: "Sorry {{username}}, but the server is offline.",
			OfflineStatus: mc.AnotherStatusResponse{
				Name:        "Ultraviolet",
				Protocol:    755,
				Description: "Some broken proxy",
			},
		},
		ConnLimitBackend: 5,
	}
}

type UltravioletConfig struct {
	ListenTo string `json:"listenTo"`

	NumberOfWorkers      int  `json:"numberOfWorkers"`
	ReceiveProxyProtocol bool `json:"receiveProxyProtocol"`

	DefaultStatus mc.AnotherStatusResponse `json:"defaultStatus"`
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
