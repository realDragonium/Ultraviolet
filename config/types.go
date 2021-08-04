package config

import (
	"crypto/ecdsa"
	"time"

	"github.com/realDragonium/Ultraviolet/mc"
)

type ServerConfig struct {
	FilePath string
	Domains  []string `json:"domains"`

	ProxyTo           string `json:"proxyTo"`
	ProxyBind         string `json:"proxyBind"`
	DialTimeout       string `json:"dialTimeout"`
	OldRealIP         bool   `json:"useRealIPv2.4"`
	NewRealIP         bool   `json:"useRealIPv2.5"`
	RealIPKey         string `json:"realIPKeyPath"`
	SendProxyProtocol bool   `json:"sendProxyProtocol"`

	DisconnectMessage string `json:"disconnectMessage"`

	CacheStatus         bool            `json:"cacheStatus"`
	CacheUpdateCooldown string          `json:"cacheUpdateCooldown"`
	ValidProtocol       int             `json:"validProtocol"`
	OfflineStatus       mc.SimpleStatus `json:"offlineStatus"`

	RateLimit           int    `json:"rateLimit"`
	RateLimitStatus     bool   `json:"rateLimitStatus"`
	RateDuration        string `json:"rateCooldown"`
	StateUpdateCooldown string `json:"stateUpdateCooldown"`
}

type UltravioletConfig struct {
	ListenTo          string          `json:"listenTo"`
	DefaultStatus     mc.SimpleStatus `json:"defaultStatus"`
	NumberOfWorkers   int             `json:"numberOfWorkers"`
	NumberOfListeners int             `json:"numberOfListeners"`
	UseProxyProtocol  bool            `json:"acceptProxyProtocol"`
	PidFile           string
}

func DefaultUltravioletConfig() UltravioletConfig {
	return UltravioletConfig{
		ListenTo: ":25565",
		DefaultStatus: mc.SimpleStatus{
			Name:        "Ultraviolet",
			Protocol:    755,
			Description: "Some broken proxy",
		},
		NumberOfWorkers:   25,
		NumberOfListeners: 5,
		PidFile:           "/var/run/ultraviolet.pid",
	}
}

type WorkerServerConfig struct {
	StateUpdateCooldown time.Duration
	OldRealIp           bool
	NewRealIP           bool
	RealIPKey           *ecdsa.PrivateKey
	CacheStatus         bool
	CacheUpdateCooldown time.Duration
	ValidProtocol       int
	OfflineStatus       mc.Packet
	DisconnectPacket    mc.Packet
	ProxyTo             string
	ProxyBind           string
	DialTimeout         time.Duration
	SendProxyProtocol   bool
	RateLimit           int
	RateLimitStatus     bool
	RateLimitDuration   time.Duration
}

type WorkerConfig struct {
	DefaultStatus mc.SimpleStatus
}

func NewWorkerConfig(uvCfg UltravioletConfig) WorkerConfig {
	return WorkerConfig{
		DefaultStatus: uvCfg.DefaultStatus,
	}
}
