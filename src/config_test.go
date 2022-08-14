package ultravioletv2_test

import (
	"testing"

	ultravioletv2 "github.com/realDragonium/Ultraviolet/src"
)

func TestBedrockStatus(t *testing.T) {
	bedrock_status := "MCPE;This Server - UV;534;1.19.10;0;100;92837498723498;MOTD_2;Survival;1;19132;-1"

	cfg := ultravioletv2.BedrockServerConfig{
		ID: 92837498723498,
		ServerStatus: ultravioletv2.BedrockStatus{
			Edition:     "MCPE",
			Description: ultravioletv2.Description{
				Text: "This Server - UV",
				Text_2: "MOTD_2",
			},
			Version: ultravioletv2.Version{
				Name:     "1.19.10",
				Protocol: 534,
			},
			Players: ultravioletv2.Players{
				Online: 0,
				Max:    100,
			},
			Gamemode: ultravioletv2.GameMode{
				Name: "Survival",
				ID:   1,
			},
			Port: ultravioletv2.Port{
				IPv4: 19132,
				IPv6: -1,
			},
		},
	}

	convertedStatus := ultravioletv2.StringToBedrockStatus(bedrock_status)
	if cfg.ServerStatus != convertedStatus {
		t.Errorf("Expected %#v, got %#v", cfg.ServerStatus, convertedStatus)
	}

	if cfg.Status() != bedrock_status {
		t.Errorf("Expected %#v, got %#v", bedrock_status, cfg.Status())
	}

}
