package old_proxy_test

import (
	"net"
	"testing"
	"time"

	"github.com/realDragonium/Ultraviolet/config"
	"github.com/realDragonium/Ultraviolet/mc"
	"github.com/realDragonium/Ultraviolet/old_proxy"
)

var basicServerCfgs = []config.ServerConfig{
	{
		Domains: []string{"uv1"},
		ProxyTo: testAddr(),
	},
	{
		Domains: []string{"uv2", "uve1"},
		ProxyTo: testAddr(),
	},
	{
		Domains: []string{"uv3", "uve2"},
		ProxyTo: testAddr(),
	},
	{
		Domains: []string{"uv4", "uve3", "uve4"},
		ProxyTo: testAddr(),
	},
}

type benchLogger struct {
	b *testing.B
}

func (bLog *benchLogger) Write(b []byte) (n int, err error) {
	bLog.b.Logf(string(b))
	return 0, nil
}

func basicBenchUVConfig(b *testing.B, w int) config.UltravioletConfig {
	return config.UltravioletConfig{
		NumberOfWorkers: w,
		DefaultStatus: mc.SimpleStatus{
			Name:        "Ultraviolet",
			Protocol:    755,
			Description: "Another proxy server",
		},
		LogOutput: &benchLogger{b: b},
	}
}

var serverWorkerCfg = config.WorkerServerConfig{
	StateUpdateCooldown: time.Second * 10,
	OfflineStatus: mc.SimpleStatus{
		Name:        "Ultraviolet",
		Protocol:    755,
		Description: "Some benchmark status",
	}.Marshal(),
	ProxyTo: "127.0.0.1:29870",
	DisconnectPacket: mc.ClientBoundDisconnect{
		Reason: "Benchmarking stay out!",
	}.Marshal(),
}

func BenchmarkPublicWorker_ProcessMcRequest_UnknownAddress_ReturnsValues(b *testing.B) {
	serverAddr := "ultraviolet"
	servers := make(map[int]old_proxy.ServerWorkerData)
	serverDict := make(map[string]int)
	reqCh := make(chan old_proxy.McRequest)
	publicWorker := old_proxy.NewPublicWorker(servers, serverDict, mc.Packet{}, reqCh)
	answerCh := make(chan old_proxy.McAnswer)
	req := old_proxy.McRequest{
		Type:       old_proxy.STATUS,
		ServerAddr: serverAddr,
		Ch:         answerCh,
	}
	go publicWorker.Work()

	b.ResetTimer()
	for n := 0; n < b.N; n++ {
		reqCh <- req
		<-answerCh
	}
}

func BenchmarkPrivateWorker_HandleRequest_Status_Offline(b *testing.B) {
	privateWorker := old_proxy.NewPrivateWorker(0, serverWorkerCfg)

	answerCh := make(chan old_proxy.McAnswer)
	req := old_proxy.McRequest{
		Type: old_proxy.STATUS,
		Ch:   answerCh,
	}

	go func() {
		for {
			<-answerCh
		}
	}()

	b.ResetTimer()
	for n := 0; n < b.N; n++ {
		privateWorker.HandleRequest(req)
	}
}

func BenchmarkPrivateWorker_HandleRequest_Status_Online(b *testing.B) {
	serverCfg := serverWorkerCfg
	serverCfg.ProxyTo = testAddr()
	privateWorker := old_proxy.NewPrivateWorker(0, serverCfg)
	answerCh := make(chan old_proxy.McAnswer)
	req := old_proxy.McRequest{
		Type: old_proxy.STATUS,
		Ch:   answerCh,
	}

	listener, err := net.Listen("tcp", serverCfg.ProxyTo)
	if err != nil {
		b.Fatal(err)
	}

	go func() {
		for {
			listener.Accept()
		}
	}()

	b.ResetTimer()
	for n := 0; n < b.N; n++ {
		privateWorker.HandleRequest(req)
	}
}

func BenchmarkPrivateWorker_HandleRequest_Login_Offline(b *testing.B) {
	privateWorker := old_proxy.NewPrivateWorker(0, serverWorkerCfg)

	answerCh := make(chan old_proxy.McAnswer)
	req := old_proxy.McRequest{
		Type: old_proxy.STATUS,
		Ch:   answerCh,
	}

	b.ResetTimer()
	for n := 0; n < b.N; n++ {
		privateWorker.HandleRequest(req)
	}
}

func BenchmarkPrivateWorker_HandleRequest_Login_Online_StateUpdate100us(b *testing.B) {
	benchmarkPrivateWorker_HandleRequest_Login_Online_StateUpdate(b, 100*time.Microsecond)
}

func BenchmarkPrivateWorker_HandleRequest_Login_Online_StateUpdate10ms(b *testing.B) {
	benchmarkPrivateWorker_HandleRequest_Login_Online_StateUpdate(b, 10*time.Millisecond)
}

func BenchmarkPrivateWorker_HandleRequest_Login_Online_StateUpdate1us(b *testing.B) {
	benchmarkPrivateWorker_HandleRequest_Login_Online_StateUpdate(b, time.Microsecond)
}

func BenchmarkPrivateWorker_HandleRequest_Login_Online_StateUpdate10s(b *testing.B) {
	benchmarkPrivateWorker_HandleRequest_Login_Online_StateUpdate(b, 10*time.Second)
}

func benchmarkPrivateWorker_HandleRequest_Login_Online_StateUpdate(b *testing.B, stateCooldown time.Duration) {
	testAddr := testAddr()
	serverCfg := serverWorkerCfg
	serverCfg.ProxyTo = testAddr
	serverCfg.StateUpdateCooldown = stateCooldown
	privateWorker := old_proxy.NewPrivateWorker(0, serverCfg)

	req := old_proxy.McRequest{
		Type: old_proxy.LOGIN,
	}

	benchmarkPrivateWorker_HandleRequest_Online(b, privateWorker, req, testAddr)
}

func benchmarkPrivateWorker_HandleRequest_Online(b *testing.B, pWorker old_proxy.PrivateWorker, req old_proxy.McRequest, addr string) {
	listener, err := net.Listen("tcp", addr)
	if err != nil {
		b.Fatal(err)
	}

	go func() {
		for {
			listener.Accept()
		}
	}()

	b.ResetTimer()
	for n := 0; n < b.N; n++ {
		pWorker.HandleRequest(req)
	}
}

// func basicWorkerServerConfigMap(cfgs []config.ServerConfig) map[string]old_proxy.WorkerServerConfig {
// 	servers := make(map[string]old_proxy.WorkerServerConfig)
// 	for _, cfg := range cfgs {
// 		workerCfg := old_proxy.FileToWorkerConfig(cfg)
// 		servers[cfg.MainDomain] = workerCfg
// 		for _, extraDomains := range cfg.ExtraDomains {
// 			servers[extraDomains] = workerCfg
// 		}
// 	}

// 	return servers
// }

// func BenchmarkStatusWorker_dialTimeout_5s_stateCooldown_5s(b *testing.B) {
// 	benchmarkStatusWorker_StatusRequest_OfflineServer_WithConnWorker(b, "5s", "5s")
// }

// func BenchmarkStatusWorker_dialTimeout_1s_stateCooldown_1s(b *testing.B) {
// 	benchmarkStatusWorker_StatusRequest_OfflineServer_WithConnWorker(b, "1s", "1s")
// }

// func BenchmarkStatusWorker_dialTimeout_100ms_stateCooldown_1s(b *testing.B) {
// 	benchmarkStatusWorker_StatusRequest_OfflineServer_WithConnWorker(b, "100ms", "1s")
// }

// func BenchmarkStatusWorker_dialTimeout_1s_stateCooldown_100ms(b *testing.B) {
// 	benchmarkStatusWorker_StatusRequest_OfflineServer_WithConnWorker(b, "1s", "100ms")
// }

// func BenchmarkStatusWorker_dialTimeout_100ms_stateCooldown_100ms(b *testing.B) {
// 	benchmarkStatusWorker_StatusRequest_OfflineServer_WithConnWorker(b, "100ms", "100ms")
// }

// func BenchmarkStatusWorker_dialTimeout_100ms_stateCooldown_10ms(b *testing.B) {
// 	benchmarkStatusWorker_StatusRequest_OfflineServer_WithConnWorker(b, "100ms", "10ms")
// }

// func BenchmarkStatusWorker_dialTimeout_10ms_stateCooldown_10ms(b *testing.B) {
// 	benchmarkStatusWorker_StatusRequest_OfflineServer_WithConnWorker(b, "10ms", "10ms")
// }

// func BenchmarkStatusWorker_dialTimeout_1ms_stateCooldown_10ms(b *testing.B) {
// 	benchmarkStatusWorker_StatusRequest_OfflineServer_WithConnWorker(b, "1ms", "10ms")
// }

// func benchmarkStatusWorker_StatusRequest_OfflineServer_WithConnWorker(b *testing.B, dialTime, cooldown string) {
// 	connCh := make(chan old_proxy.ConnRequest)
// 	statusCh := make(chan old_proxy.StatusRequest)
// 	serverCfgs := basicServerCfgs
// 	for _, cfg := range serverCfgs {
// 		cfg.DialTimeout = dialTime
// 		cfg.UpdateCooldown = cooldown
// 	}
// 	servers := basicWorkerServerConfigMap(serverCfgs)

// 	old_proxy.RunConnWorkers(10, connCh, statusCh, servers)
// 	statusWorker := old_proxy.NewStatusWorker(statusCh, connCh, servers)
// 	req := old_proxy.StatusRequest{
// 		ServerId: "uv1",
// 		Type:     old_proxy.STATUS_REQUEST,
// 	}
// 	b.ResetTimer()
// 	b.ReportAllocs()
// 	for n := 0; n < b.N; n++ {
// 		statusWorker.WorkSingle(req)
// 	}
// }

// func BenchmarkStatusWorker_StatusUpdate_To_Online(b *testing.B) {
// 	connCh := make(chan old_proxy.ConnRequest)
// 	statusCh := make(chan old_proxy.StatusRequest)
// 	servers := basicWorkerServerConfigMap(basicServerCfgs)
// 	statusWorker := old_proxy.NewStatusWorker(statusCh, connCh, servers)
// 	req := old_proxy.StatusRequest{
// 		ServerId: "uv1",
// 		Type:     old_proxy.STATE_UPDATE,
// 		State:    old_proxy.ONLINE,
// 	}

// 	b.ResetTimer()
// 	for n := 0; n < b.N; n++ {
// 		statusWorker.WorkSingle(req)
// 	}
// }

// func BenchmarkStatusWorker_StatusUpdate_To_Online_CHANNEL(b *testing.B) {
// 	connCh := make(chan old_proxy.ConnRequest)
// 	statusCh := make(chan old_proxy.StatusRequest)
// 	servers := basicWorkerServerConfigMap(basicServerCfgs)
// 	statusWorker := old_proxy.NewStatusWorker(statusCh, connCh, servers)
// 	go statusWorker.Work()

// 	req := old_proxy.StatusRequest{
// 		ServerId: "uv1",
// 		Type:     old_proxy.STATE_UPDATE,
// 		State:    old_proxy.ONLINE,
// 	}

// 	b.ResetTimer()
// 	for n := 0; n < b.N; n++ {
// 		statusCh <- req
// 	}
// }

// func BenchmarkStatusWorker_StatusUpdate_To_Offline(b *testing.B) {
// 	connCh := make(chan old_proxy.ConnRequest)
// 	statusCh := make(chan old_proxy.StatusRequest)
// 	servers := basicWorkerServerConfigMap(basicServerCfgs)
// 	statusWorker := old_proxy.NewStatusWorker(statusCh, connCh, servers)
// 	req := old_proxy.StatusRequest{
// 		ServerId: "uv4",
// 		Type:     old_proxy.STATE_UPDATE,
// 		State:    old_proxy.OFFLINE,
// 	}

// 	b.ResetTimer()
// 	for n := 0; n < b.N; n++ {
// 		statusWorker.WorkSingle(req)
// 	}
// }

func BenchmarkWorkerStatusRequest_KnownServer_Offline_CHANNEL(b *testing.B) {
	req := old_proxy.McRequest{
		ServerAddr: "something",
		Type:       old_proxy.STATUS,
	}
	benchmarkWorker(b, req)
}
func BenchmarkWorkerStatusRequest_UnknownServer_CHANNEL(b *testing.B) {
	req := old_proxy.McRequest{
		ServerAddr: "something",
		Type:       old_proxy.STATUS,
	}
	benchmarkWorker(b, req)
}

func benchmarkWorker(b *testing.B, req old_proxy.McRequest) {
	cfg := basicBenchUVConfig(b, 1)
	servers := basicServerCfgs
	reqCh := make(chan old_proxy.McRequest)
	gateway := old_proxy.NewGateway()
	gateway.StartWorkers(cfg, servers, reqCh)

	answerCh := make(chan old_proxy.McAnswer)
	req.Ch = answerCh

	b.ResetTimer()
	for n := 0; n < b.N; n++ {
		reqCh <- req
		<-answerCh
	}
}

// func BenchmarkNetworkStatusRequest_UnknownServer(b *testing.B) {
// 	targetAddr := testAddr()
// 	cfg := config.UltravioletConfig{
// 		NumberOfWorkers:       1,
// 		NumberOfConnWorkers:   1,
// 		NumberOfStatusWorkers: 1,
// 		DefaultStatus: mc.AnotherStatusResponse{
// 			Name:        "Ultraviolet",
// 			Protocol:    755,
// 			Description: "Another proxy server",
// 		},
// 	}
// 	servers := []config.ServerConfig{}
// 	reqCh := make(chan old_proxy.McRequest)
// 	ln, err := net.Listen("tcp", targetAddr)
// 	if err != nil {
// 		b.Fatalf("Can't listen: %v", err)
// 	}
// 	go old_proxy.ServeListener(ln, reqCh)

// 	old_proxy.SetupWorkers(cfg, servers, reqCh, nil)

// 	handshakePk := mc.ServerBoundHandshake{
// 		ProtocolVersion: 755,
// 		ServerAddress:   "unknown",
// 		ServerPort:      25565,
// 		NextState:       mc.HandshakeStatusState,
// 	}.Marshal()
// 	handshakeBytes, _ := handshakePk.Marshal()

// 	statusRequestPk := mc.ServerBoundRequest{}.Marshal()
// 	statusRequestBytes, _ := statusRequestPk.Marshal()
// 	pingPk := mc.NewServerBoundPing().Marshal()
// 	pingBytes, _ := pingPk.Marshal()

// 	readBuffer := make([]byte, 0xffff)

// 	b.ResetTimer()
// 	for n := 0; n < b.N; n++ {
// 		conn, err := net.Dial("tcp", targetAddr)
// 		if err != nil {
// 			b.Fatalf("error while trying to connect: %v", err)
// 		}
// 		conn.Write(handshakeBytes)
// 		conn.Write(statusRequestBytes)
// 		conn.Read(readBuffer)
// 		conn.Write(pingBytes)
// 		conn.Read(readBuffer)
// 	}
// }
