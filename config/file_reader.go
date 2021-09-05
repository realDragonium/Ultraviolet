package config

import (
	"encoding/json"
	"errors"
	"io/ioutil"
	"os"
	"path/filepath"
)

var ErrNoConfigFiles = errors.New("no config files found")

func NewBackendConfigFileReader(path string, verifier VerifyFunc) backendConfigFileReader {
	return backendConfigFileReader{
		path:     path,
		verifier: verifier,
	}
}

type backendConfigFileReader struct {
	path     string
	verifier VerifyFunc
}

func (reader backendConfigFileReader) Read() ([]ServerConfig, error) {
	cfgs, err := ReadServerConfigs(reader.path)
	if err != nil {
		return nil, err
	}
	err = reader.verifier(cfgs)
	if err != nil {
		return cfgs, err
	}
	return cfgs, nil
}

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
		if filepath.Ext(path) != ".json" {
			return nil
		}
		if info.Name() == MainConfigFileName {
			return nil
		}
		filePaths = append(filePaths, path)
		return nil
	})
	if len(filePaths) == 0 {
		return cfgs, ErrNoConfigFiles
	}
	if err != nil {
		return cfgs, err
	}
	for _, filePath := range filePaths {
		cfg, err := LoadServerCfgFromPath(filePath)
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
	cfg := DefaultServerConfig()
	if err := json.Unmarshal(bb, &cfg); err != nil {
		return cfg, err
	}
	cfg.FilePath = path
	return cfg, nil
}

func NewIVConfigFileReader(path string) UVConfigReader {
	return uvConfigFileReader{
		path: path,
	}.Read
}

func NewUVConfigFileReader(path string) UVConfigReader {
	return uvConfigFileReader{
		path: path,
	}.Read
}

type uvConfigFileReader struct {
	path string
}

func (reader uvConfigFileReader) Read() (UltravioletConfig, error) {
	return ReadUltravioletConfig(reader.path)
}

func NewUVReader(cfg UltravioletConfig) func()(UltravioletConfig, error) {
	return func() (UltravioletConfig, error) {
		return cfg, nil
	}
}
