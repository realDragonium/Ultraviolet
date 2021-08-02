package Ultraviolet

import (
	"flag"
	"log"
	"path/filepath"

	"github.com/realDragonium/Ultraviolet/config"
	"github.com/realDragonium/Ultraviolet/server"
)

var (
	// Isnt this the proper path to put config files into (for execution without docker)
	defaultCfgPath = "/etc/ultraviolet"
)

func main() {
	log.Println("Starting up Alpha-v0.12")
	var (
		cfgDir = flag.String("configs", defaultCfgPath, "`Path` to config directory")
	)
	flag.Parse()

	mainCfgPath := filepath.Join(*cfgDir, "ultraviolet.json")
	mainCfg, err := config.ReadUltravioletConfig(mainCfgPath)
	if err != nil {
		log.Fatalf("Read main config file at '%s' - error: %v", mainCfgPath, err)
	}

	serverCfgsPath := filepath.Join(*cfgDir, "config")
	serverCfgs, err := config.ReadServerConfigs(serverCfgsPath)
	if err != nil {
		log.Fatalf("Something went wrong while reading config files: %v", err)
	}

	server.StartWorkers(mainCfg, serverCfgs)
	log.Printf("Finished starting up")
	select {}
}
