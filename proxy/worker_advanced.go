package proxy

import (
	"github.com/realDragonium/Ultraviolet/mc"
)

type Worker interface {
	Work()
}

type AdvWorkerServerConfig struct {
	State ServerState

	OfflineStatus    mc.Packet
	DisconnectPacket mc.Packet

	ProxyTo           string
	ProxyBind         string
	SendProxyProtocol bool

	ConnLimitBackend int
}

func NewAdvWorker(req chan McRequest, proxies map[string]WorkerServerConfig, defaultStatus mc.Packet) Worker {
	stateCh := make(chan StateRequest)
	stateWorker := NewStateWorker(stateCh, proxies)
	go stateWorker.Work()

	statusCh := make(chan StatusRequest)
	statusWorker := NewStatusWorker(statusCh, proxies)
	go statusWorker.Work()

	connCh := make(chan ConnRequest)
	connWorker := NewConnWorker(connCh, proxies)
	go connWorker.Work()

	return AdvancedWorker{
		reqCh:         req,
		defaultStatus: defaultStatus,
		servers:       proxies,

		stateCh: stateCh,
	}
}

type AdvancedWorker struct {
	reqCh         chan McRequest
	defaultStatus mc.Packet
	servers       map[string]WorkerServerConfig

	stateCh  chan StateRequest
	statusCh chan StatusRequest
	connCh   chan ConnRequest
}

func (w AdvancedWorker) Work() {
	for {
		request := <-w.reqCh
		cfg, ok := w.servers[request.ServerAddr]
		if !ok {
			//Unknown server address
			switch request.Type {
			case STATUS:
				request.Ch <- McAnswer{
					Action:   SEND_STATUS,
					StatusPk: w.defaultStatus,
				}
			case LOGIN:
				request.Ch <- McAnswer{
					Action: CLOSE,
				}
			}
			return
		}
		stateAnswerCh := make(chan StateServerData)
		w.stateCh <- StateRequest{
			serverId: request.ServerAddr,
			answerCh: stateAnswerCh,
		}
		stateData := <-stateAnswerCh

		switch request.Type {
		case STATUS:
			if stateData.state == ONLINE {
				connAnswerCh := make(chan ConnAnswer)
				w.connCh <- ConnRequest{
					clientAddr:        request.Addr,
					sendProxyProtocol: cfg.SendProxyProtocol,
					serverId:          request.ServerAddr,
					answerCh:          connAnswerCh,
				}
				connData := <-connAnswerCh
				request.Ch <- McAnswer{
					Action:     PROXY,
					ServerConn: connData.ServerConn,
				}
				return
			}
			statusAnswerCh := make(chan mc.Packet)
			w.statusCh <- StatusRequest{
				serverId: request.ServerAddr,
				state:    stateData.state,
				answerCh: statusAnswerCh,
			}
			statusPk := <-statusAnswerCh
			request.Ch <- McAnswer{
				Action:   SEND_STATUS,
				StatusPk: statusPk,
			}
		case LOGIN:
			if stateData.state == OFFLINE {
				request.Ch <- McAnswer{
					Action:        DISCONNECT,
					DisconMessage: cfg.DisconnectPacket,
				}
				return
			}
			connAnswerCh := make(chan ConnAnswer)
			w.connCh <- ConnRequest{
				clientAddr:        request.Addr,
				sendProxyProtocol: cfg.SendProxyProtocol,
				serverId:          request.ServerAddr,
				answerCh:          connAnswerCh,
			}
			connData := <-connAnswerCh
			request.Ch <- McAnswer{
				Action:     PROXY,
				ServerConn: connData.ServerConn,
			}
		}
	}
}
