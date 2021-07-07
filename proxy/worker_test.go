package proxy_test

import (
	"bytes"
	"net"
	"strings"
	"testing"
	"time"

	"github.com/pires/go-proxyproto"
	"github.com/realDragonium/Ultraviolet/conn"
	"github.com/realDragonium/Ultraviolet/mc"
	"github.com/realDragonium/Ultraviolet/proxy"
)

var defaultChTimeout time.Duration = 10 * time.Millisecond

func netAddrToIp(addr net.Addr) string {
	return strings.Split(addr.String(), ":")[0]
}

func defaultOfflineStatusPacket() mc.Packet {
	return mc.AnotherStatusResponse{
		Name:        "Ultraviolet-ff",
		Protocol:    755,
		Description: "offline proxy being tested",
	}.Marshal()
}

func defaultOnlineStatusPacket() mc.Packet {
	return mc.AnotherStatusResponse{
		Name:        "Ultraviolet-on",
		Protocol:    755,
		Description: "online proxy being tested",
	}.Marshal()
}

func samePk(expected, received mc.Packet) bool {
	sameID := expected.ID == received.ID
	sameData := bytes.Equal(expected.Data, received.Data)

	return sameID && sameData
}

func unknownServerStatus() mc.Packet {
	return mc.AnotherStatusResponse{
		Name:        "Ultraviolet",
		Protocol:    0,
		Description: "No server found",
	}.Marshal()
}

func setupWorker(proxies []proxy.WorkerServerConfig) chan<- conn.ConnRequest {
	reqCh := make(chan conn.ConnRequest)
	servers := make(map[string]proxy.WorkerServerConfig)
	for _, proxy := range proxies {
		servers[proxy.MainDomain] = proxy
		for _, extraDomain := range proxy.ExtraDomains {
			servers[extraDomain] = proxy
		}
	}
	worker := proxy.NewWorker(reqCh, servers, unknownServerStatus())

	go worker.Work()
	return reqCh
}

func createListener(t *testing.T, addr string) (<-chan net.Conn, <-chan error) {
	connCh := make(chan net.Conn)
	errorCh := make(chan error)
	listener, err := net.Listen("tcp", addr)
	if err != nil {
		t.Fatal(err)
	}
	go func() {
		for {
			conn, err := listener.Accept()
			if err != nil {
				errorCh <- err
			}
			connCh <- conn
		}
	}()
	return connCh, errorCh
}

func TestWorker_CanReceiveRequests(t *testing.T) {
	reqCh := make(chan conn.ConnRequest)
	worker := proxy.NewWorker(reqCh, nil, unknownServerStatus())
	go worker.Work()
	select {
	case reqCh <- conn.ConnRequest{}:
		t.Log("worker has successfully received request")
	case <-time.After(defaultChTimeout):
		t.Error("timed out")
	}
}

func TestStatusUnknownAddr_ReturnDefaultStatus(t *testing.T) {
	servers := []proxy.WorkerServerConfig{}
	reqCh := setupWorker(servers)

	answerCh := make(chan conn.ConnAnswer)
	reqCh <- conn.ConnRequest{
		Type:       conn.STATUS,
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
		if answer.Action != conn.SEND_STATUS {
			t.Errorf("expcted: %v \ngot: %v", conn.SEND_STATUS, answer.Action)
		}
	case <-time.After(defaultChTimeout):
		t.Error("timed out")
	}
}

func TestStatusKnownAddr_ReturnOfflineStatus_WhenServerOffline(t *testing.T) {
	serverAddr := "ultraviolet"
	offlineStatusPk := defaultOfflineStatusPacket()

	servers := []proxy.WorkerServerConfig{{
		MainDomain:    serverAddr,
		OfflineStatus: offlineStatusPk,
		State:         proxy.OFFLINE,
	}}
	reqCh := setupWorker(servers)

	answerCh := make(chan conn.ConnAnswer)
	reqCh <- conn.ConnRequest{
		Type:       conn.STATUS,
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
		if answer.Action != conn.SEND_STATUS {
			t.Errorf("expcted: %v \ngot: %v", conn.SEND_STATUS, answer.Action)
		}
	case <-time.After(defaultChTimeout):
		t.Error("timed out")
	}
}

func TestStatusKnownAddr_ReturnsOnlineStatus_WhenServerOnline(t *testing.T) {
	serverAddr := "ultraviolet"
	onlineStatusPk := defaultOnlineStatusPacket()
	servers := []proxy.WorkerServerConfig{{
		MainDomain:   serverAddr,
		OnlineStatus: onlineStatusPk,
	}}

	reqCh := setupWorker(servers)

	answerCh := make(chan conn.ConnAnswer)
	reqCh <- conn.ConnRequest{
		Type:       conn.STATUS,
		ServerAddr: serverAddr,
		Ch:         answerCh,
	}

	select {
	case answer := <-answerCh:
		t.Log("worker has successfully responded")
		if !samePk(onlineStatusPk, answer.StatusPk) {
			t.Errorf("expcted: %v \ngot: %v", onlineStatusPk, answer.StatusPk)
		}
		if answer.Action != conn.SEND_STATUS {
			t.Errorf("expcted: %v \ngot: %v", conn.SEND_STATUS, answer.Action)
		}
	case <-time.After(defaultChTimeout):
		t.Error("timed out")
	}
}

func TestLoginUnknownAddr_ShouldClose(t *testing.T) {
	servers := []proxy.WorkerServerConfig{}
	reqCh := setupWorker(servers)

	answerCh := make(chan conn.ConnAnswer)
	reqCh <- conn.ConnRequest{
		Type:       conn.LOGIN,
		ServerAddr: "some weird server address",
		Ch:         answerCh,
	}

	select {
	case answer := <-answerCh:
		t.Log("worker has successfully responded")
		if answer.Action != conn.CLOSE {
			t.Errorf("expcted: %v \ngot: %v", conn.CLOSE, answer.Action)
		}
	case <-time.After(defaultChTimeout):
		t.Error("timed out")
	}
}

func TestLoginKnownAddr_Online_ShouldProxy(t *testing.T) {
	serverAddr := "ultraviolet"
	targetAddr := "127.0.0.1:25565"
	servers := []proxy.WorkerServerConfig{{
		MainDomain: serverAddr,
		ProxyTo:    targetAddr,
	}}
	reqCh := setupWorker(servers)

	createListener(t, targetAddr)

	answerCh := make(chan conn.ConnAnswer)
	reqCh <- conn.ConnRequest{
		Type:       conn.LOGIN,
		ServerAddr: serverAddr,
		Ch:         answerCh,
	}

	select {
	case answer := <-answerCh:
		t.Log("worker has successfully responded")
		if answer.Action != conn.PROXY {
			t.Errorf("expcted: %v \ngot: %v", conn.CLOSE, answer.Action)
		}
		err := answer.ServerConn.WritePacket(mc.Packet{Data: []byte{0}})
		if err != nil {
			t.Fatalf("Got an unexpected error: %v", err)
		}
	case <-time.After(defaultChTimeout):
		t.Error("timed out")
	}
}

func TestLoginProxyBind(t *testing.T) {
	serverCfg := proxy.WorkerServerConfig{
		MainDomain: "ultraviolet",
		ProxyTo:    "127.0.0.1:25566",
		ProxyBind:  "127.0.0.2",
	}
	servers := []proxy.WorkerServerConfig{serverCfg}
	reqCh := setupWorker(servers)

	connCh, errorCh := createListener(t, serverCfg.ProxyTo)

	answerCh := make(chan conn.ConnAnswer)
	reqCh <- conn.ConnRequest{
		Type:       conn.LOGIN,
		ServerAddr: serverCfg.MainDomain,
		Ch:         answerCh,
	}

	select {
	case err := <-errorCh:
		t.Fatalf("error while accepting connection: %v", err)
	case conn := <-connCh:
		t.Log("connection has been created")
		if netAddrToIp(conn.RemoteAddr()) != serverCfg.ProxyBind {
			t.Errorf("expcted: %v \ngot: %v", serverCfg.ProxyBind, netAddrToIp(conn.RemoteAddr()))
		}
	case <-time.After(defaultChTimeout):
		t.Error("timed out")
	}
}

func TestLoginProxyProtocol(t *testing.T) {
	playerAddr := &net.TCPAddr{
		IP:   net.ParseIP("187.34.26.123"),
		Port: 49473,
	}
	serverCfg := proxy.WorkerServerConfig{
		MainDomain:        "ultraviolet",
		ProxyTo:           "127.0.0.1:25567",
		ProxyBind:         "127.0.0.2",
		SendProxyProtocol: true,
	}
	servers := []proxy.WorkerServerConfig{serverCfg}
	reqCh := setupWorker(servers)

	listener, err := net.Listen("tcp", serverCfg.ProxyTo)
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

	answerCh := make(chan conn.ConnAnswer)
	reqCh <- conn.ConnRequest{
		Type:       conn.LOGIN,
		ServerAddr: serverCfg.MainDomain,
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

func TestLoginKnownAddr_Offline_ShouldDisconnect(t *testing.T) {
	disconPacket := mc.ClientBoundDisconnect{
		Reason: "Some disconnect message right here",
	}.Marshal()
	serverCfg := proxy.WorkerServerConfig{
		MainDomain:       "ultraviolet",
		State:            proxy.OFFLINE,
		DisconnectPacket: disconPacket,
	}
	servers := []proxy.WorkerServerConfig{serverCfg}
	reqCh := setupWorker(servers)

	answerCh := make(chan conn.ConnAnswer)
	reqCh <- conn.ConnRequest{
		Type:       conn.LOGIN,
		ServerAddr: serverCfg.MainDomain,
		Ch:         answerCh,
	}

	select {
	case answer := <-answerCh:
		t.Log("worker has successfully responded")
		if answer.Action != conn.DISCONNECT {
			t.Errorf("expcted: %v got: %v", conn.DISCONNECT, answer.Action)
		}
		if !samePk(disconPacket, answer.DisconMessage) {
			t.Errorf("expcted: %v \ngot: %v", serverCfg.DisconnectPacket, answer.DisconMessage)
		}
	case <-time.After(defaultChTimeout):
		t.Error("timed out")
	}
}
