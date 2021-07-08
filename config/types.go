package config

import (
	"github.com/realDragonium/Ultraviolet/mc"
)

type ServerConfig struct {
	MainDomain   string   `json:"mainDomain"`
	ExtraDomains []string `json:"extraDomains"`

	ProxyTo           string `json:"proxyTo"`
	ProxyBind         string `json:"proxyBind"`
	SendProxyProtocol bool   `json:"sendProxyProtocol"`

	DisconnectMessage string                   `json:"disconnectMessage"`
	OfflineStatus     mc.AnotherStatusResponse `json:"offlineStatus"`

	RateLimit    int    `json:"rateLimit"`
	RateDuration string `json:"rateCooldown"`
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
		RateLimit:    5,
		RateDuration: "1s",
	}
}

type UltravioletConfig struct {
	ListenTo             string                   `json:"listenTo"`
	ReceiveProxyProtocol bool                     `json:"receiveProxyProtocol"`
	DefaultStatus        mc.AnotherStatusResponse `json:"defaultStatus"`

	NumberOfWorkers       int `json:"numberOfWorkers"`
	NumberOfConnWorkers   int `json:"numberOfConnWorkers"`
	NumberOfStateWorkers  int `json:"numberOfStateWorkers"`
	NumberOfStatusWorkers int `json:"numberOfStatusWorkers"`
}

func DefaultUltravioletConfig() UltravioletConfig {
	return UltravioletConfig{
		ListenTo:             ":25565",
		ReceiveProxyProtocol: false,
		DefaultStatus: mc.AnotherStatusResponse{
			Name:        "Ultraviolet",
			Protocol:    755,
			Description: "One dangerous proxy",
		},

		NumberOfWorkers:       5,
		NumberOfConnWorkers:   1,
		NumberOfStateWorkers:  1,
		NumberOfStatusWorkers: 1,
	}
}
