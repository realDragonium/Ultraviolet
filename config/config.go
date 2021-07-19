package config

import (
	"crypto/ecdsa"
	"time"

	"github.com/realDragonium/Ultraviolet/mc"
)

type ServerState byte

const (
	UNKNOWN ServerState = iota
	ONLINE
	OFFLINE
	UPDATE
)

func (state ServerState) String() string {
	var text string
	switch state {
	case UNKNOWN:
		text = "Unknown"
	case ONLINE:
		text = "Online"
	case OFFLINE:
		text = "Offline"
	case UPDATE:
		text = "Update"
	}
	return text
}

type ServerConfigFile struct {
}

func (file ServerConfigFile) ReadConfig() {

}

type RateLimitConfig struct {
	rateCounter     int
	rateStartTime   time.Time
	rateLimit       int
	rateLimitStatus bool
	rateCooldown    time.Duration
}

type StateConfig struct {
	state         ServerState
	stateCooldown time.Duration
	stateUpdateCh chan ServerState
}

type OldRealIPConfig struct {
}

type NewRealIPConfig struct {
	keyPath   string
	realIPKey *ecdsa.PrivateKey
}

type OfflineStatusConfig struct {
	offlineStatus mc.Packet
}

type CacheStatusConfig struct {
	cachedStatus    mc.Packet
	statusCooldown  time.Duration
	statusCacheTime time.Time
	statusLatency   time.Duration
	statusHandshake mc.Packet
}
