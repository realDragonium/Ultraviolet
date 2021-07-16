package main

import (
	"flag"
	"fmt"
	"log"
	"os"
	"os/signal"
	"path/filepath"
	"syscall"

	"github.com/cloudflare/tableflip"
	"github.com/realDragonium/Ultraviolet/config"
	"github.com/realDragonium/Ultraviolet/proxy"
)

var (
	// Isnt this the proper path to put config files into (for execution without docker)
	defaultCfgPath            = "/etc/ultraviolet"
	defaultServerCfgPath      = filepath.Join(defaultCfgPath, "config")
	defaultUltravioletCfgPath = filepath.Join(defaultCfgPath, "ultraviolet.json")
)

func main() {
	log.Printf("Starting up Alpha-v%d", 6)
	var (
		pidFile        = flag.String("pid-file", "/run/ultraviolet.pid", "`Path` to pid file")
		mainCfgPath    = flag.String("config", defaultUltravioletCfgPath, "`Path` to main config file")
		serverCfgsPath = flag.String("server-configs", defaultServerCfgPath, "`Path` to server config files")
	)
	flag.Parse()

	mainCfg, err := config.ReadUltravioletConfig(*mainCfgPath)
	if err != nil {
		log.Fatalf("Read main config file at '%s' - error: %v", *mainCfgPath, err)
	}
	serverCfgs, err := config.ReadServerConfigs(*serverCfgsPath)
	if err != nil {
		log.Fatalf("Something went wrong while reading config files: %v", err)
	}
	reqCh := make(chan proxy.McRequest)
	gateway := proxy.NewGateway()
	gateway.StartWorkers(mainCfg, serverCfgs, reqCh)

	log.SetPrefix(fmt.Sprintf("%d ", os.Getpid()))
	upg, err := tableflip.New(tableflip.Options{
		PIDFile: *pidFile,
	})
	if err != nil {
		panic(err)
	}
	defer upg.Stop()
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

	ln, err := upg.Listen("tcp", mainCfg.ListenTo)
	if err != nil {
		log.Fatalf("Can't listen: %v", err)
	}
	defer ln.Close()
	go proxy.ServeListener(ln, reqCh)

	log.Printf("Finished starting up")
	if err := upg.Ready(); err != nil {
		panic(err)
	}
	<-upg.Exit()
	log.Println("Waiting for all open connections to close before shutting down")
	gateway.Shutdown()
	log.Println("Shutting down")
}
