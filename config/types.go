package config

import (
	"crypto/ecdsa"
	"time"

	"github.com/realDragonium/Ultraviolet/mc"
)

type ServerConfig struct {
	FilePath string
	Name     string   `json:"name"`
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

	// RateLimitStatus     bool   `json:"rateLimitStatus"`
	RateLimit           int    `json:"rateLimit"`
	RateDuration        string `json:"rateCooldown"`
	RateBanListCooldown string `json:"banListCooldown"`
	RateDisconMsg       string `json:"reconnectMsg"`

	StateUpdateCooldown string `json:"stateUpdateCooldown"`
}

func DefaultServerConfig() ServerConfig {
	return ServerConfig{
		ProxyBind:           "",
		DialTimeout:         "1s",
		OldRealIP:           false,
		NewRealIP:           false,
		SendProxyProtocol:   false,
		DisconnectMessage:   "Server  is offline",
		CacheStatus:         false,
		RateLimit:           5,
		RateDuration:        "1s",
		RateBanListCooldown: "5m",
		RateDisconMsg:       "Please reconnect to verify yourself",
		StateUpdateCooldown: "1s",
	}
}

type UltravioletConfig struct {
	ListenTo            string          `json:"listenTo"`
	DefaultStatus       mc.SimpleStatus `json:"defaultStatus"`
	NumberOfWorkers     int             `json:"numberOfWorkers"`
	NumberOfListeners   int             `json:"numberOfListeners"`
	AcceptProxyProtocol bool            `json:"acceptProxyProtocol"`
	UsePrometheus       bool            `json:"enablePrometheus"`
	PrometheusBind      string          `json:"prometheusBind"`

	EnableHotSwap bool
	PidFile       string
	IODeadline    time.Duration
}

func DefaultUltravioletConfig() UltravioletConfig {
	return UltravioletConfig{
		ListenTo: ":25565",
		DefaultStatus: mc.SimpleStatus{
			Name:        "Ultraviolet",
			Protocol:    755,
			Description: "Some broken proxy",
		},
		NumberOfWorkers:     10,
		NumberOfListeners:   1,
		AcceptProxyProtocol: false,
		UsePrometheus:       true,
		PrometheusBind:      ":9100",

		PidFile:       "/var/run/ultraviolet.pid",
		EnableHotSwap: true,
		IODeadline:    time.Second,
	}
}

type WorkerServerConfig struct {
	Name                string
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
	RateBanListCooldown time.Duration
	RateDisconPk        mc.Packet
}

func DefaultWorkerConfig() WorkerConfig {
	return WorkerConfig{
		IOTimeout: time.Second,
	}
}

type WorkerConfig struct {
	DefaultStatus mc.SimpleStatus
	IOTimeout     time.Duration
}

func NewWorkerConfig(uvCfg UltravioletConfig) WorkerConfig {
	if uvCfg.IODeadline == 0 {
		uvCfg.IODeadline = time.Second
	}
	return WorkerConfig{
		DefaultStatus: uvCfg.DefaultStatus,
		IOTimeout:     uvCfg.IODeadline,
	}
}
