package mc

import (
	"fmt"
	"net"
	"strings"
	"time"
)

type ServerBoundHandshake struct {
	ProtocolVersion VarInt
	ServerAddress   String
	ServerPort      UnsignedShort
	NextState       VarInt
}

func (pk ServerBoundHandshake) Marshal() Packet {
	return MarshalPacket(
		ServerBoundHandshakePacketID,
		pk.ProtocolVersion,
		pk.ServerAddress,
		pk.ServerPort,
		pk.NextState,
	)
}

func UnmarshalServerBoundHandshake(packet Packet) (ServerBoundHandshake, error) {
	var pk ServerBoundHandshake

	if packet.ID != ServerBoundHandshakePacketID {
		return pk, ErrInvalidPacketID
	}

	if err := packet.Scan(
		&pk.ProtocolVersion,
		&pk.ServerAddress,
		&pk.ServerPort,
		&pk.NextState,
	); err != nil {
		return pk, err
	}

	return pk, nil
}

func (pk ServerBoundHandshake) IsStatusRequest() bool {
	return pk.NextState == HandshakeStatusState
}

func (pk ServerBoundHandshake) IsLoginRequest() bool {
	return pk.NextState == HandshakeLoginState
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
	addr = strings.Trim(addr, ".")
	return addr
}

func (pk *ServerBoundHandshake) UpgradeToRealIP(clientAddr net.Addr, timestamp time.Time) {
	if pk.IsRealIPAddress() {
		return
	}

	addr := string(pk.ServerAddress)
	addrWithForge := strings.SplitN(addr, ForgeSeparator, 3)

	addr = fmt.Sprintf("%s///%s///%d", addrWithForge[0], clientAddr.String(), timestamp.Unix())

	if len(addrWithForge) > 1 {
		addr = fmt.Sprintf("%s\x00%s\x00", addr, addrWithForge[1])
	}

	pk.ServerAddress = String(addr)
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
