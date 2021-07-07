package proxy

import (
	"log"
	"net"
	"sync"

	"github.com/realDragonium/Ultraviolet/conn"
	"github.com/realDragonium/Ultraviolet/mc"
)

func NewProxy(reqCh chan conn.ConnRequest) Proxy {
	return Proxy{
		reqCh:          reqCh,
		NotifyCh:       make(chan struct{}),
		ShouldNotifyCh: make(chan struct{}),

		closedProxy: make(chan struct{}),
		openedProxy: make(chan struct{}),
		wg:          &sync.WaitGroup{},
	}
}

type Proxy struct {
	reqCh    chan conn.ConnRequest
	NotifyCh chan struct{}

	ShouldNotifyCh chan struct{}

	closedProxy chan struct{}
	openedProxy chan struct{}
	wg          *sync.WaitGroup
}

func (p *Proxy) Serve() {
	go p.manageConnections()
	go p.backend()
}

func (p *Proxy) backend() {
	for {
		request := <-p.reqCh
		switch request.Type {
		case conn.LOGIN:
			somethingElse(request)
			serverConn, err := net.Dial("tcp", "192.168.1.15:25560")
			if err != nil {
				log.Printf("Error while connection to server: %v", err)
				request.Ch <- conn.ConnAnswer{
					Action: conn.CLOSE,
				}
				return
			}
			request.Ch <- conn.ConnAnswer{
				Action:       conn.PROXY,
				ServerConn:   conn.NewMcConn(serverConn),
				NotifyClosed: p.closedProxy,
			}
			p.openedProxy <- struct{}{}
		case conn.STATUS:
			somethingElse(request)
			statusPk := mc.AnotherStatusResponse{
				Name:        "Ultraviolet",
				Protocol:    751,
				Description: "Some broken proxy",
			}.Marshal()
			request.Ch <- conn.ConnAnswer{
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

func somethingElse(a ...interface{}) int {
	return 0
}
