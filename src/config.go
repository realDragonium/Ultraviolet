package ultravioletv2

import (
	"encoding/json"
	"errors"
	"fmt"
	"math/rand"
	"os"
	"path/filepath"
	"strconv"
	"strings"
)

var (
	MainConfigFileName      = "ultraviolet.json"
	BedrockConfigFileSuffix = "_bedrock.json"
)

type BaseConfig struct {
	ListenTo string `json:"listenTo"`
	ProxyTo  string `json:"proxyTo"`

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

func StringToBedrockStatus(s string) (status BedrockStatus) {
	parts := strings.Split(s, ";")
	
	status.Edition = parts[0] 
	status.Description.Text = parts[1]
	status.Version.Protocol, _ = strconv.Atoi(parts[2])
	status.Version.Name = parts[3]
	status.Players.Online, _ = strconv.Atoi(parts[4])
	status.Players.Max, _ = strconv.Atoi(parts[5])
	// Dont convert ServerGUID 
	status.Description.Text_2 = parts[7]
	status.Gamemode.Name = parts[8]
	status.Gamemode.ID, _ = strconv.Atoi(parts[9])
	status.Port.IPv4, _ = strconv.Atoi(parts[10])
	status.Port.IPv6, _ = strconv.Atoi(parts[11])
	return
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

func ReadBedrockConfigs(path string) (cfgs []BedrockServerConfig, err error) {
	var filePaths []string
	err = filepath.Walk(path, func(path string, info os.FileInfo, err error) error {
		if err != nil {
			return err
		}
		if info.IsDir() {
			return nil
		}
		if !strings.Contains(path, BedrockConfigFileSuffix) {
			return nil
		}
		filePaths = append(filePaths, path)
		return nil
	})

	if err != nil {
		return
	}

	for _, filePath := range filePaths {
		cfg, err := LoadBedrockServerConfig(filePath)
		if err != nil {
			return nil, err
		}
		cfgs = append(cfgs, cfg)
	}

	return cfgs, nil
}

func LoadBedrockServerConfig(path string) (cfg BedrockServerConfig, err error) {
	bb, err := os.ReadFile(path)
	if err != nil {
		return
	}

	cfg = DefaultBedrockServerConfig()
	if err := json.Unmarshal(bb, &cfg); err != nil {
		return cfg, err
	}

	if cfg.BaseConfig.ListenTo == "" || cfg.BaseConfig.ProxyTo == "" {
		return cfg, fmt.Errorf("ListenTo and ProxyTo must be set")
	}

	port := strings.SplitAfter(cfg.ListenTo, ":")[1]
	cfg.ServerStatus.Port.IPv4, err = strconv.Atoi(port)
	if err != nil {
		return cfg, err
	}

	return cfg, nil
}

func DefaultBedrockServerConfig() BedrockServerConfig {
	return BedrockServerConfig{
		ID: rand.Int63(),
		ServerStatus: BedrockStatus{
			Edition: "MCPE",
			Description: Description{
				Text: "Proxied with Ultraviolet",
			},
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
				IPv4: -1,
				IPv6: -1,
			},
		},
	}
}

func ReadJavaConfigs(path string) (cfgs []JavaConfig, err error) {
	var filePaths []string
	err = filepath.Walk(path, func(path string, info os.FileInfo, err error) error {
		if err != nil {
			return err
		}
		if info.IsDir() {
			return nil
		}
		if strings.Contains(path, BedrockConfigFileSuffix) {
			return nil
		}
		if filepath.Ext(path) != ".json" {
			return nil
		}
		if info.Name() == MainConfigFileName {
			return nil
		}
		filePaths = append(filePaths, path)
		return nil
	})

	if err != nil {
		return
	}

	for _, filePath := range filePaths {
		cfg, err := LoadJavaServerConfig(filePath)
		if err != nil {
			return nil, err
		}
		cfgs = append(cfgs, cfg)
	}

	return cfgs, nil
}

func LoadJavaServerConfig(path string) (JavaConfig, error) {
	bb, err := os.ReadFile(path)
	if err != nil {
		return JavaConfig{}, err
	}

	cfg := DefaultJavaConfig()
	if err := json.Unmarshal(bb, &cfg); err != nil {
		return cfg, err
	}

	if cfg.ListenTo == "" || cfg.ProxyTo == "" {
		return cfg, errors.New("ListenTo and ProxyTo must be set")
	}

	return cfg, nil
}

func DefaultJavaConfig() JavaConfig {
	return JavaConfig{}
}
