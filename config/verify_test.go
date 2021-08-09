package config_test

import (
	"testing"

	"github.com/realDragonium/Ultraviolet/config"
)

func TestVerifyConfigs(t *testing.T) {
	t.Run("can detect duplicate domains", func(t *testing.T) {
		domain := "uv"
		cfgs := []config.ServerConfig{
			{
				FilePath: "uv1",
				Domains:  []string{domain},
				ProxyTo:  "1",
			},
			{
				FilePath: "uv2",
				Domains:  []string{domain},
				ProxyTo:  "1",
			},
		}

		errs := config.VerifyConfigs(cfgs)
		if len(errs) != 1 {
			t.Log(errs)
			t.Fatalf("expected 1 errors but got %d", len(errs))
		}
		_, ok := errs[0].(*config.DuplicateDomain)
		if !ok {
			t.Errorf("expected DuplicateDomain but got %T", errs[0])
		}
	})

	t.Run("can detect multiple duplicate domains", func(t *testing.T) {
		domain := "uv"
		cfgs := []config.ServerConfig{
			{
				FilePath: "uv1",
				Domains:  []string{domain},
				ProxyTo:  "1",
			},
			{
				FilePath: "uv2",
				Domains:  []string{domain},
				ProxyTo:  "1",
			},
			{
				FilePath: "uv3",
				Domains:  []string{domain},
				ProxyTo:  "1",
			},
		}

		errs := config.VerifyConfigs(cfgs)
		if len(errs) != 2 {
			t.Fatalf("expected 2 errors but got %d", len(errs))
		}
	})
}
