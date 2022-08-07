package ultravioletv2

import "fmt"

type BaseConfig struct {
	ListenTo string `json:"proxyTo"`
	ProxyTo  string `json:"proxyBind"`

	// TODO for later:
	//  - Proxy Protocol options
}

type JavaConfig struct {
	BaseConfig

	Domains []string `json:"domains"`
}

type BedrockServerConfig struct {
	BaseConfig

	ID           int64         `json:"id"`
	ServerStatus BedrockStatus `json:"status"`
}

func (cfg BedrockServerConfig) Status() string {
	s := cfg.ServerStatus
	return fmt.Sprintf("%s;%s;%d;%s;%d;%d;%d;%s;%s;%d;%d;%d", s.Edition, s.Description_1, s.Version.Protocol, s.Version.Name, s.Players.Online, s.Players.Max, cfg.ID, s.Description_2, s.Gamemode.Name, s.Gamemode.ID, s.Port.IPv4, s.Port.IPv6)
}

var bedrockStatus = BedrockStatus{
	Edition:       "MCPE",
	Description_1: "This Server - UV",
	Version: VersionJSON{
		Name:     "1.19.10",
		Protocol: 534,
	},
	Players: PlayersJSON{
		Online: 0,
		Max:    100,
	},
	ServerGUID: "3394339436721259498",
	Gamemode: GameMode{
		Name: "Survival",
		ID:   1,
	},
	Port: PortJSON{
		IPv4: 19132,
		IPv6: -1,
	},
}

type BedrockStatus struct {
	Edition       string      `json:"Edition"`
	Description_1 string      `json:"description"`
	Version       VersionJSON `json:"version"`
	Players       PlayersJSON `json:"players"`
	ServerGUID    string      `json:"serverGUID"`
	Description_2 string      `json:"description_2"`
	Gamemode      GameMode    `json:"gamemode"`
	Port          PortJSON    `json:"port"`
}

func (s BedrockStatus) ToString() string {
	// MCPE;This Server - UV;534;1.19.10;6;100;3394339436721259498;Ultraviolet;Survival;1;19132;-1;
	return fmt.Sprintf("%s;%s;%d;%s;%d;%d;%s;%s;%s;%d;%d;%d", s.Edition, s.Description_1, s.Version.Protocol, s.Version.Name, s.Players.Online, s.Players.Max, s.ServerGUID, s.Description_2, s.Gamemode.Name, s.Gamemode.ID, s.Port.IPv4, s.Port.IPv6)
}

type GameMode struct {
	Name string `json:"name"`
	ID   int    `json:"id"`
}

type PortJSON struct {
	IPv4 int `json:"ipv4"`
	IPv6 int `json:"ipv6"`
}

type ServerStatus struct {
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
	Max    int `json:"max"`
	Online int `json:"online"`
}

type DescriptionJSON struct {
	Text string `json:"text"`
}
