package proxy_test

import (
	"bytes"
	"net"
	"strings"
	"testing"
	"time"

	"github.com/realDragonium/Ultraviolet/conn"
	"github.com/realDragonium/Ultraviolet/mc"
	"github.com/realDragonium/Ultraviolet/proxy"
)

var defaultChTimeout time.Duration = 10 * time.Millisecond

func netAddrToIp(addr net.Addr) string {
	return strings.Split(addr.String(), ":")[0]
}

func defaultStatusPacket() mc.Packet {
	return mc.AnotherStatusResponse{
		Name:        "Ultraviolet",
		Protocol:    755,
		Description: "proxy being tested",
	}.Marshal()
}

func samePk(expected, received mc.Packet) bool {
	sameID := expected.ID == received.ID
	sameData := bytes.Equal(expected.Data, received.Data)

	return sameID && sameData
}

func UnknownServerCfg() proxy.UnknownServer {
	return proxy.UnknownServer{
		Status: defaultStatusPacket(),
	}
}

func TestWorker(t *testing.T) {
	setupWorker := func(proxies []proxy.WorkerServerConfig, defaultCfg proxy.UnknownServer) chan<- conn.ConnRequest {
		reqCh := make(chan conn.ConnRequest)
		servers := make(map[string]proxy.WorkerServerConfig)
		for _, proxy := range proxies {
			servers[proxy.MainDomain] = proxy
			for _, extraDomain := range proxy.ExtraDomains {
				servers[extraDomain] = proxy
			}
		}
		worker := proxy.Worker{
			ReqCh:      reqCh,
			Servers:    servers,
			DefaultCfg: defaultCfg,
		}
		go worker.Work()
		return reqCh
	}

	t.Run("can receive requests", func(t *testing.T) {
		reqCh := make(chan conn.ConnRequest)
		worker := proxy.Worker{ReqCh: reqCh}
		go worker.Work()
		select {
		case reqCh <- conn.ConnRequest{}:
			t.Log("worker has successfully received request")
		case <-time.After(defaultChTimeout):
			t.Error("timed out")
		}
	})

	t.Run("status-unknown address returns default status", func(t *testing.T) {
		servers := []proxy.WorkerServerConfig{}
		reqCh := setupWorker(servers, UnknownServerCfg())

		answerCh := make(chan conn.ConnAnswer)
		reqCh <- conn.ConnRequest{
			Type:       conn.STATUS,
			ServerAddr: "some weird server address",
			Ch:         answerCh,
		}
		defaultStatusPk := defaultStatusPacket()
		select {
		case answer := <-answerCh:
			t.Log("worker has successfully responded")
			if !samePk(defaultStatusPk, answer.StatusPk) {
				t.Errorf("expcted: %v \ngot: %v", defaultStatusPk, answer.StatusPk)
			}
			if answer.Action != conn.SEND_STATUS {
				t.Errorf("expcted: %v \ngot: %v", conn.SEND_STATUS, answer.Action)
			}
		case <-time.After(defaultChTimeout):
			t.Error("timed out")
		}
	})

	t.Run("status-known address returns offline status when server is offline", func(t *testing.T) {
		reqCh := make(chan conn.ConnRequest)
		serverAddr := "ultraviolet"
		offlineStatusPk := mc.AnotherStatusResponse{
			Name:        "Ultraviolet2",
			Protocol:    755,
			Description: "proxy2 being tested",
		}.Marshal()
		servers := make(map[string]proxy.WorkerServerConfig)
		servers[serverAddr] = proxy.WorkerServerConfig{
			OfflineStatus: offlineStatusPk,
			State:         proxy.OFFLINE,
		}
		worker := proxy.Worker{
			ReqCh:   reqCh,
			Servers: servers,
		}
		go worker.Work()

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
				t.Errorf("expcted: %v \ngot: %v", offlineStatusPk, answer.StatusPk)
			}
			if answer.Action != conn.SEND_STATUS {
				t.Errorf("expcted: %v \ngot: %v", conn.SEND_STATUS, answer.Action)
			}
		case <-time.After(defaultChTimeout):
			t.Error("timed out")
		}
	})

	t.Run("status-known address returns online status when server is online", func(t *testing.T) {
		reqCh := make(chan conn.ConnRequest)
		serverAddr := "ultraviolet"
		onlineStatusPk := mc.AnotherStatusResponse{
			Name:        "Ultraviolet2.o",
			Protocol:    755,
			Description: "proxy2 being tested online",
		}.Marshal()
		servers := make(map[string]proxy.WorkerServerConfig)
		servers[serverAddr] = proxy.WorkerServerConfig{
			OnlineStatus: onlineStatusPk,
		}
		worker := proxy.Worker{
			ReqCh:   reqCh,
			Servers: servers,
		}
		go worker.Work()

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
	})

	t.Run("login-unknown address should close connections", func(t *testing.T) {
		reqCh := make(chan conn.ConnRequest)
		servers := make(map[string]proxy.WorkerServerConfig)
		worker := proxy.Worker{
			ReqCh:   reqCh,
			Servers: servers,
			DefaultCfg: proxy.UnknownServer{
				Status: defaultStatusPacket(),
			},
		}
		go worker.Work()

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
	})

	t.Run("login-known address offline server should close connections", func(t *testing.T) {
		reqCh := make(chan conn.ConnRequest)
		serverAddr := "ultraviolet"
		servers := make(map[string]proxy.WorkerServerConfig)
		servers[serverAddr] = proxy.WorkerServerConfig{
			State: proxy.OFFLINE,
		}
		worker := proxy.Worker{
			ReqCh:   reqCh,
			Servers: servers,
			DefaultCfg: proxy.UnknownServer{
				Status: defaultStatusPacket(),
			},
		}
		go worker.Work()

		answerCh := make(chan conn.ConnAnswer)
		reqCh <- conn.ConnRequest{
			Type:       conn.LOGIN,
			ServerAddr: serverAddr,
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
	})

	t.Run("login-known address online server should proxy connections", func(t *testing.T) {
		reqCh := make(chan conn.ConnRequest)
		serverAddr := "ultraviolet"
		servers := make(map[string]proxy.WorkerServerConfig)
		servers[serverAddr] = proxy.WorkerServerConfig{
			ProxyTo: "127.0.0.1:25565",
		}
		worker := proxy.Worker{
			ReqCh:   reqCh,
			Servers: servers,
		}
		go worker.Work()

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
			// Test server connection...?
		case <-time.After(defaultChTimeout):
			t.Error("timed out")
		}
	})

}

func TestNewServerConn_CreateConnection(t *testing.T) {
	acceptCh := make(chan net.Conn)
	errorCh := make(chan error)
	reqCh := make(chan conn.ConnRequest)
	serverAddr := "ultraviolet"
	servers := make(map[string]proxy.WorkerServerConfig)
	servers[serverAddr] = proxy.WorkerServerConfig{
		ProxyTo: "127.0.0.1:25000",
	}
	worker := proxy.Worker{
		ReqCh:   reqCh,
		Servers: servers,
	}
	go worker.Work()

	listener, err := net.Listen("tcp", "127.0.0.1:25000")
	if err != nil {
		t.Fatal(err)
	}

	answerCh := make(chan conn.ConnAnswer)
	reqCh <- conn.ConnRequest{
		Type:       conn.LOGIN,
		ServerAddr: serverAddr,
		Ch:         answerCh,
	}

	go func() {
		conn, err := listener.Accept()
		if err != nil {
			errorCh <- err
		}
		acceptCh <- conn
	}()

	select {
	case err := <-errorCh:
		t.Fatalf("error while accepting connection: %v", err)
	case conn := <-acceptCh:
		t.Log("connection has been created")
		_, err := conn.Write([]byte{1})
		if err != nil {
			t.Fatalf("Got an unexpected error: %v", err)
		}
	case <-time.After(defaultChTimeout):
		t.Error("timed out")
	}
}

func TestNewServerConn_ProxyBind(t *testing.T) {
	acceptCh := make(chan net.Conn)
	errorCh := make(chan error)
	serverCfg := proxy.WorkerServerConfig{
		ProxyTo:   "127.0.0.1:25001",
		ProxyBind: "127.0.0.2",
	}
	listener, err := net.Listen("tcp", "127.0.0.1:25001")
	if err != nil {
		t.Fatal(err)
	}

	go func() {
		_, err = proxy.NewServerConn(serverCfg)
		if err != nil {
			errorCh <- err
		}
	}()

	go func() {
		conn, err := listener.Accept()
		if err != nil {
			errorCh <- err
		}
		acceptCh <- conn
	}()

	select {
	case err := <-errorCh:
		t.Fatalf("error while accepting connection: %v", err)
	case conn := <-acceptCh:
		t.Log("connection has been created")
		if netAddrToIp(conn.RemoteAddr()) != serverCfg.ProxyBind {
			t.Errorf("expcted: %v \ngot: %v", serverCfg.ProxyBind, netAddrToIp(conn.RemoteAddr()))
		}
	case <-time.After(defaultChTimeout):
		t.Error("timed out")
	}
}

func TestNewServerConn_ProxyProtocol(t *testing.T) {
	acceptCh := make(chan net.Conn)
	errorCh := make(chan error)
	serverCfg := proxy.WorkerServerConfig{
		ProxyTo:   "127.0.0.1:25002",
		ProxyBind: "127.0.0.2",
	}
	listener, err := net.Listen("tcp", "127.0.0.1:25002")
	if err != nil {
		t.Fatal(err)
	}

	go func() {
		_, err = proxy.NewServerConn(serverCfg)
		if err != nil {
			errorCh <- err
		}
	}()

	go func() {
		conn, err := listener.Accept()
		if err != nil {
			errorCh <- err
		}
		acceptCh <- conn
	}()

	select {
	case err := <-errorCh:
		t.Fatalf("error while accepting connection: %v", err)
	case conn := <-acceptCh:
		t.Log("connection has been created")
		if netAddrToIp(conn.RemoteAddr()) != serverCfg.ProxyBind {
			t.Errorf("expcted: %v \ngot: %v", serverCfg.ProxyBind, netAddrToIp(conn.RemoteAddr()))
		}
	case <-time.After(defaultChTimeout):
		t.Error("timed out")
	}
}
