package config

import (
	"github.com/realDragonium/Ultraviolet/mc"
)

type ServerConfig struct {
	MainDomain   string   `json:"mainDomain"`
	ExtraDomains []string `json:"extraDomains"`

	ProxyTo           string `json:"proxyTo"`
	ProxyBind         string `json:"proxyBind"`
	DialTimeout       string `json:"dialTimeout"`
	SendProxyProtocol bool   `json:"sendProxyProtocol"`

	DisconnectMessage string `json:"disconnectMessage"`

	OfflineStatus mc.AnotherStatusResponse `json:"offlineStatus"`
	OnlineStatus  mc.AnotherStatusResponse `json:"onlineStatus"`

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
}
