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

func TestReadServerConfig(t *testing.T) {
	cfg := config.ServerConfig{
		Domains: []string{"infrared"},
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
	cfg := config.ServerConfig{
		Domains: []string{"infrared"},
		ProxyTo: ":25566",
	}
	tmpDir, _ := ioutil.TempDir("", "configs")
	for i := 0; i < 3; i++ {
		file, _ := json.MarshalIndent(cfg, "", " ")
		tmpfile, err := ioutil.TempFile(tmpDir, "example*.json")
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
		loadedCfg.FilePath = ""
		if !reflect.DeepEqual(cfg, loadedCfg) {
			t.Errorf("index: %d \nWanted:%v \n got: %v", i, cfg, loadedCfg)
		}
	}
}

func TestReadServerConfigs_OnlyReadJson(t *testing.T) {
	cfg := config.ServerConfig{
		Domains: []string{"infrared"},
		ProxyTo: ":25566",
	}
	tmpDir, _ := ioutil.TempDir("", "configs")
	for i := 0; i < 3; i++ {
		fileName := "example*.json"
		if i == 0 {
			fileName = "example*"
		}
		file, _ := json.MarshalIndent(cfg, "", " ")
		tmpfile, err := ioutil.TempFile(tmpDir, fileName)
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
	if len(loadedCfgs) != 2 {
		t.Errorf("Expected 2 configs to be read but there are %d configs read", len(loadedCfgs))
	}
}

func TestReadUltravioletConfigFile(t *testing.T) {
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
	tmpfile, err := ioutil.TempFile("", "example*.json")
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

func samePK(expected, received mc.Packet) bool {
	sameID := expected.ID == received.ID
	sameData := bytes.Equal(expected.Data, received.Data)

	return sameID && sameData
}

func TestFileToWorkerConfig(t *testing.T) {
	serverCfg := config.ServerConfig{
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

	workerCfg := config.FileToWorkerConfig(serverCfg)

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
	}

	workerCfg := config.FileToWorkerConfig(serverCfg)

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
	}

	workerCfg := config.FileToWorkerConfig(serverCfg)
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
	pubkey := pub.(*ecdsa.PublicKey)
	newPubkey := workerCfg.RealIPKey.PublicKey
	if !reflect.DeepEqual(*pubkey, newPubkey) {
		t.Logf("generatedKey: %v", newPubkey)
		t.Logf("readKey: %v", pubkey)
		t.Fatal("Public keys arent the same!")
	}
}
