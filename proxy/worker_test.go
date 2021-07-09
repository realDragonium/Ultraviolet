package proxy_test

import (
	"bytes"
	"errors"
	"fmt"
	"log"
	"net"
	"strings"
	"sync"
	"testing"
	"time"

	"github.com/pires/go-proxyproto"
	"github.com/realDragonium/Ultraviolet/mc"
	"github.com/realDragonium/Ultraviolet/proxy"
)

var ErrNoResponse = errors.New("there was no response from worker")

func netAddrToIp(addr net.Addr) string {
	return strings.Split(addr.String(), ":")[0]
}

func defaultOfflineStatusPacket() mc.Packet {
	return defaultOfflineStatus().Marshal()
}

func defaultOfflineStatus() mc.AnotherStatusResponse {
	return mc.AnotherStatusResponse{
		Name:        "Ultraviolet-ff",
		Protocol:    755,
		Description: "offline proxy being tested",
	}
}

var port *int16
var portLock sync.Mutex = sync.Mutex{}

// To make sure every test gets its own unique port
func testAddr() string {
	portLock.Lock()
	defer portLock.Unlock()
	if port == nil {
		port = new(int16)
		*port = 25500
	}
	addr := fmt.Sprintf("127.0.0.1:%d", *port)
	*port++
	return addr
}

func samePk(expected, received mc.Packet) bool {
	sameID := expected.ID == received.ID
	sameData := bytes.Equal(expected.Data, received.Data)

	return sameID && sameData
}

func unknownServerStatusPk() mc.Packet {
	return unknownServerStatus().Marshal()
}

func unknownServerStatus() mc.AnotherStatusResponse {
	return mc.AnotherStatusResponse{
		Name:        "Ultraviolet",
		Protocol:    0,
		Description: "No server found",
	}
}

func setupBasicWorkers(servers map[string]proxy.WorkerServerConfig) chan<- proxy.McRequest {
	reqCh := make(chan proxy.McRequest)
	workerCfg := proxy.NewWorkerConfig(reqCh, servers, unknownServerStatusPk())
	statusCh := make(chan proxy.StatusRequest)
	connCh := make(chan proxy.ConnRequest)
	worker := proxy.NewBasicWorker(workerCfg, statusCh, connCh)
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

func TestWorker_CanReceiveRequests_Old(t *testing.T) {
	logger := testLogger{t: t}
	log.SetOutput(logger)
	reqCh := make(chan proxy.McRequest)
	workerCfg := proxy.NewWorkerConfig(reqCh, nil, unknownServerStatusPk())
	worker := proxy.NewWorker(workerCfg)
	go worker.Work()
	select {
	case reqCh <- proxy.McRequest{}:
		t.Log("worker has successfully received request")
	case <-time.After(defaultChTimeout):
		t.Error("timed out")
	}
}

func TestStatusUnknownAddr_ReturnDefaultStatus_Old(t *testing.T) {
	logger := testLogger{t: t}
	log.SetOutput(logger)
	servers := make(map[string]proxy.WorkerServerConfig)
	reqCh := setupBasicWorkers(servers)

	answerCh := make(chan proxy.McAnswer)
	reqCh <- proxy.McRequest{
		Type:       proxy.STATUS,
		ServerAddr: "some weird server address",
		Ch:         answerCh,
	}
	defaultStatusPk := unknownServerStatusPk()
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

func TestStatusKnownAddr_ReturnOfflineStatus_WhenServerOffline_Old(t *testing.T) {
	logger := testLogger{t: t}
	log.SetOutput(logger)
	serverAddr := "ultraviolet"
	offlineStatusPk := defaultOfflineStatusPacket()
	servers := make(map[string]proxy.WorkerServerConfig)
	servers[serverAddr] = proxy.WorkerServerConfig{
		OfflineStatus: offlineStatusPk,
		State:         proxy.OFFLINE,
	}
	reqCh := setupBasicWorkers(servers)

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

func TestLoginUnknownAddr_ShouldClose_Old(t *testing.T) {
	logger := testLogger{t: t}
	log.SetOutput(logger)
	servers := make(map[string]proxy.WorkerServerConfig)
	reqCh := setupBasicWorkers(servers)

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

func TestLoginKnownAddr_Online_ShouldProxy_Old(t *testing.T) {
	logger := testLogger{t: t}
	log.SetOutput(logger)
	serverAddr := "ultraviolet"
	targetAddr := testAddr()
	servers := make(map[string]proxy.WorkerServerConfig)
	servers[serverAddr] = proxy.WorkerServerConfig{
		ProxyTo: targetAddr,
		State:   proxy.ONLINE,
	}
	reqCh := setupBasicWorkers(servers)

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
			t.Errorf("Got an unexpected error: %v", err)
		}
		if answer.ProxyCh == nil {
			t.Error("No proxy channel provided")
		}
	case <-time.After(defaultChTimeout):
		t.Error("timed out")
	}
}

func TestLoginProxyBind_Old(t *testing.T) {
	logger := testLogger{t: t}
	log.SetOutput(logger)
	serverAddr := "ultraviolet"
	proxyTo := testAddr()
	proxyBind := "127.0.0.2"
	servers := make(map[string]proxy.WorkerServerConfig)
	servers[serverAddr] = proxy.WorkerServerConfig{
		ProxyTo:   proxyTo,
		State:     proxy.ONLINE,
		ProxyBind: proxyBind,
	}
	reqCh := setupBasicWorkers(servers)

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

func TestLoginProxyProtocol_Old(t *testing.T) {
	logger := testLogger{t: t}
	log.SetOutput(logger)
	serverAddr := "ultraviolet"
	proxyTo := testAddr()
	servers := make(map[string]proxy.WorkerServerConfig)
	servers[serverAddr] = proxy.WorkerServerConfig{
		ProxyTo:           proxyTo,
		SendProxyProtocol: true,
		State:             proxy.ONLINE,
	}
	playerAddr := &net.TCPAddr{
		IP:   net.ParseIP("187.34.26.123"),
		Port: 49473,
	}
	reqCh := setupBasicWorkers(servers)

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

func TestLoginKnownAddr_Offline_ShouldDisconnect_Old(t *testing.T) {
	logger := testLogger{t: t}
	log.SetOutput(logger)
	serverAddr := "ultraviolet"
	disconPacket := mc.ClientBoundDisconnect{
		Reason: "Some disconnect message right here",
	}.Marshal()
	servers := make(map[string]proxy.WorkerServerConfig)
	servers[serverAddr] = proxy.WorkerServerConfig{
		State:            proxy.OFFLINE,
		DisconnectPacket: disconPacket,
	}
	reqCh := setupBasicWorkers(servers)

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

func TestStatusKnownAddr_ProxyConnection_WhenServerOnline_Old(t *testing.T) {
	logger := testLogger{t: t}
	log.SetOutput(logger)
	serverAddr := "ultraviolet"
	targetAddr := testAddr()
	servers := make(map[string]proxy.WorkerServerConfig)
	servers[serverAddr] = proxy.WorkerServerConfig{
		ProxyTo: targetAddr,
		State:   proxy.ONLINE,
	}
	reqCh := setupBasicWorkers(servers)

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
	logger := testLogger{t: t}
	log.SetOutput(logger)
	rateLimit := 3
	rateLimitDuration := 1 * time.Minute
	serverAddr := "ultraviolet"
	targetAddr := testAddr()
	servers := make(map[string]proxy.WorkerServerConfig)
	servers[serverAddr] = proxy.WorkerServerConfig{
		ProxyTo:           targetAddr,
		RateLimit:         rateLimit,
		RateLimitDuration: rateLimitDuration,
		State:             proxy.ONLINE,
	}
	reqCh := setupBasicWorkers(servers)

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
	logger := testLogger{t: t}
	log.SetOutput(logger)
	rateLimit := 1
	rateLimitDuration := 10 * time.Millisecond
	serverAddr := "ultraviolet"
	targetAddr := testAddr()
	servers := make(map[string]proxy.WorkerServerConfig)
	servers[serverAddr] = proxy.WorkerServerConfig{
		ProxyTo:           targetAddr,
		RateLimit:         rateLimit,
		RateLimitDuration: rateLimitDuration,
		State:             proxy.ONLINE,
	}
	reqCh := setupBasicWorkers(servers)

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

func TestLoginKnownAddr_UNKNOWNStateWithoutListener_ShouldDisconnect(t *testing.T) {
	logger := testLogger{t: t}
	log.SetOutput(logger)
	serverAddr := "ultraviolet"
	targetAddr := testAddr()
	servers := make(map[string]proxy.WorkerServerConfig)
	servers[serverAddr] = proxy.WorkerServerConfig{
		ProxyTo: targetAddr,
		State:   proxy.UNKNOWN,
	}
	reqCh := setupBasicWorkers(servers)

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
			t.Fatalf("expcted: %v \ngot: %v", proxy.DISCONNECT, answer.Action)
		}
	case <-time.After(defaultChTimeout):
		t.Error("timed out")
	}
}

func TestStatusKnownAddr_UNKNOWNStateWithListener_ShouldProxyConnection(t *testing.T) {
	logger := testLogger{t: t}
	log.SetOutput(logger)
	serverAddr := "ultraviolet"
	targetAddr := testAddr()
	servers := make(map[string]proxy.WorkerServerConfig)
	servers[serverAddr] = proxy.WorkerServerConfig{
		ProxyTo: targetAddr,
		State:   proxy.UNKNOWN,
	}
	reqCh := setupBasicWorkers(servers)

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

func TestStatusKnownAddr_UNKNOWNStateWithoutListener_ShouldSendStatus(t *testing.T) {
	logger := testLogger{t: t}
	log.SetOutput(logger)
	serverAddr := "ultraviolet"
	targetAddr := testAddr()
	servers := make(map[string]proxy.WorkerServerConfig)
	servers[serverAddr] = proxy.WorkerServerConfig{
		ProxyTo: targetAddr,
		State:   proxy.UNKNOWN,
	}
	reqCh := setupBasicWorkers(servers)

	answerCh := make(chan proxy.McAnswer)
	reqCh <- proxy.McRequest{
		Type:       proxy.STATUS,
		ServerAddr: serverAddr,
		Ch:         answerCh,
	}

	select {
	case answer := <-answerCh:
		t.Log("worker has successfully responded")
		if answer.Action != proxy.SEND_STATUS {
			t.Fatalf("expcted: %v \ngot: %v", proxy.SEND_STATUS, answer.Action)
		}
	case <-time.After(defaultChTimeout):
		t.Error("timed out")
	}
}

func TestUnknownState_Should_NOT_CallAgainWithinCooldown(t *testing.T) {
	logger := testLogger{t: t}
	log.SetOutput(logger)
	serverAddr := "ultraviolet"
	targetAddr := testAddr()
	servers := make(map[string]proxy.WorkerServerConfig)
	servers[serverAddr] = proxy.WorkerServerConfig{
		ProxyTo:             targetAddr,
		State:               proxy.UNKNOWN,
		StateUpdateCooldown: time.Minute,
	}
	reqCh := setupBasicWorkers(servers)

	answerCh := make(chan proxy.McAnswer)
	request := proxy.McRequest{
		Type:       proxy.LOGIN,
		ServerAddr: serverAddr,
		Ch:         answerCh,
	}

	reqCh <- request
	<-answerCh

	connCh, _ := createListener(t, targetAddr)
	reqCh <- request
	select {
	case <-connCh:
		t.Error("worker called server again")
	case <-time.After(defaultChTimeout):
		t.Log("worker didnt call again")
	}
}

func TestUnknownState_ShouldCallAgainOutOfCooldown(t *testing.T) {
	logger := testLogger{t: t}
	log.SetOutput(logger)
	serverAddr := "ultraviolet"
	targetAddr := testAddr()
	cooldown := defaultChTimeout
	servers := make(map[string]proxy.WorkerServerConfig)
	servers[serverAddr] = proxy.WorkerServerConfig{
		ProxyTo:             targetAddr,
		State:               proxy.UNKNOWN,
		StateUpdateCooldown: cooldown,
	}
	reqCh := setupBasicWorkers(servers)

	answerCh := make(chan proxy.McAnswer)
	request := proxy.McRequest{
		Type:       proxy.LOGIN,
		ServerAddr: serverAddr,
		Ch:         answerCh,
	}

	reqCh <- request
	<-answerCh
	connCh, _ := createListener(t, targetAddr)
	time.Sleep(cooldown * 2)
	reqCh <- request
	select {
	case <-connCh:
		t.Log("worker has successfully responded")
	case <-time.After(defaultChTimeout):
		t.Error("timed out")
	}
}

func TestStatusWorker_ShareServerData(t *testing.T) {
	logger := testLogger{t: t}
	log.SetOutput(logger)
	serverAddr := "ultraviolet"
	targetAddr := testAddr()
	cooldown := time.Minute
	servers := make(map[string]proxy.WorkerServerConfig)
	servers[serverAddr] = proxy.WorkerServerConfig{
		ProxyTo:             targetAddr,
		State:               proxy.UNKNOWN,
		StateUpdateCooldown: cooldown,
	}
	connCh := make(chan proxy.ConnRequest)
	statusCh := make(chan proxy.StatusRequest)
	proxy.RunConnWorkers(1, connCh, statusCh, servers)
	proxy.RunStatusWorkers(2, statusCh, connCh, servers)

	answerCh := make(chan proxy.StatusAnswer)
	statusCh <- proxy.StatusRequest{
		ServerId: serverAddr,
		Type:     proxy.STATE_REQUEST,
		AnswerCh: answerCh,
	}
	time.Sleep(defaultChTimeout)
	answerCh2 := make(chan proxy.StatusAnswer)
	statusCh <- proxy.StatusRequest{
		ServerId: serverAddr,
		Type:     proxy.STATE_REQUEST,
		AnswerCh: answerCh2,
	}

	select {
	case answer := <-answerCh2:
		t.Log("worker has successfully responded")
		if answer.State != proxy.OFFLINE {
			t.Errorf("expected: %v got: %v", proxy.OFFLINE, answer)
		}
	case <-time.After(defaultChTimeout):
		t.Error("timed out")
	}
}
