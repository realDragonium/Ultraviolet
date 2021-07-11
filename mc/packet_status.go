package mc

import (
	"encoding/json"
	"time"
)

const (
	ClientBoundResponsePacketID byte = 0x00
	ServerBoundRequestPacketID  byte = 0x00
	ServerBoundPingPacketID     byte = 0x01
	ClientBoundPongPacketID     byte = 0x01
)

type SimpleStatus struct {
	Name        string `json:"name"`
	Protocol    int    `json:"protocol"`
	Description string `json:"text"`
	Favicon     string `json:"favicon,omitempty"`
}

func (pk SimpleStatus) Marshal() Packet {
	jsonResponse := ResponseJSON{
		Version: VersionJSON{
			Name:     pk.Name,
			Protocol: pk.Protocol,
		},
		Description: DescriptionJSON{
			Text: pk.Description,
		},
		Favicon: pk.Favicon,
	}
	text, _ := json.Marshal(jsonResponse)
	return ClientBoundResponse{
		JSONResponse: String(text),
	}.Marshal()
}

type DifferentStatusResponse struct {
	Version     VersionJSON     `json:"version"`
	Description DescriptionJSON `json:"description"`
	Favicon     string          `json:"favicon,omitempty"`
}

func (pk DifferentStatusResponse) Marshal() Packet {
	jsonResponse := ResponseJSON{
		Version:     pk.Version,
		Description: pk.Description,
		Favicon:     pk.Favicon,
	}
	text, _ := json.Marshal(jsonResponse)
	return ClientBoundResponse{
		JSONResponse: String(text),
	}.Marshal()
}

type ClientBoundResponse struct {
	JSONResponse String
}

func (pk ClientBoundResponse) Marshal() Packet {
	return MarshalPacket(
		ClientBoundResponsePacketID,
		pk.JSONResponse,
	)
}

func UnmarshalClientBoundResponse(packet Packet) (ClientBoundResponse, error) {
	var pk ClientBoundResponse

	if packet.ID != ClientBoundResponsePacketID {
		return pk, ErrInvalidPacketID
	}

	if err := packet.Scan(
		&pk.JSONResponse,
	); err != nil {
		return pk, err
	}

	return pk, nil
}

type ResponseJSON struct {
	Version     VersionJSON     `json:"version"`
	Players     PlayersJSON     `json:"players"`
	Description DescriptionJSON `json:"description"`
	Favicon     string          `json:"favicon"`
}

type VersionJSON struct {
	Name     string `json:"name"`
	Protocol int    `json:"protocol"`
}

type PlayersJSON struct {
	Max    int                `json:"max"`
	Online int                `json:"online"`
	Sample []PlayerSampleJSON `json:"sample"`
}

type PlayerSampleJSON struct {
	Name string `json:"name"`
	ID   string `json:"id"`
}

type DescriptionJSON struct {
	Text string `json:"text"`
}

type ServerBoundRequest struct{}

func (pk ServerBoundRequest) Marshal() Packet {
	return MarshalPacket(
		ServerBoundRequestPacketID,
	)
}

func NewServerBoundPing() ServerBoundPing {
	millisecondTime :=  time.Now().UnixNano() / 1e6
	return ServerBoundPing{
		Time: Long(millisecondTime),
	}
}

type ServerBoundPing struct {
	Time Long
}

func (pk ServerBoundPing) Marshal() Packet {
	return MarshalPacket(
		ClientBoundResponsePacketID,
		pk.Time,
	)
}
