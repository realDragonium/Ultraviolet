package config

import (
	"encoding/json"
	"io/ioutil"
	"log"
	"os"
	"path/filepath"
)

var (
	// Isnt this the proper path to put config files into (for execution without docker)
	defaultCfgPath            = "/etc/ultraviolet"
	defaultServerCfgPath      = filepath.Join(defaultCfgPath, "config")
	defaultUltravioletCfgPath = filepath.Join(defaultCfgPath, "ultraviolet.json")
)

func ReadServerConfigs(path string) ([]ServerConfig, error) {
	var cfgs []ServerConfig
	if path == "" {
		path = defaultServerCfgPath
	}
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
		log.Println(err)
		return cfgs, err
	}
	for _, filePath := range filePaths {
		log.Println("Loading", filePath)
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
	cfg := DefaultUltravioletConfig()
	if path == "" {
		path = defaultUltravioletCfgPath
	}
	// Check or file exists and if not write default config file to it
	log.Printf("Loading Ultraviolet main config file at: %s", path)
	bb, err := ioutil.ReadFile(path)
	if err != nil {
		return UltravioletConfig{}, err
	}
	if err := json.Unmarshal(bb, &cfg); err != nil {
		return cfg, err
	}
	return cfg, nil
}
