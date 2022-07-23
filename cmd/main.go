package cmd

import (
	"errors"
	"flag"
	"fmt"
	"io/ioutil"
	"log"
	"net"
	"net/http"
	"os"
	"os/signal"
	"path/filepath"
	"runtime"
	"syscall"

	"github.com/cloudflare/tableflip"
	"github.com/pires/go-proxyproto"
	ultraviolet "github.com/realDragonium/Ultraviolet"
	ultravioletv2 "github.com/realDragonium/Ultraviolet/src"
	"github.com/realDragonium/Ultraviolet/config"
	"github.com/realDragonium/Ultraviolet/core"
	"github.com/realDragonium/Ultraviolet/worker"
)

var (
	defaultCfgPath = "/etc/ultraviolet"
	configPath     string
	uvVersion      = "(unknown version)"
	pidFilePath    = "/bin/ultraviolet/uv.pid"
	pidFileName    = "uv.pid"
	upg            *tableflip.Upgrader
)

func Main() {
	if len(os.Args) < 2 {
		log.Fatalf("Didnt receive enough arguments, try adding 'run' or 'reload' after the command")
	}

	flags := flag.NewFlagSet("", flag.ExitOnError)
	cfgDir := flags.String("config", defaultCfgPath, "`Path` to be used as directory")
	flags.Parse(os.Args[2:])

	configPath = *cfgDir

	switch os.Args[1] {
	case "run":
		ultravioletv2.Run()
		// log.Printf("Starting Ultraviolet %v", uvVersion)
		// err := runProxy(configPath)
		// log.Printf("got error while starting up: %v", err)
	case "reload":
		err := callReloadAPI(configPath)
		if err != nil {
			log.Fatalf("got error: %v ", err)
		}
		log.Println("Finished reloading")
	}
}

func runProxy(configPath string) error {
	uvReader := config.NewUVConfigFileReader(configPath)

	cfg, err := uvReader()
	if err != nil {
		return err
	}

	var newProxyFunc core.NewProxyFunc
	newProxyFunc = worker.NewProxy
	if cfg.UseLessStableMode {
		newProxyFunc = ultraviolet.NewProxy
	}

	return RunProxy(configPath, uvVersion, newProxyFunc)
}

func RunProxy(configPath, version string, newProxy core.NewProxyFunc) error {
	pidFilePath = filepath.Join(configPath, pidFileName)
	uvReader := config.NewUVConfigFileReader(configPath)
	serverCfgReader := config.NewBackendConfigFileReader(configPath, config.VerifyConfigs)
	cfg, err := uvReader()
	if err != nil {
		return err
	}

	notUseHotSwap := !cfg.UseTableflip || runtime.GOOS == "windows" || version == "docker"
	newListener, err := createListener(cfg, notUseHotSwap)
	if err != nil {
		return err
	}

	proxy := newProxy(uvReader, newListener, serverCfgReader.Read)
	err = proxy.Start()
	if err != nil {
		return err
	}

	if notUseHotSwap {
		select {}
	}

	if err := upg.Ready(); err != nil {
		panic(err)
	}
	<-upg.Exit()

	// log.Println("Waiting for all connections to be closed before shutting down")
	// for {
	// 	active := backendManager.CheckActiveConnections()
	// 	if !active {
	// 		break
	// 	}
	// 	time.Sleep(time.Minute)
	// }
	// log.Println("All connections closed, shutting down process")
	return nil
}

func createListener(cfg config.UltravioletConfig, notUseHotSwap bool) (net.Listener, error) {
	var ln net.Listener
	var err error
	if notUseHotSwap {
		ln, err = net.Listen("tcp", cfg.ListenTo)
	} else {
		if cfg.PidFile == "" {
			cfg.PidFile = pidFilePath
		}
		if _, err := os.Stat(cfg.PidFile); errors.Is(err, os.ErrNotExist) {
			pid := fmt.Sprint(os.Getpid())
			bb := []byte(pid)
			os.WriteFile(cfg.PidFile, bb, os.ModePerm)
		}
		ln, err = tableflipListener(cfg)
	}

	if err != nil {
		log.Fatalf("Can't listen: %v", err)
	}

	if cfg.AcceptProxyProtocol {
		policyFunc := func(upstream net.Addr) (proxyproto.Policy, error) {
			return proxyproto.REQUIRE, nil
		}
		proxyListener := &proxyproto.Listener{
			Listener: ln,
			Policy:   policyFunc,
		}
		return proxyListener, nil
	}
	return ln, nil
}

func tableflipListener(cfg config.UltravioletConfig) (net.Listener, error) {
	var err error
	upg, err = tableflip.New(tableflip.Options{
		PIDFile: cfg.PidFile,
	})
	if err != nil {
		log.Fatal(err)
	}
	go func() {
		sig := make(chan os.Signal, 1)
		signal.Notify(sig, syscall.SIGHUP)
		for range sig {
			err := upg.Upgrade()
			if err != nil {
				log.Println("upgrade failed:", err)
			}
		}
	}()
	return upg.Listen("tcp", cfg.ListenTo)
}

func callReloadAPI(configPath string) error {
	mainCfg, err := config.ReadUltravioletConfig(configPath)
	if err != nil {
		log.Fatalf("Read main config file error: %v", err)
	}
	url := fmt.Sprintf("http://%s/reload", mainCfg.APIBind)
	resp, err := http.Get(url)
	if err != nil {
		return err
	}
	bb, err := ioutil.ReadAll(resp.Body)
	if err != nil {
		return err
	}
	fmt.Printf("%s", bb)
	resp.Body.Close()
	return nil
}
