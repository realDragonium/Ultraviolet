package main

import (
	"flag"
	"fmt"
	"log"
	"os"
	"os/signal"
	"syscall"

	"github.com/cloudflare/tableflip"
	"github.com/realDragonium/Ultraviolet/config"
	"github.com/realDragonium/Ultraviolet/proxy"
)

func main() {
	log.Println("Starting up")
	var (
		pidFile     = flag.String("pid-file", "/run/ultraviolet.pid", "`Path` to pid file")
		mainCfgPath = flag.String("config", "", "`Path` to main config file")
	)
	flag.Parse()
	log.SetPrefix(fmt.Sprintf("%d ", os.Getpid()))

	upg, err := tableflip.New(tableflip.Options{
		// UpgradeTimeout: time.Duration(24 * time.Hour),
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

	cfg, err := config.ReadUltravioletConfig(*mainCfgPath)
	if err != nil {
		log.Fatalln("Can't listen:", err)
	}

	ln, err := upg.Listen("tcp", cfg.ListenTo)
	if err != nil {
		log.Fatalln("Can't listen:", err)
	}
	defer ln.Close()

	reqCh := make(chan proxy.McRequest)
	go proxy.Serve(ln, reqCh)

	p := proxy.NewProxy(reqCh)
	p.Serve()

	log.Printf("ready")
	if err := upg.Ready(); err != nil {
		panic(err)
	}
	log.Println("Upgrade in process...")
	<-upg.Exit()

	p.ShouldNotifyCh <- struct{}{}
	log.Println("Waiting for all open connections to close before shutting down")
	<-p.NotifyCh

	log.Println("Shutting down")
}
