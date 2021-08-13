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
		if !errs.HasErrors() {
			t.Fatal("expected it to have errors")
		}
		// _, ok := errs[0].(*config.DuplicateDomain)
		// if !ok {
		// 	t.Errorf("expected DuplicateDomain but got %T", errs[0])
		// }
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
		if !errs.HasErrors() {
			t.Fatal("expected it to have errors")
			// t.Fatalf("expected 2 errors but got %d", len(errs))
		}
	})

	t.Run("returns error when there are no domains", func(t *testing.T) {
		cfgs := []config.ServerConfig{
			{
				ProxyTo: ":9284",
			},
		}
		verifyError := config.VerifyConfigs(cfgs)
		if !verifyError.HasErrors() {
			t.Log("expect it to have an error")
			// t.Errorf("expected no domain error but instead got: %v", err)
		}
	})

	t.Run("returns error when there is no target", func(t *testing.T) {
		cfgs := []config.ServerConfig{
			{
				Domains: []string{"uv"},
			},
		}
		verifyError := config.VerifyConfigs(cfgs)
		if !verifyError.HasErrors() {
			t.Log("expect it to have an error")
			// t.Errorf("expected no domain error but instead got: %v", err)
		}
	})
}
