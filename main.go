package main

import (
	"flag"
	"log"
	"net"
	"path/filepath"

	"github.com/realDragonium/Ultraviolet/config"
	"github.com/realDragonium/Ultraviolet/old_proxy"
)

var (
	// Isnt this the proper path to put config files into (for execution without docker)
	defaultCfgPath            = "/etc/ultraviolet"
	defaultServerCfgPath      = filepath.Join(defaultCfgPath, "config")
	defaultUltravioletCfgPath = filepath.Join(defaultCfgPath, "ultraviolet.json")
)

func main() {
	log.Printf("Starting up Alpha-v0.%d", 9)
	var (
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
	reqCh := make(chan old_proxy.McRequest)
	gateway := old_proxy.NewGateway()
	gateway.StartWorkers(mainCfg, serverCfgs, reqCh)
	ln, err := net.Listen("tcp", mainCfg.ListenTo)
	if err != nil {
		log.Fatalf("Can't listen: %v", err)
	}
	defer ln.Close()
	go old_proxy.ServeListener(ln, reqCh)

	log.Printf("Finished starting up")
	select {}
}
