package main

import (
	"flag"
	"fmt"
	"log"
	"os"
	"os/signal"
	"syscall"

	"github.com/cloudflare/tableflip"
	"github.com/realDragonium/UltraViolet/proxy"
)

func main() {
	fmt.Println("Starting up")
	var (
		pidFile = flag.String("pid-file", "/run/ultraviolet.pid", "`Path` to pid file")
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

	ln, err := upg.Listen("tcp", ":25565")
	if err != nil {
		log.Fatalln("Can't listen:", err)
	}
	defer ln.Close()

	p := proxy.NewProxy(ln)
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
