package proxy_test

import (
	"net"
	"testing"
	"time"

	"github.com/pires/go-proxyproto"
	"github.com/realDragonium/Ultraviolet/mc"
	"github.com/realDragonium/Ultraviolet/proxy"
)

func setupAdvWorker(servers map[string]proxy.WorkerServerConfig) chan<- proxy.McRequest {
	reqCh := make(chan proxy.McRequest)
	worker := proxy.NewWorker(reqCh, servers, unknownServerStatus())

	go worker.Work()
	return reqCh
}

func TestAdvWorker_CanReceiveRequests(t *testing.T) {
	reqCh := make(chan proxy.McRequest)
	worker := proxy.NewAdvWorker(reqCh, nil, unknownServerStatus())
	go worker.Work()
	select {
	case reqCh <- proxy.McRequest{}:
		t.Log("worker has successfully received request")
	case <-time.After(defaultChTimeout):
		t.Error("timed out")
	}
}

func TestAdvStatusUnknownAddr_ReturnDefaultStatus(t *testing.T) {
	servers := make(map[string]proxy.WorkerServerConfig)
	reqCh := setupAdvWorker(servers)

	answerCh := make(chan proxy.McAnswer)
	reqCh <- proxy.McRequest{
		Type:       proxy.STATUS,
		ServerAddr: "some weird server address",
		Ch:         answerCh,
	}
	defaultStatusPk := unknownServerStatus()
	select {
	case answer := <-answerCh:
		t.Log("worker has successfully responded")
		if !samePk(defaultStatusPk, answer.StatusPk) {
			defaultStatus, _ := mc.UnmarshalClientBoundResponse(defaultStatusPk)
			receivedStatus, _ := mc.UnmarshalClientBoundResponse(answer.StatusPk)
			t.Errorf("expcted: %v \ngot: %v", defaultStatus, receivedStatus)
		}
		if answer.Action != proxy.SEND_STATUS {
			t.Errorf("expcted: %v \ngot: %v", proxy.SEND_STATUS, answer.Action)
		}
	case <-time.After(defaultChTimeout):
		t.Error("timed out")
	}
}

func TestAdvStatusKnownAddr_ReturnOfflineStatus_WhenServerOffline(t *testing.T) {
	serverAddr := "ultraviolet"
	offlineStatusPk := defaultOfflineStatusPacket()
	servers := make(map[string]proxy.WorkerServerConfig)
	servers[serverAddr] = proxy.WorkerServerConfig{
		OfflineStatus: offlineStatusPk,
		State:         proxy.OFFLINE,
	}
	reqCh := setupAdvWorker(servers)

	answerCh := make(chan proxy.McAnswer)
	reqCh <- proxy.McRequest{
		Type:       proxy.STATUS,
		ServerAddr: serverAddr,
		Ch:         answerCh,
	}

	select {
	case answer := <-answerCh:
		t.Log("worker has successfully responded")
		if !samePk(offlineStatusPk, answer.StatusPk) {
			offlineStatus, _ := mc.UnmarshalClientBoundResponse(offlineStatusPk)
			receivedStatus, _ := mc.UnmarshalClientBoundResponse(answer.StatusPk)
			t.Errorf("expcted: %v \ngot: %v", offlineStatus, receivedStatus)
		}
		if answer.Action != proxy.SEND_STATUS {
			t.Errorf("expcted: %v \ngot: %v", proxy.SEND_STATUS, answer.Action)
		}
	case <-time.After(defaultChTimeout):
		t.Error("timed out")
	}
}

// func TestAdvStatusKnownAddr_ReturnsOnlineStatus_WhenServerOnline(t *testing.T) {
// 	serverAddr := "ultraviolet"
// 	onlineStatusPk := defaultOnlineStatusPacket()
// 	servers := make(map[string]proxy.WorkerServerConfig)
// 	servers[serverAddr] = proxy.WorkerServerConfig{
// 		OnlineStatus: onlineStatusPk,
// 	}

// 	reqCh := setupAdvWorker(servers)

// 	answerCh := make(chan proxy.ConnAnswer)
// 	reqCh <- proxy.ConnRequest{
// 		Type:       proxy.STATUS,
// 		ServerAddr: serverAddr,
// 		Ch:         answerCh,
// 	}

// 	select {
// 	case answer := <-answerCh:
// 		t.Log("worker has successfully responded")
// 		if !samePk(onlineStatusPk, answer.StatusPk) {
// 			t.Errorf("expcted: %v \ngot: %v", onlineStatusPk, answer.StatusPk)
// 		}
// 		if answer.Action != proxy.SEND_STATUS {
// 			t.Errorf("expcted: %v \ngot: %v", proxy.SEND_STATUS, answer.Action)
// 		}
// 	case <-time.After(defaultChTimeout):
// 		t.Error("timed out")
// 	}
// }

func TestAdvLoginUnknownAddr_ShouldClose(t *testing.T) {
	servers := make(map[string]proxy.WorkerServerConfig)
	reqCh := setupAdvWorker(servers)

	answerCh := make(chan proxy.McAnswer)
	reqCh <- proxy.McRequest{
		Type:       proxy.LOGIN,
		ServerAddr: "some weird server address",
		Ch:         answerCh,
	}

	select {
	case answer := <-answerCh:
		t.Log("worker has successfully responded")
		if answer.Action != proxy.CLOSE {
			t.Errorf("expcted: %v \ngot: %v", proxy.CLOSE, answer.Action)
		}
	case <-time.After(defaultChTimeout):
		t.Error("timed out")
	}
}

func TestAdvLoginKnownAddr_Online_ShouldProxy(t *testing.T) {
	serverAddr := "ultraviolet"
	targetAddr := "127.0.0.1:25665"
	servers := make(map[string]proxy.WorkerServerConfig)
	servers[serverAddr] = proxy.WorkerServerConfig{
		ProxyTo: targetAddr,
	}
	reqCh := setupAdvWorker(servers)

	createListener(t, targetAddr)

	answerCh := make(chan proxy.McAnswer)
	reqCh <- proxy.McRequest{
		Type:       proxy.LOGIN,
		ServerAddr: serverAddr,
		Ch:         answerCh,
	}

	select {
	case answer := <-answerCh:
		t.Log("worker has successfully responded")
		if answer.Action != proxy.PROXY {
			t.Fatalf("expcted: %v \ngot: %v", proxy.CLOSE, answer.Action)
			t.FailNow()
		}
		err := answer.ServerConn.WritePacket(mc.Packet{Data: []byte{0}})
		if err != nil {
			t.Fatalf("Got an unexpected error: %v", err)
		}
	case <-time.After(defaultChTimeout):
		t.Error("timed out")
	}
}

func TestAdvLoginProxyBind(t *testing.T) {
	serverAddr := "ultraviolet"
	proxyTo := "127.0.0.1:25666"
	proxyBind := "127.0.0.2"
	servers := make(map[string]proxy.WorkerServerConfig)
	servers[serverAddr] = proxy.WorkerServerConfig{
		ProxyTo:   proxyTo,
		ProxyBind: proxyBind,
	}
	reqCh := setupAdvWorker(servers)

	connCh, errorCh := createListener(t, proxyTo)

	answerCh := make(chan proxy.McAnswer)
	reqCh <- proxy.McRequest{
		Type:       proxy.LOGIN,
		ServerAddr: serverAddr,
		Ch:         answerCh,
	}

	select {
	case err := <-errorCh:
		t.Fatalf("error while accepting connection: %v", err)
	case conn := <-connCh:
		t.Log("connection has been created")
		if netAddrToIp(conn.RemoteAddr()) != proxyBind {
			t.Errorf("expcted: %v \ngot: %v", proxyBind, netAddrToIp(conn.RemoteAddr()))
		}
	case <-time.After(defaultChTimeout):
		t.Error("timed out")
	}
}

func TestAdvLoginProxyProtocol(t *testing.T) {
	serverAddr := "ultraviolet"
	proxyTo := "127.0.0.1:25667"
	servers := make(map[string]proxy.WorkerServerConfig)
	servers[serverAddr] = proxy.WorkerServerConfig{
		ProxyTo:           proxyTo,
		SendProxyProtocol: true,
	}
	playerAddr := &net.TCPAddr{
		IP:   net.ParseIP("187.34.26.123"),
		Port: 49473,
	}
	reqCh := setupAdvWorker(servers)

	listener, err := net.Listen("tcp", proxyTo)
	if err != nil {
		t.Fatal(err)
	}

	proxyListener := &proxyproto.Listener{Listener: listener}
	connCh := make(chan net.Conn)
	errorCh := make(chan error)
	go func() {
		for {
			conn, err := proxyListener.Accept()
			if err != nil {
				errorCh <- err
			}
			connCh <- conn
		}
	}()

	answerCh := make(chan proxy.McAnswer)
	reqCh <- proxy.McRequest{
		Type:       proxy.LOGIN,
		ServerAddr: serverAddr,
		Addr:       playerAddr,
		Ch:         answerCh,
	}

	select {
	case err := <-errorCh:
		t.Fatalf("error while accepting connection: %v", err)
	case conn := <-connCh:
		t.Log("connection has been created")
		if conn.RemoteAddr().String() != playerAddr.String() {
			t.Errorf("expcted: %v \ngot: %v", playerAddr, conn.RemoteAddr())
		}
	case <-time.After(defaultChTimeout):
		t.Error("timed out")
	}
}

func TestAdvLoginKnownAddr_Offline_ShouldDisconnect(t *testing.T) {
	serverAddr := "ultraviolet"
	disconPacket := mc.ClientBoundDisconnect{
		Reason: "Some disconnect message right here",
	}.Marshal()
	servers := make(map[string]proxy.WorkerServerConfig)
	servers[serverAddr] = proxy.WorkerServerConfig{
		State:            proxy.OFFLINE,
		DisconnectPacket: disconPacket,
	}
	reqCh := setupAdvWorker(servers)

	answerCh := make(chan proxy.McAnswer)
	reqCh <- proxy.McRequest{
		Type:       proxy.LOGIN,
		ServerAddr: serverAddr,
		Ch:         answerCh,
	}

	select {
	case answer := <-answerCh:
		t.Log("worker has successfully responded")
		if answer.Action != proxy.DISCONNECT {
			t.Errorf("expcted: %v got: %v", proxy.DISCONNECT, answer.Action)
		}
		if !samePk(disconPacket, answer.DisconMessage) {
			t.Errorf("expcted: %v \ngot: %v", disconPacket, answer.DisconMessage)
		}
	case <-time.After(defaultChTimeout):
		t.Error("timed out")
	}
}

func TestAdvStatusKnownAddr_ProxyConnection_WhenServerOnline(t *testing.T) {
	serverAddr := "ultraviolet"
	targetAddr := "127.0.0.1:25668"
	servers := make(map[string]proxy.WorkerServerConfig)
	servers[serverAddr] = proxy.WorkerServerConfig{
		ProxyTo: targetAddr,
	}
	reqCh := setupAdvWorker(servers)

	createListener(t, targetAddr)

	answerCh := make(chan proxy.McAnswer)
	reqCh <- proxy.McRequest{
		Type:       proxy.STATUS,
		ServerAddr: serverAddr,
		Ch:         answerCh,
	}

	select {
	case answer := <-answerCh:
		t.Log("worker has successfully responded")
		if answer.Action != proxy.PROXY {
			t.Fatalf("expcted: %v \ngot: %v", proxy.PROXY, answer.Action)
		}
		err := answer.ServerConn.WritePacket(mc.Packet{Data: []byte{0}})
		if err != nil {
			t.Fatalf("Got an unexpected error: %v", err)
		}
	case <-time.After(defaultChTimeout):
		t.Error("timed out")
	}
}
