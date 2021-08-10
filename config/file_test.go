package config_test

import (
	"bytes"
	"crypto/ecdsa"
	"crypto/elliptic"
	"crypto/rand"
	"crypto/x509"
	"encoding/json"
	"errors"
	"fmt"
	"io/ioutil"
	"os"
	"path/filepath"
	"reflect"
	"testing"
	"time"

	"github.com/realDragonium/Ultraviolet/config"
	"github.com/realDragonium/Ultraviolet/mc"
)

func samePK(expected, received mc.Packet) bool {
	sameID := expected.ID == received.ID
	sameData := bytes.Equal(expected.Data, received.Data)
	return sameID && sameData
}

func TestReadServerConfig(t *testing.T) {
	cfg := config.ServerConfig{
		Domains: []string{"ultraviolet"},
		ProxyTo: ":25566",
	}
	tmpfile, err := ioutil.TempFile("", "example*.json")
	cfg.FilePath = tmpfile.Name()
	file, _ := json.MarshalIndent(cfg, "", " ")
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
	generateNumberOfFile := 3
	cfg := config.ServerConfig{
		Domains: []string{"ultraviolet"},
		ProxyTo: ":25566",
	}
	tt := []struct {
		testName         string
		hasDifferentFile bool
		specialName      string
		expectedReadFile int
	}{
		{
			testName:         "normal configs",
			hasDifferentFile: false,
			expectedReadFile: generateNumberOfFile,
		},
		{
			testName:         "doesnt read file with no extension",
			hasDifferentFile: true,
			specialName:      "example*",
			expectedReadFile: generateNumberOfFile - 1,
		},
		{
			testName:         "doesnt read file with different extension",
			hasDifferentFile: true,
			specialName:      "example*.yml",
			expectedReadFile: generateNumberOfFile - 1,
		},
		{
			testName:         "doesnt read ultraviolet config file",
			hasDifferentFile: true,
			specialName:      "ultraviolet.json",
			expectedReadFile: generateNumberOfFile - 1,
		},
	}

	for _, tc := range tt {
		t.Run(tc.testName, func(t *testing.T) {
			tmpDir, _ := ioutil.TempDir("", "configs")
			bb, _ := json.MarshalIndent(cfg, "", " ")
			for i := 0; i < generateNumberOfFile; i++ {
				fileName := "example*.json"
				if tc.hasDifferentFile && i == 0 {
					fileName = tc.specialName
				}
				tmpfile, err := ioutil.TempFile(tmpDir, fileName)
				if err != nil {
					t.Fatal(err)
				}
				if _, err := tmpfile.Write(bb); err != nil {
					t.Fatal(err)
				}
				if err := tmpfile.Close(); err != nil {
					t.Fatal(err)
				}
			}
			loadedCfgs, _ := config.ReadServerConfigs(tmpDir)
			for i, loadedCfg := range loadedCfgs {
				loadedCfg.FilePath = ""
				if !reflect.DeepEqual(cfg, loadedCfg) {
					t.Errorf("index: %d \nWanted:%v \n got: %v", i, cfg, loadedCfg)
				}
			}
			if len(loadedCfgs) != tc.expectedReadFile {
				t.Errorf("Expected %v configs to be read but there are %d configs read", tc.expectedReadFile, len(loadedCfgs))
			}
		})
	}
}

func TestReadUltravioletConfigFile(t *testing.T) {
	t.Run("normal config", func(t *testing.T) {
		cfg := config.UltravioletConfig{
			ListenTo: ":25565",
			DefaultStatus: mc.SimpleStatus{
				Name:        "Ultraviolet",
				Protocol:    755,
				Description: "One dangerous proxy",
			},
			NumberOfWorkers: 5,
		}

		file, _ := json.MarshalIndent(cfg, "", " ")
		tmpDir, err := ioutil.TempDir("", "uv-normal*")
		if err != nil {
			t.Fatal(err)
		}
		filename := filepath.Join(tmpDir, "ultraviolet.json")
		os.WriteFile(filename, file, os.ModeAppend)
		loadedCfg, err := config.ReadUltravioletConfig(tmpDir)
		if err != nil {
			t.Error(err)
		}

		expectedCfg, err := config.CombineUltravioletConfigs(config.DefaultUltravioletConfig(), cfg)
		if err != nil {
			t.Fatalf("didnt expect error but got: %v", err)
		}

		if !reflect.DeepEqual(expectedCfg, loadedCfg) {
			t.Errorf("Wanted:%v \n got: %v", cfg, loadedCfg)
		}
	})

	t.Run("creates folder if it does not exist", func(t *testing.T) {
		tmpDir, err := ioutil.TempDir("", "uv-normal*")
		if err != nil {
			t.Fatal(err)
		}
		dirPath := filepath.Join(tmpDir, "uv-dir")
		config.ReadUltravioletConfig(dirPath)
		if _, err := os.Stat(dirPath); os.IsNotExist(err) {
			t.Errorf("there was no dir created at %s", dirPath)
		}
	})

	t.Run("creates file if it does not exist", func(t *testing.T) {
		tmpDir, err := ioutil.TempDir("", "uv-normal*")
		if err != nil {
			t.Fatal(err)
		}
		dirPath := filepath.Join(tmpDir, "uv-dir")
		cfg, err := config.ReadUltravioletConfig(dirPath)
		if err != nil {
			t.Fatal(err)
		}
		if _, err := os.Stat(dirPath); os.IsNotExist(err) {
			t.Errorf("there was no dir created at %s", dirPath)
		}

		expectedCfg := config.DefaultUltravioletConfig()
		if !reflect.DeepEqual(cfg, expectedCfg) {
			t.Error("expected config to be the same")
			t.Logf("expected: %v", expectedCfg)
			t.Logf("got: %v", cfg)
		}
	})

}

func TestReadRealIPPrivateKeyFile(t *testing.T) {
	privKey, err := ecdsa.GenerateKey(elliptic.P256(), rand.Reader)
	if err != nil {
		t.Fatalf("got error: %v", err)
	}
	keyBytes, err := x509.MarshalECPrivateKey(privKey)
	if err != nil {
		t.Fatalf("error during marshal key: %v", err)
	}

	tmpfile, err := ioutil.TempFile("", "example")
	if err != nil {
		t.Fatal(err)
	}
	defer os.Remove(tmpfile.Name())
	if _, err := tmpfile.Write(keyBytes); err != nil {
		t.Fatal(err)
	}
	if err := tmpfile.Close(); err != nil {
		t.Fatal(err)
	}
	readKey, err := config.ReadPrivateKey(tmpfile.Name())
	if err != nil {
		t.Fatalf("error during key reading: %v", err)
	}

	if !readKey.Equal(privKey) {
		t.Logf("generatedKey: %v", privKey)
		t.Logf("readKey: %v", readKey)
		t.Fatal("Keys arent the same!")
	}
}

func TestReadRealIPPrivateKey_NonExistingFile_ReturnsError(t *testing.T) {
	fileName := "this-private-key"
	tmpDir, _ := ioutil.TempDir("", "configs")
	filePath := filepath.Join(tmpDir, fileName)
	_, err := config.ReadPrivateKey(filePath)
	if !errors.Is(err, os.ErrNotExist) {
		t.Fatalf("error during key reading: %v", err)
	}
}

func TestFileToWorkerConfig(t *testing.T) {
	t.Run("filled in values should match", func(t *testing.T) {
		serverCfg := config.ServerConfig{
			Name:              "UV",
			Domains:           []string{"Ultraviolet", "Ultraviolet2", "UltraV", "UV"},
			ProxyTo:           "127.0.10.5:25565",
			ProxyBind:         "127.0.0.5",
			OldRealIP:         true,
			DialTimeout:       "1s",
			SendProxyProtocol: true,
			DisconnectMessage: "HelloThereWeAreClosed...Sorry",
			OfflineStatus: mc.SimpleStatus{
				Name:        "Ultraviolet",
				Protocol:    755,
				Description: "Some broken proxy",
			},
			RateLimit:           5,
			RateDuration:        "1m",
			StateUpdateCooldown: "1m",
		}

		expectedDisconPk := mc.ClientBoundDisconnect{
			Reason: mc.String(serverCfg.DisconnectMessage),
		}.Marshal()
		expectedOfflineStatus := mc.SimpleStatus{
			Name:        "Ultraviolet",
			Protocol:    755,
			Description: "Some broken proxy",
		}.Marshal()
		expectedRateDuration := 1 * time.Minute
		expectedUpdateCooldown := 1 * time.Minute
		expectedDialTimeout := 1 * time.Second

		workerCfg, err := config.FileToWorkerConfig(serverCfg)
		if err != nil {
			t.Fatalf("received unexpected error: %v", err)
		}

		if workerCfg.Name != serverCfg.Name {
			t.Errorf("expected: %v - got: %v", serverCfg.Name, workerCfg.Name)
		}
		if workerCfg.ProxyTo != serverCfg.ProxyTo {
			t.Errorf("expected: %v - got: %v", serverCfg.ProxyTo, workerCfg.ProxyTo)
		}
		if workerCfg.ProxyBind != serverCfg.ProxyBind {
			t.Errorf("expected: %v - got: %v", serverCfg.ProxyBind, workerCfg.ProxyBind)
		}
		if workerCfg.SendProxyProtocol != serverCfg.SendProxyProtocol {
			t.Errorf("expected: %v - got: %v", serverCfg.SendProxyProtocol, workerCfg.SendProxyProtocol)
		}
		if workerCfg.RateLimit != serverCfg.RateLimit {
			t.Errorf("expected: %v - got: %v", serverCfg.RateLimit, workerCfg.RateLimit)
		}
		if workerCfg.OldRealIp != serverCfg.OldRealIP {
			t.Errorf("expected: %v - got: %v", serverCfg.OldRealIP, workerCfg.OldRealIp)
		}
		if expectedRateDuration != workerCfg.RateLimitDuration {
			t.Errorf("expected: %v - got: %v", expectedRateDuration, workerCfg.RateLimitDuration)
		}
		if expectedUpdateCooldown != workerCfg.StateUpdateCooldown {
			t.Errorf("expected: %v - got: %v", expectedRateDuration, workerCfg.StateUpdateCooldown)
		}
		if expectedDialTimeout != workerCfg.DialTimeout {
			t.Errorf("expected: %v - got: %v", expectedDialTimeout, workerCfg.DialTimeout)
		}
		if !samePK(expectedOfflineStatus, workerCfg.OfflineStatus) {
			offlineStatus, _ := mc.UnmarshalClientBoundResponse(expectedOfflineStatus)
			receivedStatus, _ := mc.UnmarshalClientBoundResponse(workerCfg.OfflineStatus)
			t.Errorf("expcted: %v \ngot: %v", offlineStatus, receivedStatus)
		}

		if !samePK(expectedDisconPk, workerCfg.DisconnectPacket) {
			expectedDiscon, _ := mc.UnmarshalClientDisconnect(expectedDisconPk)
			receivedDiscon, _ := mc.UnmarshalClientDisconnect(workerCfg.DisconnectPacket)
			t.Errorf("expcted: %v \ngot: %v", expectedDiscon, receivedDiscon)
		}
	})

	t.Run("when no name, first domain will be name", func(t *testing.T) {
		serverCfg := config.ServerConfig{
			Domains: []string{"Ultraviolet", "UV"},
			ProxyTo: "1",
		}
		workerCfg, err := config.FileToWorkerConfig(serverCfg)
		if err != nil {
			t.Fatalf("received unexpected error: %v", err)
		}
		if workerCfg.Name != serverCfg.Domains[0] {
			t.Errorf("expected: %v - got: %v", serverCfg.Domains[0], workerCfg.Name)
		}
	})

	t.Run("returns error when there are no domains", func(t *testing.T) {
		serverCfg := config.ServerConfig{
			ProxyTo: ":9284",
		}
		_, err := config.FileToWorkerConfig(serverCfg)
		if !errors.Is(err, config.ErrNoDomainInConfig) {
			t.Errorf("expected no domain error but instead got: %v", err)
		}
	})

	t.Run("returns error when there is no target", func(t *testing.T) {
		serverCfg := config.ServerConfig{
			Domains: []string{"uv"},
		}
		_, err := config.FileToWorkerConfig(serverCfg)
		if !errors.Is(err, config.ErrNoProxyToAddr) {
			t.Errorf("expected no proxy target error but instead got: %v", err)
		}
	})
}

func TestFileToWorkerConfig_NewRealIP_ReadsKeyCorrectly(t *testing.T) {
	privKey, err := ecdsa.GenerateKey(elliptic.P256(), rand.Reader)
	if err != nil {
		t.Fatalf("error during creating privatekey: %v", err)
	}
	keyBytes, err := x509.MarshalECPrivateKey(privKey)
	if err != nil {
		t.Fatalf("error during marshal key: %v", err)
	}

	keyFile, err := ioutil.TempFile("", "example")
	if err != nil {
		t.Fatal(err)
	}
	defer os.Remove(keyFile.Name())
	if _, err := keyFile.Write(keyBytes); err != nil {
		t.Fatal(err)
	}
	if err := keyFile.Close(); err != nil {
		t.Fatal(err)
	}
	keyPath := keyFile.Name()
	serverCfg := config.ServerConfig{
		Domains:   []string{"Ultraviolet"},
		NewRealIP: true,
		RealIPKey: keyPath,
		ProxyTo:   "1",
	}

	workerCfg, err := config.FileToWorkerConfig(serverCfg)
	if err != nil {
		t.Fatalf("received unexpected error: %v", err)
	}

	if !workerCfg.RealIPKey.Equal(privKey) {
		t.Logf("generatedKey: %v", privKey)
		t.Logf("readKey: %v", workerCfg.RealIPKey)
		t.Fatal("Keys arent the same!")
	}
}

func TestFileToWorkerConfig_NewRealIP_GenerateKeyCorrect(t *testing.T) {
	tmpDir, _ := ioutil.TempDir("", "configs")
	cfgPath := filepath.Join(tmpDir, "config")
	firstDomainName := "Ultraviolet"
	keyPrivatePath := filepath.Join(tmpDir, fmt.Sprintf("%s-%s", firstDomainName, "private.key"))
	keyPublicPath := filepath.Join(tmpDir, fmt.Sprintf("%s-%s", firstDomainName, "public.key"))
	serverCfg := config.ServerConfig{
		FilePath:  cfgPath,
		Domains:   []string{firstDomainName},
		NewRealIP: true,
		ProxyTo:   "1",
	}

	workerCfg, err := config.FileToWorkerConfig(serverCfg)
	if err != nil {
		t.Fatalf("received unexpected error: %v", err)
	}

	readKey, err := config.ReadPrivateKey(keyPrivatePath)
	if err != nil {
		t.Fatalf("error during key reading: %v", err)
	}
	if !reflect.DeepEqual(workerCfg.RealIPKey, readKey) {
		t.Logf("generatedKey: %v", workerCfg.RealIPKey)
		t.Logf("readKey: %v", readKey)
		t.Fatal("Private keys arent the same!")
	}

	bb, err := ioutil.ReadFile(keyPublicPath)
	if err != nil {
		t.Fatalf("error during key reading: %v", err)
	}
	pub, err := x509.ParsePKIXPublicKey(bb)
	if err != nil {
		t.Fatalf("didnt expect error but got: %v", err)
	}
	pubkey := pub.(*ecdsa.PublicKey)
	newPubkey := workerCfg.RealIPKey.PublicKey
	if !reflect.DeepEqual(*pubkey, newPubkey) {
		t.Logf("generatedKey: %v", newPubkey)
		t.Logf("readKey: %v", pubkey)
		t.Fatal("Public keys arent the same!")
	}
}
