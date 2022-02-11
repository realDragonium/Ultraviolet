package ultraviolet

import (
	"strings"

	"github.com/realDragonium/Ultraviolet/core"
	"github.com/realDragonium/Ultraviolet/mc"
)

type ServerCatalog interface {
	Find(addr string) (core.Server, error)
	DefaultStatus() mc.Packet
	VerifyConn() mc.Packet
}

func NewBasicServerCatalog(defaultStatusPk, verifyConnPk mc.Packet) BasicServerCatalog {
	return BasicServerCatalog{
		ServerDict:      make(map[string]core.Server),
		defaultStatusPk: defaultStatusPk,
		verifyConnPk:    verifyConnPk,
	}
}

type BasicServerCatalog struct {
	ServerDict      map[string]core.Server
	defaultStatusPk mc.Packet
	verifyConnPk    mc.Packet
}

func (catalog BasicServerCatalog) Find(addr string) (core.Server, error) {
	cleanAddr := strings.ToLower(addr)
	server, ok := catalog.ServerDict[cleanAddr]
	if !ok {
		return nil, core.ErrNoServerFound
	}
	return server, nil
}

func (catalog BasicServerCatalog) DefaultStatus() mc.Packet {
	return catalog.defaultStatusPk
}

func (catalog BasicServerCatalog) VerifyConn() mc.Packet {
	return catalog.verifyConnPk
}
