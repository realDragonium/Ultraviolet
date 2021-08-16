package config_test

import (
	"encoding/json"
	"errors"
	"io/ioutil"
	"os"
	"path/filepath"
	"reflect"
	"strings"
	"testing"

	"github.com/realDragonium/Ultraviolet/config"
	"github.com/realDragonium/Ultraviolet/mc"
)

var emptyVerifyFunc = func(cfgs []config.ServerConfig) error {
	return nil
}

func writeDefaultMainConfig(path string) {
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
	filename := filepath.Join(path, config.MainConfigFileName)
	os.WriteFile(filename, file, os.ModePerm)
}

func writeServerConfig(path string, cfg config.ServerConfig) error {
	file, err := json.MarshalIndent(cfg, "", " ")
	if err != nil {
		return err
	}
	tmpfile, err := ioutil.TempFile(path, "config*.json")
	if err != nil {
		return err
	}
	if _, err := tmpfile.Write(file); err != nil {
		return err
	}
	if err := tmpfile.Close(); err != nil {
		return err
	}
	return nil
}

func TestBackendConfigFileReader(t *testing.T) {
	tmpDir, err := ioutil.TempDir("", "file_config")
	if err != nil {
		t.Fatal(err)
	}
	defer os.RemoveAll(tmpDir)
	createDir := func() string {
		path, err := ioutil.TempDir(tmpDir, "config")
		if err != nil {
			t.Fatal(err)
		}
		return path
	}

	t.Run("should read no config files", func(t *testing.T) {
		tt := []struct {
			name  string
			setup func(path string)
		}{
			{
				name:  "empty dir",
				setup: func(path string) {},
			},
			{
				name: "non existing dir",
				setup: func(path string) {
					os.Remove(path)
				},
			},
			{
				name: "only main config",
				setup: func(path string) {
					writeDefaultMainConfig(path)
				},
			},
			{
				name: "config without .json",
				setup: func(path string) {
					filename := filepath.Join(path, "serverconfig")
					os.WriteFile(filename, []byte{1, 2, 3, 4}, os.ModePerm)
				},
			},
			{
				name: "dir contains empty dir",
				setup: func(path string) {
					_, err := ioutil.TempDir(path, "empty")
					if err != nil {
						t.Fatal(err)
					}
				},
			},
		}

		for _, tc := range tt {
			t.Run(tc.name, func(t *testing.T) {
				testDir := createDir()
				tc.setup(testDir)
				cfgReader := config.NewBackendConfigFileReader(testDir, emptyVerifyFunc)
				_, err := cfgReader.Read()
				if !errors.Is(err, config.ErrNoConfigFiles) {
					t.Errorf("expected no config files error but got: %v", err)
				}
			})
		}
	})

	t.Run("reads right amount of configs", func(t *testing.T) {
		testAmount := func(path string, expectedAmount int) {
			cfgReader := config.NewBackendConfigFileReader(path, emptyVerifyFunc)
			cfgs, err := cfgReader.Read()
			if err != nil {
				t.Fatal(err)
			}

			if len(cfgs) != expectedAmount {
				t.Errorf("expected %d configs, but got %d", expectedAmount, len(cfgs))
			}
		}

		t.Run("one config in main dir", func(t *testing.T) {
			testDir := createDir()
			expectedAmountConfigs := 1
			serverCfg := config.ServerConfig{
				Domains: []string{"uv"},
				ProxyTo: ":25566",
			}
			err := writeServerConfig(testDir, serverCfg)
			if err != nil {
				t.Fatal(err)
			}
			testAmount(testDir, expectedAmountConfigs)
		})

		t.Run("two configs in main dir", func(t *testing.T) {
			testDir := createDir()
			expectedAmountConfigs := 2
			serverCfg := config.ServerConfig{
				Domains: []string{"uv"},
				ProxyTo: ":25566",
			}
			serverCfg2 := config.ServerConfig{
				Domains: []string{"uv1"},
				ProxyTo: ":25567",
			}
			err := writeServerConfig(testDir, serverCfg)
			if err != nil {
				t.Fatal(err)
			}
			err = writeServerConfig(testDir, serverCfg2)
			if err != nil {
				t.Fatal(err)
			}
			testAmount(testDir, expectedAmountConfigs)
		})

		t.Run("one config in sub dir", func(t *testing.T) {
			testDir := createDir()
			subDir, err := ioutil.TempDir(testDir, "subdir")
			if err != nil {
				t.Fatal(err)
			}
			expectedAmountConfigs := 1
			serverCfg := config.ServerConfig{
				Domains: []string{"uv"},
				ProxyTo: ":25566",
			}

			err = writeServerConfig(subDir, serverCfg)
			if err != nil {
				t.Fatal(err)
			}
			testAmount(testDir, expectedAmountConfigs)
		})
	})

	t.Run("verify func", func(t *testing.T) {
		testDir := createDir()
		serverCfg := config.ServerConfig{
			Domains: []string{"uv"},
			ProxyTo: ":25566",
		}
		err := writeServerConfig(testDir, serverCfg)
		if err != nil {
			t.Fatal(err)
		}
		t.Run("Calls verify func when there are configs to verify", func(t *testing.T) {
			counter := 0
			verifyFunc := func(cfgs []config.ServerConfig) error {
				counter++
				return nil
			}
			cfgReader := config.NewBackendConfigFileReader(testDir, verifyFunc)
			_, err = cfgReader.Read()
			if counter != 1 {
				t.Errorf("expected to be called once but was called %d times", counter)
			}
			if err != nil {
				t.Error("didnt expect an error")
			}
		})
		t.Run("returns error when there is an error", func(t *testing.T) {
			tErr := errors.New("test error")
			verifyFunc := func(cfgs []config.ServerConfig) error {
				return tErr
			}
			cfgReader := config.NewBackendConfigFileReader(testDir, verifyFunc)
			_, err = cfgReader.Read()
			if err == nil {
				t.Error("expected an error")
			}
		})
	})

	t.Run("should know their file path", func(t *testing.T) {
		testDir := createDir()
		verifyFunc := func(cfgs []config.ServerConfig) error {
			return nil
		}
		serverCfg := config.ServerConfig{
			Domains: []string{"uv"},
			ProxyTo: ":25566",
		}

		err = writeServerConfig(testDir, serverCfg)
		if err != nil {
			t.Fatal(err)
		}
		cfgReader := config.NewBackendConfigFileReader(testDir, verifyFunc)
		cfgs, err := cfgReader.Read()
		if err != nil {
			t.Fatal(err)
		}
		for _, cfg := range cfgs {
			if !strings.HasPrefix(cfg.FilePath, testDir) {
				t.Error("config should have know its config path")
				t.Log(testDir)
				t.Log(cfg.FilePath)
			}
		}
	})

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
			specialName:      config.MainConfigFileName,
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
