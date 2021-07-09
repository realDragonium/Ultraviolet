package proxy_test

import (
	"net"
	"testing"
	"time"

	"github.com/realDragonium/Ultraviolet/config"
	"github.com/realDragonium/Ultraviolet/mc"
	"github.com/realDragonium/Ultraviolet/proxy"
)

func TestFileToWorkerConfig(t *testing.T) {
	serverCfg := config.ServerConfig{
		MainDomain:        "Ultraviolet",
		ExtraDomains:      []string{"Ultraviolet2", "UltraV", "UV"},
		ProxyTo:           "127.0.10.5:25565",
		ProxyBind:         "127.0.0.5",
		SendProxyProtocol: true,
		DisconnectMessage: "HelloThereWeAreClosed...Sorry",
		OfflineStatus: mc.AnotherStatusResponse{
			Name:        "Ultraviolet",
			Protocol:    755,
			Description: "Some broken proxy",
		},
		RateLimit:    5,
		RateDuration: "1m",
	}

	expectedDisconPk := mc.ClientBoundDisconnect{
		Reason: mc.String(serverCfg.DisconnectMessage),
	}.Marshal()
	expectedOfflineStatus := mc.AnotherStatusResponse{
		Name:        "Ultraviolet",
		Protocol:    755,
		Description: "Some broken proxy",
	}.Marshal()
	expectedRateDuration := 1 * time.Minute

	workerCfg := proxy.FileToWorkerConfig(serverCfg)

	if workerCfg.ProxyTo != serverCfg.ProxyTo {
		t.Errorf("expected: %v - got: %v", serverCfg.ProxyTo, workerCfg.ProxyTo)
	}
	if workerCfg.ProxyBind != serverCfg.ProxyBind {
		t.Errorf("expected: %v - got: %v", serverCfg.ProxyBind, workerCfg.ProxyBind)
	}
	if workerCfg.SendProxyProtocol != serverCfg.SendProxyProtocol {
		t.Errorf("expected: %v - got: %v", serverCfg.SendProxyProtocol, workerCfg.SendProxyProtocol)
	}
	if workerCfg.RateLimit != serverCfg.RateLimit {
		t.Errorf("expected: %v - got: %v", serverCfg.RateLimit, workerCfg.RateLimit)
	}
	if expectedRateDuration != workerCfg.RateLimitDuration {
		t.Errorf("expected: %v - got: %v", expectedRateDuration, workerCfg.RateLimitDuration)
	}
	if !samePk(expectedOfflineStatus, workerCfg.OfflineStatus) {
		offlineStatus, _ := mc.UnmarshalClientBoundResponse(expectedOfflineStatus)
		receivedStatus, _ := mc.UnmarshalClientBoundResponse(workerCfg.OfflineStatus)
		t.Errorf("expcted: %v \ngot: %v", offlineStatus, receivedStatus)
	}

	if !samePk(expectedDisconPk, workerCfg.DisconnectPacket) {
		expectedDiscon, _ := mc.UnmarshalClientDisconnect(expectedDisconPk)
		receivedDiscon, _ := mc.UnmarshalClientDisconnect(workerCfg.DisconnectPacket)
		t.Errorf("expcted: %v \ngot: %v", expectedDiscon, receivedDiscon)
	}
}

func BenchmarkWorkerStatusRequest_UnknownServer(b *testing.B) {
	cfg := config.UltravioletConfig{
		NumberOfWorkers:       1,
		NumberOfConnWorkers:   1,
		NumberOfStatusWorkers: 1,
		DefaultStatus: mc.AnotherStatusResponse{
			Name:        "Ultraviolet",
			Protocol:    755,
			Description: "Another proxy server",
		},
	}
	servers := []config.ServerConfig{}
	reqCh := make(chan proxy.McRequest)
	proxy.SetupWorkers(cfg, servers, reqCh, nil)

	b.ResetTimer()
	for n := 0; n < b.N; n++ {
		answerCh := make(chan proxy.McAnswer)
		request := proxy.McRequest{
			ServerAddr: "something",
			Type:       proxy.STATUS,
			Ch:         answerCh,
		}
		reqCh <- request
		go func(answerCh chan proxy.McAnswer) {
			<-answerCh
		}(answerCh)
	}
}

func BenchmarkNetworkStatusRequest_UnknownServer(b *testing.B) {
	targetAddr := testAddr()
	cfg := config.UltravioletConfig{
		NumberOfWorkers:       1,
		NumberOfConnWorkers:   1,
		NumberOfStatusWorkers: 1,
		DefaultStatus: mc.AnotherStatusResponse{
			Name:        "Ultraviolet",
			Protocol:    755,
			Description: "Another proxy server",
		},
	}
	servers := []config.ServerConfig{}
	reqCh := make(chan proxy.McRequest)
	ln, err := net.Listen("tcp", targetAddr)
	if err != nil {
		b.Fatalf("Can't listen: %v", err)
	}
	go proxy.ServeListener(ln, reqCh)

	proxy.SetupWorkers(cfg, servers, reqCh, nil)

	handshakePk := mc.ServerBoundHandshake{
		ProtocolVersion: 755,
		ServerAddress:   "unknown",
		ServerPort:      25565,
		NextState:       mc.HandshakeStatusState,
	}.Marshal()
	handshakeBytes, _ := handshakePk.Marshal()

	statusRequestPk := mc.ServerBoundRequest{}.Marshal()
	statusRequestBytes, _ := statusRequestPk.Marshal()
	pingPk := mc.NewServerBoundPing().Marshal()
	pingBytes, _ := pingPk.Marshal()

	readBuffer := make([]byte, 0xffff)

	b.ResetTimer()
	for n := 0; n < b.N; n++ {
		conn, err := net.Dial("tcp", targetAddr)
		if err != nil {
			b.Fatalf("error while trying to connect: %v", err)
		}
		conn.Write(handshakeBytes)
		conn.Write(statusRequestBytes)
		conn.Read(readBuffer)
		conn.Write(pingBytes)
		conn.Read(readBuffer)
	}
}
