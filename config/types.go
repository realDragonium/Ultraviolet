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

	CacheStatus         bool            `json:"cacheStatus"`
	CacheUpdateCooldown string          `json:"cacheUpdateCooldown"`
	OfflineStatus       mc.SimpleStatus `json:"offlineStatus"`

	RateLimit           int    `json:"rateLimit"`
	RateDuration        string `json:"rateCooldown"`
	StateUpdateCooldown string `json:"stateUpdateCooldown"`
}

type UltravioletConfig struct {
	ListenTo        string          `json:"listenTo"`
	DefaultStatus   mc.SimpleStatus `json:"defaultStatus"`
	NumberOfWorkers int             `json:"numberOfWorkers"`

	LogOutput io.Writer
}
