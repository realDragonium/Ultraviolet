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
	return fmt.Sprintf("%s;%s;%d;%s;%d;%d;%d;%s;%s;%d;%d;%d", s.Edition,
		s.Description.Text, s.Version.Protocol, s.Version.Name, s.Players.Online,
		s.Players.Max, cfg.ID, s.Description.Text_2, s.Gamemode.Name, s.Gamemode.ID,
		s.Port.IPv4, s.Port.IPv6)
}

var bedrockStatus = BedrockStatus{
	Edition:     "MCPE",
	Description: Description{Text: "This Server - UV"},
	Version: Version{
		Name:     "1.19.10",
		Protocol: 534,
	},
	Players: Players{
		Online: 0,
		Max:    100,
	},
	Gamemode: GameMode{
		Name: "Survival",
		ID:   1,
	},
	Port: Port{
		IPv4: 19132,
		IPv6: -1,
	},
}

type BedrockStatus struct {
	Edition     string      `json:"Edition"`
	Description Description `json:"Description"`
	Version     Version     `json:"version"`
	Players     Players     `json:"players"`
	Gamemode    GameMode    `json:"gamemode"`
	Port        Port        `json:"port"`
}

type GameMode struct {
	Name string `json:"name"`
	ID   int    `json:"id"`
}

type Port struct {
	IPv4 int `json:"ipv4"`
	IPv6 int `json:"ipv6"`
}

type JavaStatus struct {
	Version     Version     `json:"version"`
	Players     Players     `json:"players"`
	Description Description `json:"description"`
	Favicon     string      `json:"favicon"`
}

type Version struct {
	Name     string `json:"name"`
	Protocol int    `json:"protocol"`
}

type Players struct {
	Max    int `json:"max"`
	Online int `json:"online"`
}

type Description struct {
	Text   string `json:"text"`
	Text_2 string `json:"text_2"`
}
