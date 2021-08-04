package ultraviolet

import (
	"crypto/ecdsa"

	"github.com/realDragonium/Ultraviolet/mc"
)

type HandshakeModifier interface {
	Modify(hs *mc.ServerBoundHandshake, addr string)
}

func NewRealIP2_4() realIPv2_4 {
	return realIPv2_4{}
}

type realIPv2_4 struct{}

func (rip realIPv2_4) Modify(hs *mc.ServerBoundHandshake, addr string) {
	hs.UpgradeToOldRealIP(addr)
}

func NewRealIP2_5(key *ecdsa.PrivateKey) realIPv2_5 {
	return realIPv2_5{
		realIPKey: key,
	}
}

type realIPv2_5 struct {
	realIPKey *ecdsa.PrivateKey
}

func (rip realIPv2_5) Modify(hs *mc.ServerBoundHandshake, addr string) {
	hs.UpgradeToNewRealIP(addr, rip.realIPKey)
}
