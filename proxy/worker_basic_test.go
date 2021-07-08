package proxy_test

import (
	"bytes"
	"net"
	"strings"
	"testing"
	"time"

	"github.com/pires/go-proxyproto"
	"github.com/realDragonium/Ultraviolet/mc"
	"github.com/realDragonium/Ultraviolet/proxy"
)

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

func setupBasicWorker(servers map[string]proxy.WorkerServerConfig) chan<- proxy.McRequest {
	reqCh := make(chan proxy.McRequest)
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
	reqCh := make(chan proxy.McRequest)
	worker := proxy.NewWorker(reqCh, nil, unknownServerStatus())
	go worker.Work()
	select {
	case reqCh <- proxy.McRequest{}:
		t.Log("worker has successfully received request")
	case <-time.After(defaultChTimeout):
		t.Error("timed out")
	}
}

func TestStatusUnknownAddr_ReturnDefaultStatus(t *testing.T) {
	servers := make(map[string]proxy.WorkerServerConfig)
	reqCh := setupBasicWorker(servers)

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

func TestStatusKnownAddr_ReturnOfflineStatus_WhenServerOffline(t *testing.T) {
	serverAddr := "ultraviolet"
	offlineStatusPk := defaultOfflineStatusPacket()
	servers := make(map[string]proxy.WorkerServerConfig)
	servers[serverAddr] = proxy.WorkerServerConfig{
		OfflineStatus: offlineStatusPk,
		State:         proxy.OFFLINE,
	}
	reqCh := setupBasicWorker(servers)

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

// func TestStatusKnownAddr_ReturnsOnlineStatus_WhenServerOnline(t *testing.T) {
// 	serverAddr := "ultraviolet"
// 	onlineStatusPk := defaultOnlineStatusPacket()
// 	servers := make(map[string]proxy.WorkerServerConfig)
// 	servers[serverAddr] = proxy.WorkerServerConfig{
// 		OnlineStatus: onlineStatusPk,
// 	}

// 	reqCh := setupBasicWorker(servers)

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

func TestLoginUnknownAddr_ShouldClose(t *testing.T) {
	servers := make(map[string]proxy.WorkerServerConfig)
	reqCh := setupBasicWorker(servers)

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

func TestLoginKnownAddr_Online_ShouldProxy(t *testing.T) {
	serverAddr := "ultraviolet"
	targetAddr := "127.0.0.1:25565"
	servers := make(map[string]proxy.WorkerServerConfig)
	servers[serverAddr] = proxy.WorkerServerConfig{
		ProxyTo: targetAddr,
	}
	reqCh := setupBasicWorker(servers)

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

func TestLoginProxyBind(t *testing.T) {
	serverAddr := "ultraviolet"
	proxyTo := "127.0.0.1:25566"
	proxyBind := "127.0.0.2"
	servers := make(map[string]proxy.WorkerServerConfig)
	servers[serverAddr] = proxy.WorkerServerConfig{
		ProxyTo:   proxyTo,
		ProxyBind: proxyBind,
	}
	reqCh := setupBasicWorker(servers)

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

func TestLoginProxyProtocol(t *testing.T) {
	serverAddr := "ultraviolet"
	proxyTo := "127.0.0.1:25567"
	servers := make(map[string]proxy.WorkerServerConfig)
	servers[serverAddr] = proxy.WorkerServerConfig{
		ProxyTo:           proxyTo,
		SendProxyProtocol: true,
	}
	playerAddr := &net.TCPAddr{
		IP:   net.ParseIP("187.34.26.123"),
		Port: 49473,
	}
	reqCh := setupBasicWorker(servers)

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

func TestLoginKnownAddr_Offline_ShouldDisconnect(t *testing.T) {
	serverAddr := "ultraviolet"
	disconPacket := mc.ClientBoundDisconnect{
		Reason: "Some disconnect message right here",
	}.Marshal()
	servers := make(map[string]proxy.WorkerServerConfig)
	servers[serverAddr] = proxy.WorkerServerConfig{
		State:            proxy.OFFLINE,
		DisconnectPacket: disconPacket,
	}
	reqCh := setupBasicWorker(servers)

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

func TestStatusKnownAddr_ProxyConnection_WhenServerOnline(t *testing.T) {
	serverAddr := "ultraviolet"
	targetAddr := "127.0.0.1:25568"
	servers := make(map[string]proxy.WorkerServerConfig)
	servers[serverAddr] = proxy.WorkerServerConfig{
		ProxyTo: targetAddr,
	}
	reqCh := setupBasicWorker(servers)

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

func TestProxyManyRequests_WillRateLimit(t *testing.T) {
	rateLimit := 3
	rateLimitDuration := 1 * time.Minute
	serverAddr := "ultraviolet"
	targetAddr := "127.0.0.1:25569"
	servers := make(map[string]proxy.WorkerServerConfig)
	servers[serverAddr] = proxy.WorkerServerConfig{
		ProxyTo:           targetAddr,
		RateLimit:         rateLimit,
		RateLimitDuration: rateLimitDuration,
	}
	reqCh := setupBasicWorker(servers)

	connCh, _ := createListener(t, targetAddr)
	go func() {
		for {
			<-connCh
		}
	}()

	sendRequest := func() chan proxy.McAnswer {
		answerCh := make(chan proxy.McAnswer)
		reqCh <- proxy.McRequest{
			Type:       proxy.STATUS,
			ServerAddr: serverAddr,
			Ch:         answerCh,
		}
		return answerCh
	}

	for i := 0; i < rateLimit; i++ {
		ch := sendRequest()
		go func(ch chan proxy.McAnswer) {
			<-ch
		}(ch)
	}

	answerCh := sendRequest()

	select {
	case answer := <-answerCh:
		t.Log("worker has successfully responded")
		if answer.Action != proxy.CLOSE {
			t.Fatalf("expcted: %v \ngot: %v", proxy.CLOSE, answer.Action)
		}
	case <-time.After(defaultChTimeout):
		t.Error("timed out")
	}
}

func TestProxyRateLimited_WillAllowNewConn_AfterDurationEnded(t *testing.T) {
	rateLimit := 1
	rateLimitDuration := 10 * time.Millisecond
	serverAddr := "ultraviolet"
	targetAddr := "127.0.0.1:25570"
	servers := make(map[string]proxy.WorkerServerConfig)
	servers[serverAddr] = proxy.WorkerServerConfig{
		ProxyTo:           targetAddr,
		RateLimit:         rateLimit,
		RateLimitDuration: rateLimitDuration,
	}
	reqCh := setupBasicWorker(servers)

	connCh, _ := createListener(t, targetAddr)
	go func() {
		for {
			<-connCh
		}
	}()

	sendRequest := func() chan proxy.McAnswer {
		answerCh := make(chan proxy.McAnswer)
		reqCh <- proxy.McRequest{
			Type:       proxy.STATUS,
			ServerAddr: serverAddr,
			Ch:         answerCh,
		}
		return answerCh
	}

	for i := 0; i < rateLimit; i++ {
		ch := sendRequest()
		go func(ch chan proxy.McAnswer) {
			<-ch
		}(ch)
	}

	time.Sleep(rateLimitDuration)
	answerCh := sendRequest()

	select {
	case answer := <-answerCh:
		t.Log("worker has successfully responded")
		if answer.Action != proxy.PROXY {
			t.Fatalf("expcted: %v \ngot: %v", proxy.PROXY, answer.Action)
		}
	case <-time.After(defaultChTimeout):
		t.Error("timed out")
	}
}
