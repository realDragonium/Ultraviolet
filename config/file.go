package config

import (
	"encoding/json"
	"io/ioutil"
	"os"
	"path/filepath"
)

func ReadServerConfigs(path string) ([]ServerConfig, error) {
	var cfgs []ServerConfig
	var filePaths []string
	err := filepath.Walk(path, func(path string, info os.FileInfo, err error) error {
		if err != nil {
			return err
		}
		if info.IsDir() {
			return nil
		}
		filePaths = append(filePaths, path)
		return nil
	})
	if err != nil {
		return cfgs, err
	}
	for _, filePath := range filePaths {
		cfg, err := LoadServerCfgFromPath(filePath)
		if err != nil {
			return nil, err
		}
		if err != nil {
			return nil, err
		}
		cfgs = append(cfgs, cfg)
	}

	return cfgs, nil
}

func LoadServerCfgFromPath(path string) (ServerConfig, error) {
	bb, err := ioutil.ReadFile(path)
	if err != nil {
		return ServerConfig{}, err
	}
	cfg := ServerConfig{}
	if err := json.Unmarshal(bb, &cfg); err != nil {
		return cfg, err
	}
	return cfg, nil
}

func ReadUltravioletConfig(path string) (UltravioletConfig, error) {
	var cfg UltravioletConfig

	// TODO: Check or file exists and if not write default config file to it
	bb, err := ioutil.ReadFile(path)
	if err != nil {
		return UltravioletConfig{}, err
	}
	if err := json.Unmarshal(bb, &cfg); err != nil {
		return cfg, err
	}
	return cfg, nil
}
