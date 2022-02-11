package module

import (
	"errors"
	"time"

	"github.com/realDragonium/Ultraviolet/core"
	"github.com/realDragonium/Ultraviolet/mc"
)

type StatusCache interface {
	Status() (mc.Packet, error)
}

func NewStatusCache(protocol int, cooldown time.Duration, connCreator ConnectionCreator) StatusCache {
	handshake := mc.ServerBoundHandshake{
		ProtocolVersion: protocol,
		ServerAddress:   "Ultraviolet",
		ServerPort:      25565,
		NextState:       1,
	}

	return &statusCache{
		connCreator: connCreator,
		cooldown:    cooldown,
		handshake:   handshake,
	}
}

type statusCache struct {
	connCreator ConnectionCreator

	status    mc.Packet
	cooldown  time.Duration
	cacheTime time.Time
	handshake mc.ServerBoundHandshake
}

func (cache *statusCache) Status() (mc.Packet, error) {
	if time.Since(cache.cacheTime) < cache.cooldown {
		return cache.status, nil
	}
	answer, err := cache.newStatus()
	if err != nil && !errors.Is(err, core.ErrStatusPing) {
		return cache.status, err
	}
	cache.cacheTime = time.Now()
	cache.status = answer
	return cache.status, nil
}

func (cache *statusCache) newStatus() (pk mc.Packet, err error) {
	connFunc := cache.connCreator.Conn()
	conn, err := connFunc()
	if err != nil {
		return
	}

	mcConn := mc.NewMcConn(conn)
	if err = mcConn.WriteMcPacket(cache.handshake); err != nil {
		return
	}
	if err = mcConn.WritePacket(mc.ServerBoundRequest{}.Marshal()); err != nil {
		return
	}

	pk, err = mcConn.ReadPacket()
	if err != nil {
		return
	}

	conn.Close()
	return
}
