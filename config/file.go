package config

import (
	"crypto/ecdsa"
	"crypto/elliptic"
	"crypto/rand"
	"crypto/x509"
	"encoding/json"
	"errors"
	"fmt"
	"io/ioutil"
	"log"
	"os"
	"path/filepath"
	"time"

	"github.com/realDragonium/Ultraviolet/mc"
)

var ErrPrivateKey = errors.New("could not load private key")

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
	cfg.FilePath = path
	return cfg, nil
}

func ReadUltravioletConfig(path string) (UltravioletConfig, error) {
	cfg := defaultUltravioletConfig() 

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

func ReadPrivateKey(path string) (*ecdsa.PrivateKey, error) {
	var key *ecdsa.PrivateKey
	bb, err := ioutil.ReadFile(path)
	if err != nil {
		return key, err
	}
	return x509.ParseECPrivateKey(bb)
}

func FileToWorkerConfig(cfg ServerConfig) WorkerServerConfig {
	var privateKey *ecdsa.PrivateKey
	if cfg.NewRealIP {
		var err error
		privateKey, err = ReadPrivateKey(cfg.RealIPKey)
		if errors.Is(err, os.ErrNotExist) {
			//Check or there is already one generate or save the generated path
			// TODO: IMPROVE THIS
			if key, ok := existingGeneratedKey(cfg); ok {
				privateKey = key
			} else {
				log.Printf("No existing key for %s has been found, generating one...", cfg.Domains[0])
				privateKey = generateKeys(cfg)
			}
		} else if err != nil {
			log.Printf("error during reading of private key: %v", err)
		}
	}
	disconPk := mc.ClientBoundDisconnect{
		Reason: mc.Chat(cfg.DisconnectMessage),
	}.Marshal()
	offlineStatusPk := cfg.OfflineStatus.Marshal()
	duration, _ := time.ParseDuration(cfg.RateDuration)
	if duration == 0 {
		duration = time.Second
	}
	cooldown, _ := time.ParseDuration(cfg.StateUpdateCooldown)
	if cooldown == 0 {
		cooldown = time.Second
	}
	dialTimeout, _ := time.ParseDuration(cfg.DialTimeout)
	if dialTimeout == 0 {
		dialTimeout = time.Second
	}
	cacheCooldown, _ := time.ParseDuration(cfg.CacheUpdateCooldown)
	if cacheCooldown == 0 {
		cacheCooldown = time.Second
	}
	return WorkerServerConfig{
		ProxyTo:             cfg.ProxyTo,
		ProxyBind:           cfg.ProxyBind,
		DialTimeout:         dialTimeout,
		SendProxyProtocol:   cfg.SendProxyProtocol,
		CacheStatus:         cfg.CacheStatus,
		ValidProtocol:       cfg.ValidProtocol,
		CacheUpdateCooldown: cacheCooldown,
		OfflineStatus:       offlineStatusPk,
		DisconnectPacket:    disconPk,
		RateLimit:           cfg.RateLimit,
		RateLimitStatus:     cfg.RateLimitStatus,
		RateLimitDuration:   duration,
		StateUpdateCooldown: cooldown,
		OldRealIp:           cfg.OldRealIP,
		NewRealIP:           cfg.NewRealIP,
		RealIPKey:           privateKey,
	}
}

func existingGeneratedKey(cfg ServerConfig) (*ecdsa.PrivateKey, bool) {
	dir := filepath.Dir(cfg.FilePath)
	privkeyFileName := filepath.Join(dir, fmt.Sprintf("%s-%s", cfg.Domains[0], "private.key"))
	if _, err := os.Stat(privkeyFileName); err != nil {
		if os.IsNotExist(err) {
			return nil, false
		}
	}
	privateKey, err := ReadPrivateKey(privkeyFileName)
	if err != nil {
		log.Printf("error during reading key: %v", err)
		return nil, false
	}
	return privateKey, true
}

func generateKeys(cfg ServerConfig) *ecdsa.PrivateKey {
	privkey, err := ecdsa.GenerateKey(elliptic.P384(), rand.Reader)
	if err != nil {
		log.Printf("error during creating privatekey: %v", err)
		return privkey
	}
	pubkey := privkey.Public()
	dir := filepath.Dir(cfg.FilePath)
	privkeyFileName := filepath.Join(dir, fmt.Sprintf("%s-%s", cfg.Domains[0], "private.key"))
	pubkeyFileName := filepath.Join(dir, fmt.Sprintf("%s-%s", cfg.Domains[0], "public.key"))

	privkeyFile, err := os.Create(privkeyFileName)
	if err != nil {
		log.Printf("error during creating private key file: %v", err)
	}
	privkeyBytes, err := x509.MarshalECPrivateKey(privkey)
	if err != nil {
		log.Printf("error during marshal private key: %v", err)
	}
	if _, err := privkeyFile.Write(privkeyBytes); err != nil {
		log.Printf("error during saving private key to file: %v", err)
	}
	if err := privkeyFile.Close(); err != nil {
		log.Printf("error during closing private key file: %v", err)
	}

	pubkeyFile, err := os.Create(pubkeyFileName)
	if err != nil {
		log.Printf("error during creating public key file: %v", err)
	}
	pubkeyBytes, err := x509.MarshalPKIXPublicKey(pubkey)
	if err != nil {
		log.Printf("error during marshal public key: %v", err)
	}
	if _, err := pubkeyFile.Write(pubkeyBytes); err != nil {
		log.Printf("error during saving public key to file: %v", err)
	}
	if err := pubkeyFile.Close(); err != nil {
		log.Printf("error during closing public key file: %v", err)
	}
	return privkey
}

func FileToWorkerConfig2(cfg ServerConfig) (WorkerServerConfig2, error) {
	var privateKey *ecdsa.PrivateKey
	if cfg.NewRealIP {
		var err error
		privateKey, err = ReadPrivateKey(cfg.RealIPKey)
		if errors.Is(err, os.ErrNotExist) {
			//Check or there is already one generate or save the generated path
			// TODO: IMPROVE THIS
			if key, ok := existingGeneratedKey(cfg); ok {
				privateKey = key
			} else {
				log.Printf("No existing key for %s has been found, generating one...", cfg.Domains[0])
				privateKey = generateKeys(cfg)
			}
		} else if err != nil {
			log.Printf("error during reading of private key: %v", err)
		}
	}
	disconPk := mc.ClientBoundDisconnect{
		Reason: mc.Chat(cfg.DisconnectMessage),
	}.Marshal()
	offlineStatusPk := cfg.OfflineStatus.Marshal()
	duration, _ := time.ParseDuration(cfg.RateDuration)
	if duration == 0 {
		duration = time.Second
	}
	cooldown, _ := time.ParseDuration(cfg.StateUpdateCooldown)
	if cooldown == 0 {
		cooldown = time.Second
	}
	dialTimeout, _ := time.ParseDuration(cfg.DialTimeout)
	if dialTimeout == 0 {
		dialTimeout = time.Second
	}
	cacheCooldown, _ := time.ParseDuration(cfg.CacheUpdateCooldown)
	if cacheCooldown == 0 {
		cacheCooldown = time.Second
	}
	offlineBytes, err := offlineStatusPk.Marshal()
	if err != nil {
		return WorkerServerConfig2{}, err
	}
	disconBytes, err := disconPk.Marshal()
	if err != nil {
		return WorkerServerConfig2{}, err
	}
	return WorkerServerConfig2{
		ProxyTo:             cfg.ProxyTo,
		ProxyBind:           cfg.ProxyBind,
		DialTimeout:         dialTimeout,
		SendProxyProtocol:   cfg.SendProxyProtocol,
		CacheStatus:         cfg.CacheStatus,
		ValidProtocol:       cfg.ValidProtocol,
		CacheUpdateCooldown: cacheCooldown,
		OfflineStatus:       offlineBytes,
		DisconnectPacket:    disconBytes,
		RateLimit:           cfg.RateLimit,
		RateLimitStatus:     cfg.RateLimitStatus,
		RateLimitDuration:   duration,
		StateUpdateCooldown: cooldown,
		OldRealIp:           cfg.OldRealIP,
		NewRealIP:           cfg.NewRealIP,
		RealIPKey:           privateKey,
	}, nil
}
