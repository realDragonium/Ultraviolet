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

var (
	ErrPrivateKey            = errors.New("could not load private key")
	ErrCantCombineConfigs    = errors.New("failed to combine config structs")
	ErrFailedToConvertConfig = errors.New("failed to convert server config to a more usable config")
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
		if filepath.Ext(path) != ".json" {
			return nil
		}
		if info.Name() == "ultraviolet.json" {
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
	cfg := DefaultServerConfig()
	if err := json.Unmarshal(bb, &cfg); err != nil {
		return cfg, err
	}
	cfg.FilePath = path
	return cfg, nil
}

func ReadUltravioletConfig(path string) (UltravioletConfig, error) {
	cfg := DefaultUltravioletConfig()

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

func CombineUltravioletConfigs(old, new UltravioletConfig) (UltravioletConfig, error) {
	cfg := old
	bb, err := json.Marshal(new)
	if err != nil {
		return cfg, ErrCantCombineConfigs
	}
	if err := json.Unmarshal(bb, &cfg); err != nil {
		return cfg, ErrCantCombineConfigs
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

var ErrNoDomainInConfig = errors.New("there wasnt any domain in config")
var ErrNoProxyToAddr = errors.New("there wasnt any domain in config")

func FileToWorkerConfig(cfg ServerConfig) (WorkerServerConfig, error) {
	if len(cfg.Domains) == 0 {
		return WorkerServerConfig{}, ErrNoDomainInConfig
	}
	if cfg.ProxyTo == "" {
		return WorkerServerConfig{}, ErrNoProxyToAddr
	}
	name := cfg.Name
	if name == "" {
		name = cfg.Domains[0]
	}
	workerCfg := WorkerServerConfig{
		Name:              name,
		ProxyTo:           cfg.ProxyTo,
		ProxyBind:         cfg.ProxyBind,
		SendProxyProtocol: cfg.SendProxyProtocol,
		RateLimit:         cfg.RateLimit,
		OldRealIp:         cfg.OldRealIP,
		NewRealIP:         cfg.NewRealIP,
		StateOption:       NewStateOption(cfg.CheckStateOption),
	}

	if cfg.NewRealIP {
		var privateKey *ecdsa.PrivateKey
		var err error
		privateKey, err = ReadPrivateKey(cfg.RealIPKey)
		if errors.Is(err, os.ErrNotExist) {
			if key, ok := existingGeneratedKey(cfg); ok {
				privateKey = key
			} else {
				log.Printf("No existing key for %s has been found, generating one...", cfg.Domains[0])
				privateKey = generateKeys(cfg)
			}
		} else if err != nil {
			return WorkerServerConfig{}, err
		}
		workerCfg.NewRealIP = true
		workerCfg.RealIPKey = privateKey
	}
	disconPk := mc.ClientBoundDisconnect{
		Reason: mc.Chat(cfg.DisconnectMessage),
	}.Marshal()
	workerCfg.DisconnectPacket = disconPk

	offlineStatusPk := cfg.OfflineStatus.Marshal()
	workerCfg.OfflineStatus = offlineStatusPk

	stateUpdateCooldown, err := time.ParseDuration(cfg.StateUpdateCooldown)
	if err != nil {
		stateUpdateCooldown = time.Second
	}
	workerCfg.StateUpdateCooldown = stateUpdateCooldown

	dialTimeout, err := time.ParseDuration(cfg.DialTimeout)
	if err != nil {
		dialTimeout = time.Second
	}
	workerCfg.DialTimeout = dialTimeout

	if cfg.CacheStatus {
		cacheCooldown, err := time.ParseDuration(cfg.CacheUpdateCooldown)
		if err != nil {
			cacheCooldown = time.Second
		}
		workerCfg.CacheStatus = true
		workerCfg.CacheUpdateCooldown = cacheCooldown
		workerCfg.ValidProtocol = cfg.ValidProtocol
	}

	if cfg.RateLimit > 0 {
		rateDuration, err := time.ParseDuration(cfg.RateDuration)
		if err != nil {
			rateDuration = time.Second
		}
		rateBanCooldown, err := time.ParseDuration(cfg.RateBanListCooldown)
		if err != nil {
			rateBanCooldown = 15 * time.Minute
		}
		rateDisconPk := mc.ClientBoundDisconnect{
			Reason: mc.String(cfg.RateDisconMsg),
		}.Marshal()

		workerCfg.RateLimitDuration = rateDuration
		workerCfg.RateBanListCooldown = rateBanCooldown
		workerCfg.RateDisconPk = rateDisconPk
	}
	return workerCfg, nil
}

func CombineServerConfigs(old, new ServerConfig) (ServerConfig, error) {
	cfg := old
	bb, err := json.Marshal(new)
	if err != nil {
		return cfg, ErrCantCombineConfigs
	}
	if err := json.Unmarshal(bb, &cfg); err != nil {
		return cfg, ErrCantCombineConfigs
	}
	return cfg, nil
}
