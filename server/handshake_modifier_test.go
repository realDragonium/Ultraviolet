package server_test

import (
	"crypto/ecdsa"
	"crypto/elliptic"
	"crypto/rand"
	"strings"
	"testing"

	"github.com/realDragonium/Ultraviolet/mc"
	"github.com/realDragonium/Ultraviolet/server"
)

func TestRateLimter_RealIPv2_4(t *testing.T) {
	handshakeModifier := server.NewRealIP2_4()
	handshake := mc.ServerBoundHandshake{
		ProtocolVersion: 755,
		ServerAddress:   "ultraviolet",
		ServerPort:      25565,
		NextState:       2,
	}

	handshakeModifier.Modify(&handshake, "192.32.27.85:57493")
	if !handshake.IsRealIPAddress() {
		t.Fatal("Should have changed to RealIP format")
	}
	parts := strings.Split(handshake.ServerAddress, mc.RealIPSeparator)
	if len(parts) != 3 {
		t.Errorf("Wrong RealIP format?!? %v", parts)
	}
}

func TestRateLimter_RealIPv2_5(t *testing.T) {
	privKey, err := ecdsa.GenerateKey(elliptic.P256(), rand.Reader)
	if err != nil {
		t.Fatalf("got error: %v", err)
	}
	handshakeModifier := server.NewRealIP2_5(privKey)
	handshake := mc.ServerBoundHandshake{
		ProtocolVersion: 755,
		ServerAddress:   "ultraviolet",
		ServerPort:      25565,
		NextState:       2,
	}

	handshakeModifier.Modify(&handshake, "192.32.27.85:57493")
	if !handshake.IsRealIPAddress() {
		t.Fatal("Should have changed to RealIP format")
	}
	parts := strings.Split(handshake.ServerAddress, mc.RealIPSeparator)
	if len(parts) != 4 {
		t.Errorf("wrong RealIP format? %v", parts)
	}
}
