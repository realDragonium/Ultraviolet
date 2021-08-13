package ultravioletcmd

import (
	"flag"
	"fmt"
	"io/ioutil"
	"log"
	"net/http"
	"os"

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
		log.Println("Starting up Alpha-v0.13")
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
