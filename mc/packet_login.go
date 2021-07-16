package mc

import (
	"crypto/ecdsa"
	"crypto/rand"
	"crypto/sha512"
	"encoding/base64"
	"fmt"
	"strings"
	"time"
)

type McTypesHandshake struct {
	ProtocolVersion VarInt
	ServerAddress   String
	ServerPort      UnsignedShort
	NextState       VarInt
}

type ServerBoundHandshake struct {
	ProtocolVersion int
	ServerAddress   string
	ServerPort      int16
	NextState       int
}

func (pk ServerBoundHandshake) Marshal() Packet {
	return MarshalPacket(
		ServerBoundHandshakePacketID,
		VarInt(pk.ProtocolVersion),
		String(pk.ServerAddress),
		UnsignedShort(pk.ServerPort),
		VarInt(pk.NextState),
	)
}

func UnmarshalServerBoundHandshake(packet Packet) (ServerBoundHandshake, error) {
	var pk McTypesHandshake
	var hs ServerBoundHandshake

	if packet.ID != ServerBoundHandshakePacketID {
		return hs, ErrInvalidPacketID
	}

	if err := packet.Scan(
		&pk.ProtocolVersion,
		&pk.ServerAddress,
		&pk.ServerPort,
		&pk.NextState,
	); err != nil {
		return hs, err
	}
	hs = ServerBoundHandshake{
		ProtocolVersion: int(pk.ProtocolVersion),
		ServerAddress:   string(pk.ServerAddress),
		ServerPort:      int16(pk.ServerPort),
		NextState:       int(pk.NextState),
	}
	return hs, nil
}

func (pk ServerBoundHandshake) IsStatusRequest() bool {
	return VarInt(pk.NextState) == HandshakeStatusState
}

func (pk ServerBoundHandshake) IsLoginRequest() bool {
	return VarInt(pk.NextState) == HandshakeLoginState
}

func (pk ServerBoundHandshake) IsForgeAddress() bool {
	addr := string(pk.ServerAddress)
	return len(strings.Split(addr, ForgeSeparator)) > 1
}

func (pk ServerBoundHandshake) IsRealIPAddress() bool {
	addr := string(pk.ServerAddress)
	return len(strings.Split(addr, RealIPSeparator)) > 1
}

func (pk ServerBoundHandshake) ParseServerAddress() string {
	addr := string(pk.ServerAddress)
	addr = strings.Split(addr, ForgeSeparator)[0]
	addr = strings.Split(addr, RealIPSeparator)[0]
	return addr
}

func (hs *ServerBoundHandshake) UpgradeToOldRealIP(clientAddr string) {
	hs.UpgradeToOldRealIP_WithTime(clientAddr, time.Now())
}

func (hs *ServerBoundHandshake) UpgradeToOldRealIP_WithTime(clientAddr string, stamp time.Time) {
	if hs.IsRealIPAddress() {
		return
	}

	addr := string(hs.ServerAddress)
	addrWithForge := strings.SplitN(addr, ForgeSeparator, 3)

	addr = fmt.Sprintf("%s///%s///%d", addrWithForge[0], clientAddr, stamp.Unix())

	if len(addrWithForge) > 1 {
		addr = fmt.Sprintf("%s\x00%s\x00", addr, addrWithForge[1])
	}

	hs.ServerAddress = addr
}

func (hs *ServerBoundHandshake) UpgradeToNewRealIP(clientAddr string, key *ecdsa.PrivateKey) error {
	hs.UpgradeToOldRealIP(clientAddr)
	text := hs.ServerAddress
	hash := sha512.Sum512([]byte(text))
	bytes, err := ecdsa.SignASN1(rand.Reader, key, hash[:])
	if err != nil {
		return err
	}
	encoded := base64.StdEncoding.EncodeToString(bytes)
	addr := fmt.Sprintf("%s///%s", hs.ServerAddress, encoded)
	hs.ServerAddress = addr
	return nil
}

const ServerBoundLoginStartPacketID byte = 0x00

type ServerLoginStart struct {
	Name String
}

func (pk ServerLoginStart) Marshal() Packet {
	return MarshalPacket(ServerBoundLoginStartPacketID, pk.Name)
}

func UnmarshalServerBoundLoginStart(packet Packet) (ServerLoginStart, error) {
	var pk ServerLoginStart

	if packet.ID != ServerBoundLoginStartPacketID {
		return pk, ErrInvalidPacketID
	}

	if err := packet.Scan(&pk.Name); err != nil {
		return pk, err
	}

	return pk, nil
}

const ClientBoundDisconnectPacketID byte = 0x00

type ClientBoundDisconnect struct {
	Reason Chat
}

func (pk ClientBoundDisconnect) Marshal() Packet {
	return MarshalPacket(
		ClientBoundDisconnectPacketID,
		pk.Reason,
	)
}

func UnmarshalClientDisconnect(packet Packet) (ClientBoundDisconnect, error) {
	var pk ClientBoundDisconnect

	if packet.ID != ClientBoundDisconnectPacketID {
		return pk, ErrInvalidPacketID
	}

	err := packet.Scan(&pk.Reason)
	return pk, err
}
