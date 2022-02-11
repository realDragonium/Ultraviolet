package cmd

import (
	"flag"
	"fmt"
	"io/ioutil"
	"log"
	"net/http"
	"os"

	"github.com/realDragonium/Ultraviolet/config"
	"github.com/realDragonium/Ultraviolet/worker"
)

var (
	defaultCfgPath = "/etc/ultraviolet"
	configPath     string
	uvVersion      = "(unknown version)"
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

func runProxy(configPath string) error {
	return worker.RunProxy(configPath, uvVersion)
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
