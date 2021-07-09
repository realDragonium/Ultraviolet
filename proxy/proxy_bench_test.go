package proxy_test

import (
	"testing"

	"github.com/realDragonium/Ultraviolet/config"
	"github.com/realDragonium/Ultraviolet/mc"
	"github.com/realDragonium/Ultraviolet/proxy"
)

var basicServerCfgs = []config.ServerConfig{
	{
		MainDomain: "uv1",
		ProxyTo:    testAddr(),
	},
	{
		MainDomain:   "uv2",
		ExtraDomains: []string{"uve1"},
		ProxyTo:      testAddr(),
	},
	{
		MainDomain:   "uv3",
		ExtraDomains: []string{"uve2"},
		ProxyTo:      testAddr(),
	},
	{
		MainDomain:   "uv4",
		ExtraDomains: []string{"uve3", "uve4"},
		ProxyTo:      testAddr(),
	},
}

type benchLogger struct {
	b *testing.B
}

func (bLog *benchLogger) Write(b []byte) (n int, err error) {
	bLog.b.Logf(string(b))
	return 0, nil
}

func basicBenchUVConfig(b *testing.B, w, cw, sw int) config.UltravioletConfig {
	return config.UltravioletConfig{
		NumberOfWorkers:       w,
		NumberOfConnWorkers:   cw,
		NumberOfStatusWorkers: sw,
		DefaultStatus: mc.AnotherStatusResponse{
			Name:        "Ultraviolet",
			Protocol:    755,
			Description: "Another proxy server",
		},
		LogOutput: &benchLogger{b: b},
	}
}

func basicWorkerServerConfigMap(cfgs []config.ServerConfig) map[string]proxy.WorkerServerConfig {
	servers := make(map[string]proxy.WorkerServerConfig)
	for _, cfg := range cfgs {
		workerCfg := proxy.FileToWorkerConfig(cfg)
		servers[cfg.MainDomain] = workerCfg
		for _, extraDomains := range cfg.ExtraDomains {
			servers[extraDomains] = workerCfg
		}
	}

	return servers
}

// func BenchmarkStatusWorker_dialTime_5s_stateCooldown_5s(b *testing.B) {
// 	benchmarkStatusWorker_StatusRequest_OfflineServer_WithConnWorker(b, "5s", "5s")
// }

// func BenchmarkStatusWorker_dialTime_1s_stateCooldown_1s(b *testing.B) {
// 	benchmarkStatusWorker_StatusRequest_OfflineServer_WithConnWorker(b, "1s", "1s")
// }

// func BenchmarkStatusWorker_dialTime_100ms_stateCooldown_1s(b *testing.B) {
// 	benchmarkStatusWorker_StatusRequest_OfflineServer_WithConnWorker(b, "100ms", "1s")
// }

func BenchmarkStatusWorker_dialTime_1s_stateCooldown_100ms(b *testing.B) {
	benchmarkStatusWorker_StatusRequest_OfflineServer_WithConnWorker(b, "1s", "100ms")
}

func BenchmarkStatusWorker_dialTime_100ms_stateCooldown_100ms(b *testing.B) {
	benchmarkStatusWorker_StatusRequest_OfflineServer_WithConnWorker(b, "100ms", "100ms")
}

func BenchmarkStatusWorker_dialTime_100ms_stateCooldown_10ms(b *testing.B) {
	benchmarkStatusWorker_StatusRequest_OfflineServer_WithConnWorker(b, "100ms", "10ms")
}

func BenchmarkStatusWorker_dialTime_10ms_stateCooldown_10ms(b *testing.B) {
	benchmarkStatusWorker_StatusRequest_OfflineServer_WithConnWorker(b, "10ms", "10ms")
}

func BenchmarkStatusWorker_dialTime_1ms_stateCooldown_10ms(b *testing.B) {
	benchmarkStatusWorker_StatusRequest_OfflineServer_WithConnWorker(b, "1ms", "10ms")
}

func benchmarkStatusWorker_StatusRequest_OfflineServer_WithConnWorker(b *testing.B, dialTime, cooldown string) {
	connCh := make(chan proxy.ConnRequest)
	statusCh := make(chan proxy.StatusRequest)
	serverCfgs := basicServerCfgs
	for _, cfg := range serverCfgs {
		cfg.DialTimeout = dialTime
		cfg.UpdateCooldown = cooldown
	}
	servers := basicWorkerServerConfigMap(serverCfgs)

	proxy.RunConnWorkers(50, connCh, statusCh, servers)
	statusWorker := proxy.NewStatusWorker(statusCh, connCh, servers)
	req := proxy.StatusRequest{
		ServerId: "uv1",
		Type:     proxy.STATUS_REQUEST,
	}
	b.ResetTimer()
	b.ReportAllocs()
	for n := 0; n < b.N; n++ {
		statusWorker.WorkSingle(req)
	}
}

func BenchmarkStatusWorker_StatusUpdate_To_Online(b *testing.B) {
	connCh := make(chan proxy.ConnRequest)
	statusCh := make(chan proxy.StatusRequest)
	servers := basicWorkerServerConfigMap(basicServerCfgs)
	statusWorker := proxy.NewStatusWorker(statusCh, connCh, servers)
	req := proxy.StatusRequest{
		ServerId: "uv1",
		Type:     proxy.STATE_UPDATE,
		State:    proxy.ONLINE,
	}

	b.ResetTimer()
	for n := 0; n < b.N; n++ {
		statusWorker.WorkSingle(req)
	}
}

func BenchmarkStatusWorker_StatusUpdate_To_Online_CHANNEL(b *testing.B) {
	connCh := make(chan proxy.ConnRequest)
	statusCh := make(chan proxy.StatusRequest)
	servers := basicWorkerServerConfigMap(basicServerCfgs)
	statusWorker := proxy.NewStatusWorker(statusCh, connCh, servers)
	go statusWorker.Work()

	req := proxy.StatusRequest{
		ServerId: "uv1",
		Type:     proxy.STATE_UPDATE,
		State:    proxy.ONLINE,
	}

	b.ResetTimer()
	for n := 0; n < b.N; n++ {
		statusCh <- req
	}
}

func BenchmarkStatusWorker_StatusUpdate_To_Offline(b *testing.B) {
	connCh := make(chan proxy.ConnRequest)
	statusCh := make(chan proxy.StatusRequest)
	servers := basicWorkerServerConfigMap(basicServerCfgs)
	statusWorker := proxy.NewStatusWorker(statusCh, connCh, servers)
	req := proxy.StatusRequest{
		ServerId: "uv4",
		Type:     proxy.STATE_UPDATE,
		State:    proxy.OFFLINE,
	}

	b.ResetTimer()
	for n := 0; n < b.N; n++ {
		statusWorker.WorkSingle(req)
	}
}

func BenchmarkWorkerStatusRequest_KnownServer_Offline_CHANNEL(b *testing.B) {
	req := proxy.McRequest{
		ServerAddr: "something",
		Type:       proxy.STATUS,
	}
	benchmarkWorker(b, req)
}
func BenchmarkWorkerStatusRequest_UnknownServer_CHANNEL(b *testing.B) {
	req := proxy.McRequest{
		ServerAddr: "something",
		Type:       proxy.STATUS,
	}
	benchmarkWorker(b, req)
}

func benchmarkWorker(b *testing.B, req proxy.McRequest) {
	cfg := basicBenchUVConfig(b, 1, 1, 1)
	servers := []config.ServerConfig{
		{
			MainDomain: "uv1",
			ProxyTo:    testAddr(),
		},
		{
			MainDomain:   "uv2",
			ExtraDomains: []string{"uve1"},
			ProxyTo:      testAddr(),
		},
		{
			MainDomain:   "uv3",
			ExtraDomains: []string{"uve2"},
			ProxyTo:      testAddr(),
		},
		{
			MainDomain:   "uv4",
			ExtraDomains: []string{"uve3", "uve4"},
			ProxyTo:      testAddr(),
		},
	}
	reqCh := make(chan proxy.McRequest)
	proxy.SetupWorkers(cfg, servers, reqCh, nil)

	answerCh := make(chan proxy.McAnswer)
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
// 	reqCh := make(chan proxy.McRequest)
// 	ln, err := net.Listen("tcp", targetAddr)
// 	if err != nil {
// 		b.Fatalf("Can't listen: %v", err)
// 	}
// 	go proxy.ServeListener(ln, reqCh)

// 	proxy.SetupWorkers(cfg, servers, reqCh, nil)

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
