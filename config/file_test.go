package config_test

import (
	"encoding/json"
	"io/ioutil"
	"os"
	"reflect"
	"testing"

	"github.com/realDragonium/Ultraviolet/config"
	"github.com/realDragonium/Ultraviolet/mc"
)

func TestReadServerConfigFile(t *testing.T) {
	cfg := config.ServerConfig{
		MainDomain: "infrared",
		ProxyTo:    ":25566",
	}
	file, _ := json.MarshalIndent(cfg, "", " ")
	tmpfile, err := ioutil.TempFile("", "example")
	if err != nil {
		t.Fatal(err)
	}
	defer os.Remove(tmpfile.Name())
	if _, err := tmpfile.Write(file); err != nil {
		t.Fatal(err)
	}
	if err := tmpfile.Close(); err != nil {
		t.Fatal(err)
	}
	loadedCfg, err := config.LoadServerCfgFromPath(tmpfile.Name())
	if err != nil {
		t.Error(err)
	}

	if !reflect.DeepEqual(cfg, loadedCfg) {
		t.Errorf("Wanted:%v \n got: %v", cfg, loadedCfg)
	}
}

func TestReadServerConfigs(t *testing.T) {
	cfg := config.ServerConfig{
		MainDomain: "infrared",
		ProxyTo:    ":25566",
	}
	tmpDir, _ := ioutil.TempDir("", "configs")
	for i := 0; i < 3; i++ {
		file, _ := json.MarshalIndent(cfg, "", " ")
		tmpfile, err := ioutil.TempFile(tmpDir, "example")
		if err != nil {
			t.Fatal(err)
		}
		defer os.Remove(tmpfile.Name())
		if _, err := tmpfile.Write(file); err != nil {
			t.Fatal(err)
		}
		if err := tmpfile.Close(); err != nil {
			t.Fatal(err)
		}
	}
	loadedCfgs, _ := config.ReadServerConfigs(tmpDir)
	for i, loadedCfg := range loadedCfgs {
		if !reflect.DeepEqual(cfg, loadedCfg) {
			t.Errorf("index: %d \nWanted:%v \n got: %v", i, cfg, loadedCfg)
		}
	}
}

func TestReadUltravioletConfigFile(t *testing.T) {
	cfg := config.UltravioletConfig{
		ListenTo:             ":25565",
		ReceiveProxyProtocol: false,
		DefaultStatus: mc.AnotherStatusResponse{
			Name:        "Ultraviolet",
			Protocol:    755,
			Description: "One dangerous proxy",
		},

		NumberOfWorkers:       5,
		NumberOfConnWorkers:   1,
		NumberOfStatusWorkers: 3,
	}
	file, _ := json.MarshalIndent(cfg, "", " ")
	tmpfile, err := ioutil.TempFile("", "example")
	if err != nil {
		t.Fatal(err)
	}
	defer os.Remove(tmpfile.Name())
	if _, err := tmpfile.Write(file); err != nil {
		t.Fatal(err)
	}
	if err := tmpfile.Close(); err != nil {
		t.Fatal(err)
	}
	loadedCfg, err := config.ReadUltravioletConfig(tmpfile.Name())
	if err != nil {
		t.Error(err)
	}

	if !reflect.DeepEqual(cfg, loadedCfg) {
		t.Errorf("Wanted:%v \n got: %v", cfg, loadedCfg)
	}
}
