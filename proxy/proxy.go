package proxy

import (
	"net"

	"github.com/realDragonium/UltraViolet/conn"
	"github.com/realDragonium/UltraViolet/mc"
)

func Something() {
	netListener, _ := net.Listen("tcp", ":25565")
	listener := conn.NewListener(netListener)

	for {
		select {
		case request := <-listener.LoginReqCh:
			somethingElse(request)
		case request := <-listener.StatusReqCh:
			somethingElse(request)
			statusPk := mc.AnotherStatusResponse{
				Name:        "UltraViolet",
				Protocol:    751,
				Description: "Some broken proxy",
			}.Marshal()
			statusAnswer := conn.StatusAnswer{
				Proxy:    false,
				StatusPk: statusPk,
			}
			listener.StatusAnsCh <- statusAnswer
		}
	}
}

func somethingElse(a ...interface{}) {

}
