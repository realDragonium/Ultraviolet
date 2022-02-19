package ultraviolet_test

import (
	"errors"
	"net"
	"testing"
	"time"

	"github.com/google/go-cmp/cmp"
	ultraviolet "github.com/realDragonium/Ultraviolet"
	"github.com/realDragonium/Ultraviolet/config"
	"github.com/realDragonium/Ultraviolet/core"
	"github.com/realDragonium/Ultraviolet/mc"
)

var (
	defaultFastTimeout = 5 * time.Millisecond
	loginHs            = mc.ServerBoundHandshake{
		ProtocolVersion: 1,
		ServerAddress:   "Ultraviolet",
		ServerPort:      25565,
		NextState:       byte(mc.Login),
	}
	loginHsPk     = loginHs.Marshal()
	loginSecondPk = mc.ServerLoginStart{
		Name: "drago",
	}.Marshal()

	statusHs = mc.ServerBoundHandshake{
		ProtocolVersion: 1,
		ServerAddress:   "Ultraviolet",
		ServerPort:      25565,
		NextState:       byte(mc.Status),
	}
	statusHsPk     = statusHs.Marshal()
	statusSecondPk = mc.ServerBoundRequest{}.Marshal()
)

func TestReadStuff(t *testing.T) {
	tt := []struct {
		name            string
		pksSend         []mc.Packet
		expectedError   error
		expectNoError   bool
		compareReqData  bool
		expectedReqData core.RequestData
	}{
		{
			name:          "nothing to read",
			pksSend:       []mc.Packet{},
			expectedError: core.ErrClientToSlow,
		},
		{
			name:           "normal flow login",
			pksSend:        []mc.Packet{loginHsPk, loginSecondPk},
			expectNoError:  true,
			compareReqData: true,
			expectedReqData: core.RequestData{
				Type:       mc.Login,
				Handshake:  loginHs,
				ServerAddr: "ultraviolet",
				Username:   "drago",
			},
		},
		{
			name:           "normal flow status",
			pksSend:        []mc.Packet{statusHsPk, statusSecondPk},
			expectNoError:  true,
			compareReqData: true,
			expectedReqData: core.RequestData{
				Type:       mc.Status,
				Handshake:  statusHs,
				ServerAddr: "ultraviolet",
			},
		},
		{
			name:          "timeout reading second packet with login",
			pksSend:       []mc.Packet{loginHsPk},
			expectedError: core.ErrClientToSlow,
		},
		{
			name:          "timeout reading second packet with status",
			pksSend:       []mc.Packet{statusHsPk},
			expectedError: core.ErrClientToSlow,
		},
		{
			name: "unknown handshake state",
			pksSend: []mc.Packet{mc.ServerBoundHandshake{
				NextState: 3,
			}.Marshal()},
			expectedError: core.ErrNotValidHandshake,
		},
	}

	for _, tc := range tt {
		t.Run(tc.name, func(t *testing.T) {
			ultraviolet.ConnTimeoutDuration = defaultFastTimeout
			c1, c2 := net.Pipe()
			if tc.expectedReqData.ServerAddr != "" {
				tc.expectedReqData.Addr = c1.RemoteAddr()
			}

			go func() {
				mcConn := mc.NewMcConn(c2)
				for _, pk := range tc.pksSend {
					mcConn.WritePacket(pk)
				}
			}()

			req, err := ultraviolet.ReadStuff(c1)
			if err != nil {
				if tc.expectNoError {
					t.Fatalf("got unexpected error while reading: %v", err)
				}
				if errors.Is(err, tc.expectedError) {
					t.Log("received expected error")
				} else {
					t.Errorf("got different error then expected, got: %#v", err)
				}

			}

			if tc.compareReqData && !cmp.Equal(req, tc.expectedReqData) {
				t.Errorf("received difference in data...\ngot:    %#v\nexpect: %#v", req, tc.expectedReqData)
			}

		})
	}
}

func TestLookupServer(t *testing.T) {
	defaultReqData := core.RequestData{
		Type:       mc.Login,
		Handshake:  loginHs,
		ServerAddr: "Ultraviolet",
		Addr:       &net.IPAddr{},
		Username:   "drago",
	}
	notRegisteredServerReqData := core.RequestData{
		Type:       mc.Login,
		Handshake:  loginHs,
		ServerAddr: "uv",
		Addr:       &net.IPAddr{},
		Username:   "drago",
	}

	simpleServer := ultraviolet.ProxyAllServer{}
	serverCatalog := core.NewEmptyServerCatalog(mc.Packet{}, mc.Packet{})
	serverCatalog.ServerDict["ultraviolet"] = simpleServer

	tt := []struct {
		name           string
		reqData        core.RequestData
		expectedError  error
		expectNoError  bool
		compareServer  bool
		expectedServer core.Server
	}{
		{
			name:           "default flow",
			reqData:        defaultReqData,
			expectNoError:  true,
			compareServer:  true,
			expectedServer: simpleServer,
		},
		{
			name:          "cant find server",
			reqData:       notRegisteredServerReqData,
			expectedError: core.ErrNoServerFound,
		},
	}

	for _, tc := range tt {
		t.Run(tc.name, func(t *testing.T) {

			server, err := ultraviolet.LookupServer(tc.reqData, &serverCatalog)
			if err != nil {
				if tc.expectNoError {
					t.Fatalf("got unexpected error while reading: %v", err)
				}
				if errors.Is(err, tc.expectedError) && !tc.expectNoError {
					t.Log("received expected error")
				} else {
					t.Errorf("got different error then expected, got: %v", err)
				}
			}

			if tc.compareServer && !cmp.Equal(server, tc.expectedServer) {
				t.Errorf("received difference in data...\ngot:    %#v\nexpect: %#v", server, tc.expectedServer)
			}
		})
	}
}

func TestSendResponse(t *testing.T) {
	disconnectPk := mc.ClientBoundDisconnect{
		Reason: "Nope",
	}.Marshal()

	statusResponsePk := mc.SimpleStatus{
		Name: "Simple Status",
	}.Marshal()

	tt := []struct {
		name                  string
		expectedError         error
		expectNoError         bool
		compareReceivedPks    bool
		expectedPk            mc.Packet
		withPing              bool
		breakReadingConnAfter int
	}{
		{
			name:          "disconnect flow",
			expectNoError: true,
			expectedPk:    disconnectPk,
		},
		{
			name:          "respond with server status with ping",
			expectNoError: true,
			withPing:      true,
			expectedPk:    statusResponsePk,
		},
		{
			name:                  "close conn after initial request",
			expectNoError:         true,
			expectedPk:            statusResponsePk,
			breakReadingConnAfter: 1,
		},
	}

	for _, tc := range tt {
		t.Run(tc.name, func(t *testing.T) {
			t.Run("sending - "+tc.name, func(t *testing.T) {
				c1, c2 := net.Pipe()
				go func() {
					mcConn := mc.NewMcConn(c2)
					mcConn.ReadPacket()

					if !tc.withPing {
						c2.Close()
						return
					}

					mcConn.WritePacket(mc.NewServerBoundPing().Marshal())

					mcConn.ReadPacket()
				}()

				err := ultraviolet.SendResponse(c1, tc.expectedPk, tc.withPing)
				if err != nil {
					if tc.expectNoError {
						t.Fatalf("got unexpected error while reading: %v", err)
					}
					if errors.Is(err, tc.expectedError) && !tc.expectNoError {
						t.Log("received expected error")
					} else {
						t.Errorf("got different error then expected, got: %v", err)
					}
				}
			})

			t.Run("reading - "+tc.name, func(t *testing.T) {
				c1, c2 := net.Pipe()
				mcConn := mc.NewMcConn(c2)
				go ultraviolet.SendResponse(c1, tc.expectedPk, tc.withPing)

				pk, err := mcConn.ReadPacket()
				if err != nil {
					t.Errorf("received error while reading: %v", err)
				}
				if !cmp.Equal(pk, tc.expectedPk) {
					t.Errorf("received difference in data...\ngot:    %#v\nexpect: %#v", pk, tc.expectedPk)
				}

				if !tc.withPing {
					c2.Close()
				} else {
					pingPk := mc.NewServerBoundPing().Marshal()
					mcConn.WritePacket(pingPk)

					pk, err := mcConn.ReadPacket()
					if err != nil {
						t.Errorf("received error while reading: %v", err)
					}

					if !cmp.Equal(pk, pingPk) {
						t.Errorf("received difference in data...\ngot:    %#v\nexpect: %#v", pk, pingPk)
					}
				}

				// Currently not expecting this to close connection
				// if _, err := c2.Write([]byte{1}); !errors.Is(err, io.ErrClosedPipe) {
				// 	t.Errorf("expected connection to be closed, but wasnt")
				// }
			})

		})
	}
}

func TestFullRun(t *testing.T) {
	proxyServer := ultraviolet.ProxyAllServer{}
	serverCatalog := core.NewEmptyServerCatalog(mc.Packet{}, mc.Packet{})
	serverCatalog.ServerDict["ultraviolet"] = proxyServer

	pksSend := []mc.Packet{loginHsPk, loginSecondPk}

	c1, c2 := net.Pipe()

	go func() {
		mcConn := mc.NewMcConn(c2)
		for _, pk := range pksSend {
			mcConn.WritePacket(pk)
		}
	}()

	err := ultraviolet.FullRun(c1, &serverCatalog)

	if err != nil {
		t.Errorf("Didnt expect error: %v", err)
	}
}

func processRequestBenching(b *testing.B, serverCfgs []config.APIServerConfig, newReqData func() core.RequestData) {
	servers := make(map[string]core.Server)

	for _, cfg := range serverCfgs {
		server := ultraviolet.NewAPIServer(cfg)
		for _, domain := range cfg.Domains {
			servers[domain] = server
		}
	}

	servercatalog := core.NewServerCatalog(servers, defaultStatusPk, mc.Packet{})

	for i := 0; i < b.N; i++ {
		reqData := newReqData()

		ultraviolet.ProcessRequest(reqData, servercatalog)
	}
}

var defaultBenchServerCfg = config.APIServerConfig{
	ID:                "bench-server",
	Domains:           []string{"localhost", "127.0.0.1", "ultraviolet"},
	ProxyTo:           ":25566",
	DialTimeout:       "1s",
	IsOnline:          false,
	DisconnectMessage: "No login pls, thnx",
	UseStatusCache:    true,
	CachedStatus:      defaultStatus,
}

func BenchmarkProcessRequest_Internal(b *testing.B) {
	serverCfg := defaultBenchServerCfg

	newReqData := func() core.RequestData {
		return core.RequestData{
			Type:       mc.Status,
			Handshake:  statusHs,
			ServerAddr: "localhost",
			Addr:       &net.IPAddr{},
		}
	}

	processRequestBenching(b, []config.APIServerConfig{serverCfg}, newReqData)
}

func BenchmarkProcessRequest_WithBotLimiter(b *testing.B) {
	serverCfg := defaultBenchServerCfg
	serverCfg.LimitBots = true

	newReqData := func() core.RequestData {
		return core.RequestData{
			Type:       mc.Status,
			Handshake:  statusHs,
			ServerAddr: "localhost",
			Addr:       &net.IPAddr{},
		}
	}

	processRequestBenching(b, []config.APIServerConfig{serverCfg}, newReqData)
}

func BenchmarkFullRun_NetPipe(b *testing.B) {
	serverCfgs := []config.APIServerConfig{defaultBenchServerCfg}
	servers := make(map[string]core.Server)

	for _, cfg := range serverCfgs {
		server := ultraviolet.NewAPIServer(cfg)
		for _, domain := range cfg.Domains {
			servers[domain] = server
		}
	}

	servercatalog := core.NewServerCatalog(servers, defaultStatusPk, mc.Packet{})

	for i := 0; i < b.N; i++ {
		c1, c2 := net.Pipe()

		go func() {
			mcConn := mc.NewMcConn(c1)
			mcConn.WritePacket(loginHsPk)
			mcConn.WritePacket(loginSecondPk)
			mcConn.ReadPacket()
		}()

		err := ultraviolet.FullRun(c2, servercatalog)
		if errors.Is(err, core.ErrNoServerFound) {
			b.Fail()
		}
	}
}
