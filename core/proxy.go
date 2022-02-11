package core

import (
	"net"

	"github.com/realDragonium/Ultraviolet/config"
)

type NewProxyFunc func(config.UVConfigReader, net.Listener, config.ServerConfigReader) Proxy

type Proxy interface {
	Start() error
}
