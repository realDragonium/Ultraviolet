package cmd

import (
	"errors"
	"flag"
	"fmt"
	"io/ioutil"
	"log"
	"math/rand"
	"net"
	"net/http"
	"os"
	"os/signal"
	"path/filepath"
	"runtime"
	"syscall"
	"time"

	"github.com/cloudflare/tableflip"
	"github.com/pires/go-proxyproto"
	ultraviolet "github.com/realDragonium/Ultraviolet"
	"github.com/realDragonium/Ultraviolet/config"
	"github.com/realDragonium/Ultraviolet/mc"
)

var (
	defaultCfgPath = "/etc/ultraviolet"
	configPath     string
	uvVersion      = "(unknown version)"
	upg            *tableflip.Upgrader
	pidFileName    = "uv.pid"
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
		log.Printf("Starting Ultraviolet %v", uvVersion)
		err := runProxy(configPath)
		log.Printf("got error while starting up: %v", err)
	case "reload":
		err := callReloadAPI(configPath)
		if err != nil {
			log.Fatalf("got error: %v ", err)
		}
		log.Println("Finished reloading")
	}
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

func runProxy(configPath string) error {
	uvReader := config.NewUVConfigFileReader(configPath)
	serverCfgReader := config.NewBackendConfigFileReader(configPath, config.VerifyConfigs)
	cfg, err := uvReader()
	if err != nil {
		return err
	}
	notUseHotSwap := !cfg.UseTableflip || runtime.GOOS == "windows" || uvVersion == "docker"
	newListener, err := createListener(cfg, notUseHotSwap)
	// newListener, err := newStressTestListener(cfg)
	if err != nil {
		return err
	}
	proxy := ultraviolet.NewProxy(uvReader, newListener, serverCfgReader.Read)
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

	// promeServer.Close()
	// API.Close()

	log.Println("Waiting for all connections to be closed before shutting down")
	for {
		active := ultraviolet.BackendManager.CheckActiveConnections()
		if !active {
			break
		}
		time.Sleep(time.Minute)
	}
	log.Println("All connections closed, shutting down process")
	return nil
}

func createListener(cfg config.UltravioletConfig, notUseHotSwap bool) (net.Listener, error) {
	var ln net.Listener
	var err error
	if notUseHotSwap {
		ln, err = net.Listen("tcp", cfg.ListenTo)
	} else {
		if cfg.PidFile == "" {
			cfg.PidFile = filepath.Join(configPath, pidFileName)
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

type stressTestListener struct {
	interval      time.Duration
	realRequestCh chan net.Conn
	r             *rand.Rand
}

func (l *stressTestListener) Accept() (net.Conn, error) {
	var conn net.Conn
	select {
	case <-time.After(l.interval):
		c1, c2 := net.Pipe()
		conn = c1
		go func() {
			mcConn := mc.NewMcConn(c2)
			hs := mc.ServerBoundHandshake{
				ProtocolVersion: 755,
				ServerAddress:   "localhost",
				ServerPort:      25565,
				NextState:       mc.LoginState,
			}
			hsPk := hs.Marshal()
			mcConn.WritePacket(hsPk)
			login := mc.ServerLoginStart{
				Name: mc.String(fmt.Sprint(l.r.Int())),
			}
			loginPk := login.Marshal()
			mcConn.WritePacket(loginPk)
		}()
	case c := <-l.realRequestCh:
		conn = c
	}
	return conn, nil
}

func (l *stressTestListener) Close() error {
	return nil
}

func (l *stressTestListener) Addr() net.Addr {
	return nil
}

func newStressTestListener(cfg config.UltravioletConfig) (net.Listener, error) {
	ln, err := net.Listen("tcp", cfg.ListenTo)
	if err != nil {
		return ln, err
	}
	realReqCh := make(chan net.Conn)
	go func() {
		for {
			conn, err := ln.Accept()
			if err != nil {
				log.Printf("got error from real listener: %v", err)
				continue
			}
			realReqCh <- conn
		}
	}()
	source := rand.NewSource(time.Now().UnixNano())
	random := rand.New(source)
	stressListener := stressTestListener{
		interval:      time.Millisecond,
		realRequestCh: realReqCh,
		r:             random,
	}
	return &stressListener, nil
}
