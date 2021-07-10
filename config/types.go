package config

import (
	"io"

	"github.com/realDragonium/Ultraviolet/mc"
)

type ServerConfig struct {
	Domains []string `json:"domains"`

	ProxyTo           string `json:"proxyTo"`
	ProxyBind         string `json:"proxyBind"`
	DialTimeout       string `json:"dialTimeout"`
	SendProxyProtocol bool   `json:"sendProxyProtocol"`

	DisconnectMessage string `json:"disconnectMessage"`

	OfflineStatus mc.AnotherStatusResponse `json:"offlineStatus"`

	RateLimit      int    `json:"rateLimit"`
	RateDuration   string `json:"rateCooldown"`
	UpdateCooldown string `json:"stateUpdateCooldown"`
}

type UltravioletConfig struct {
	ListenTo             string                   `json:"listenTo"`
	ReceiveProxyProtocol bool                     `json:"receiveProxyProtocol"`
	DefaultStatus        mc.AnotherStatusResponse `json:"defaultStatus"`

	NumberOfWorkers       int `json:"numberOfWorkers"`
	NumberOfConnWorkers   int `json:"numberOfConnWorkers"`
	NumberOfStatusWorkers int `json:"numberOfStatusWorkers"`

	LogOutput io.Writer
}
