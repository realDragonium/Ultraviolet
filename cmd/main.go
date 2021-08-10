package ultravioletcmd

import (
	"flag"
	"fmt"
	"log"
	"net/http"
	"os"
	"path/filepath"

	ultraviolet "github.com/realDragonium/Ultraviolet"
	"github.com/realDragonium/Ultraviolet/config"
)

var (
	defaultCfgPath = "/etc/ultraviolet"
	configPath     string
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
		log.Println("Starting up Alpha-v0.12.1")
		ultraviolet.RunProxy(configPath)
	case "reload":
		err := callReloadAPI(configPath)
		if err != nil {
			log.Fatalf("got error: %v ", err)
		}
		log.Println("Finished reloading")
	}
}

func callReloadAPI(configPath string) error {
	mainCfgPath := filepath.Join(configPath, "ultraviolet.json")
	mainCfg, err := config.ReadUltravioletConfig(mainCfgPath)
	if err != nil {
		log.Fatalf("Read main config file at '%s' - error: %v", mainCfgPath, err)
	}

	url := fmt.Sprintf("http://%s/reload", mainCfg.APIBind)
	resp, err := http.Get(url)
	if err != nil {
		return err
	}
	resp.Body.Close()
	return nil
}
