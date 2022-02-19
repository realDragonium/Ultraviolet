package core

import (
	"strings"

	"github.com/realDragonium/Ultraviolet/mc"
)

type ServerCatalog interface {
	Find(addr string) (Server, error)
	DefaultStatus() mc.Packet
	VerifyConn() mc.Packet
}

func NewServerCatalog(servers map[string]Server, defaultStatusPk, verifyConnPk mc.Packet) ServerCatalog {
	return BasicServerCatalog{
		ServerDict:      servers,
		defaultStatusPk: defaultStatusPk,
		verifyConnPk:    verifyConnPk,
	}
}

func NewEmptyServerCatalog(defaultStatusPk, verifyConnPk mc.Packet) BasicServerCatalog {
	return BasicServerCatalog{
		ServerDict:      make(map[string]Server),
		defaultStatusPk: defaultStatusPk,
		verifyConnPk:    verifyConnPk,
	}
}

type BasicServerCatalog struct {
	ServerDict      map[string]Server
	defaultStatusPk mc.Packet
	verifyConnPk    mc.Packet
}

func (catalog BasicServerCatalog) Find(addr string) (Server, error) {
	cleanAddr := strings.ToLower(addr)
	server, ok := catalog.ServerDict[cleanAddr]
	if !ok {
		return nil, ErrNoServerFound
	}
	return server, nil
}

func (catalog BasicServerCatalog) DefaultStatus() mc.Packet {
	return catalog.defaultStatusPk
}

func (catalog BasicServerCatalog) VerifyConn() mc.Packet {
	return catalog.verifyConnPk
}
