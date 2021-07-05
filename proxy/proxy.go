package proxy

import (
	"log"
	"net"
	"sync"

	"github.com/realDragonium/UltraViolet/conn"
	"github.com/realDragonium/UltraViolet/mc"
)

func NewProxy(netListener net.Listener) Proxy {
	return Proxy{
		NotifyCh:       make(chan struct{}),
		ShouldNotifyCh: make(chan struct{}),

		listener:    conn.NewListener(netListener),
		closedProxy: make(chan struct{}),
		openedProxy: make(chan struct{}),
		wg:          &sync.WaitGroup{},
	}
}

type Proxy struct {
	listener conn.Listener
	NotifyCh chan struct{}

	ShouldNotifyCh chan struct{}

	closedProxy chan struct{}
	openedProxy chan struct{}
	wg          *sync.WaitGroup
}

func (p *Proxy) Serve() {
	go p.listener.Serve()
	go p.manageConnections()
	go p.backend()
}

func (p *Proxy) backend() {
	for {
		select {
		case request := <-p.listener.LoginReqCh:
			somethingElse(request)
			serverConn, err := net.Dial("tcp", "192.168.1.15:25560")
			if err != nil {
				log.Printf("Error while connection to server: %v", err)
				p.listener.LoginAnsCh <- conn.LoginAnswer{
					Action: conn.CLOSE,
				}
				return
			}
			p.listener.LoginAnsCh <- conn.LoginAnswer{
				Action:       conn.PROXY,
				ServerConn:   serverConn,
				NotifyClosed: p.closedProxy,
			}
			p.openedProxy <- struct{}{}
		case request := <-p.listener.StatusReqCh:
			somethingElse(request)
			statusPk := mc.AnotherStatusResponse{
				Name:        "UltraViolet",
				Protocol:    751,
				Description: "Some broken proxy",
			}.Marshal()
			p.listener.StatusAnsCh <- conn.StatusAnswer{
				Action:       conn.SEND_STATUS,
				StatusPk:     statusPk,
				NotifyClosed: p.closedProxy,
			}

		}
	}
}

func (p *Proxy) manageConnections() {
	go func() {
		<-p.ShouldNotifyCh
		p.wg.Wait()
		p.NotifyCh <- struct{}{}
	}()

	for {
		select {
		case <-p.openedProxy:
			p.wg.Add(1)
		case <-p.closedProxy:
			p.wg.Done()
		}
	}
}

func somethingElse(a ...interface{}) {

}
