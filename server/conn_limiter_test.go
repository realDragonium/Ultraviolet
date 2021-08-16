package server_test

import (
	"fmt"
	"net"
	"testing"
	"time"

	"github.com/realDragonium/Ultraviolet/mc"
	"github.com/realDragonium/Ultraviolet/server"
)

func TestAbsoluteConnLimiter_DeniesWhenLimitIsReached(t *testing.T) {
	tt := []struct {
		limit       int
		limitStatus bool
		cooldown    time.Duration
		states      []mc.HandshakeState
		shouldLimit bool
	}{
		{
			limit:       3,
			limitStatus: true,
			cooldown:    time.Minute,
			states:      []mc.HandshakeState{mc.Login, mc.Status},
			shouldLimit: true,
		},
		{
			limit:       3,
			limitStatus: false,
			cooldown:    time.Minute,
			states:      []mc.HandshakeState{mc.Status},
			shouldLimit: false,
		},
	}

	for _, tc := range tt {
		name := fmt.Sprintf("limits on: %v, limit status: %v, cooldown: %v", tc.limit, tc.limitStatus, tc.cooldown)
		t.Run(name, func(t *testing.T) {
			req := server.BackendRequest{}
			connLimiter := server.NewAbsConnLimiter(tc.limit, tc.cooldown, tc.limitStatus)

			for i := 0; i < tc.limit; i++ {
				_, ok := connLimiter.Allow(req)
				if !ok {
					t.Error("expected ok to be true but its false")
				}
			}
			ans, ok := connLimiter.Allow(req)
			if ok == tc.shouldLimit {
				t.Error("expected ok to be false but its true")
			}
			if tc.shouldLimit && ans.Action() != server.Close {
				t.Error("Got a different answer then expected")
				t.Errorf("expected this answer: %v", server.NewCloseAnswer())
				t.Errorf("got this instead: %v", ans)
			}
		})
	}

}

func TestAbsoluteConnLimiter_AllowsNewConnectionsAfterCooldown(t *testing.T) {
	limit := 5
	cooldown := time.Millisecond
	req := server.BackendRequest{}
	connLimiter := server.NewAbsConnLimiter(limit, cooldown, true)

	for i := 0; i < limit+1; i++ {
		connLimiter.Allow(req)
	}

	time.Sleep(cooldown)
	ans, ok := connLimiter.Allow(req)
	if !ok {
		t.Error("expected ok to be true but its false")
	}
	if ans.Action() != server.Error {
		t.Errorf("expected answer to be empty but its not? %v", ans)
	}

}

func TestAlwaysAllowConnection(t *testing.T) {
	limiter := server.AlwaysAllowConnection{}
	req := server.BackendRequest{}

	ans, ok := limiter.Allow(req)
	if !ok {
		t.Error("expected ok to be true but its false")
	}
	if ans.Action() != server.Error {
		t.Errorf("expected answer to be empty but its not? %v", ans)
	}

}

// dont expect to need to generate more than 256 ip addresses
var counter = 0

func generateIPAddr() net.Addr {
	counter++
	return &net.IPAddr{
		IP: net.IPv4(1, 1, 1, byte(counter)),
	}
}

func TestBotFilterConnLimiter(t *testing.T) {
	disconMsg := "You have been disconnected"
	rateDisconPk := mc.ClientBoundDisconnect{
		Reason: mc.String(disconMsg),
	}.Marshal()
	listClearTime := time.Minute
	normalCooldown := time.Second
	normalUnverifyCooldown := 10 * time.Second

	t.Run("testing rate limit border", func(t *testing.T) {
		tt := []struct {
			rateLimit int
			cooldown  time.Duration
			reqType   mc.HandshakeState
			allowed   bool
		}{
			{
				rateLimit: 2,
				cooldown:  time.Second,
				reqType:   mc.Login,
				allowed:   true,
			},
			{
				rateLimit: 0,
				cooldown:  time.Second,
				reqType:   mc.Status,
				allowed:   true,
			},
			{
				rateLimit: 1,
				cooldown:  time.Second,
				reqType:   mc.Login,
				allowed:   true,
			},
			{
				rateLimit: 0,
				cooldown:  time.Second,
				reqType:   mc.Login,
				allowed:   false,
			},
		}
		for _, tc := range tt {
			name := fmt.Sprintf("type:%v allowed:%v ratelimit:%d", tc.reqType, tc.allowed, tc.rateLimit)
			t.Run(name, func(t *testing.T) {
				connLimiter := server.NewBotFilterConnLimiter(tc.rateLimit, tc.cooldown, listClearTime, normalUnverifyCooldown, rateDisconPk)
				playerAddr := net.IPAddr{
					IP: net.IPv4(10, 10, 10, 10),
				}
				playerName := "backend"
				req := server.BackendRequest{
					Type:      tc.reqType,
					Handshake: mc.ServerBoundHandshake{},
					Addr:      &playerAddr,
					Username:  playerName,
				}

				ans, ok := connLimiter.Allow(req)
				if ok != tc.allowed {
					t.Errorf("expected: %v, got: %v", tc.allowed, ok)
				}
				if tc.allowed {
					return
				}
				if ans.Action() != server.Disconnect {
					t.Errorf("got %v - wanted %v", ans.Action(), server.Disconnect)
				}
				receivedPk := ans.Response()
				disconPacket, err := mc.UnmarshalClientDisconnect(receivedPk)
				if err != nil {
					t.Fatalf("got error: %v", err)
				}
				if disconPacket.Reason != mc.String(disconMsg) {
					t.Error("Received different notification than expected")
					t.Logf("expected: %v", disconMsg)
					t.Logf("got: %v", disconPacket.Reason)
				}
			})
		}
	})

	t.Run("clears counter when cooldown is over", func(t *testing.T) {
		ratelimit := 1
		cooldown := time.Millisecond
		disconMsg := "You have been disconnected with the server"
		disconPk := mc.ClientBoundDisconnect{
			Reason: mc.String(disconMsg),
		}.Marshal()
		connLimiter := server.NewBotFilterConnLimiter(ratelimit, cooldown, listClearTime, normalUnverifyCooldown, disconPk)
		playerAddr := net.IPAddr{
			IP: net.IPv4(10, 10, 10, 10),
		}
		playerName := "backend"
		req := server.BackendRequest{
			Type:      mc.Login,
			Handshake: mc.ServerBoundHandshake{},
			Addr:      &playerAddr,
			Username:  playerName,
		}
		connLimiter.Allow(req)
		time.Sleep(cooldown)
		_, ok := connLimiter.Allow(req)
		if !ok {
			t.Fatal("expected to be allowed but it was denied")
		}
	})

	t.Run("when over ratelimit still limits after cooldown", func(t *testing.T) {
		ratelimit := 1
		cooldown := time.Millisecond
		disconPk := mc.ClientBoundDisconnect{
			Reason: "You have been disconnected with the server",
		}.Marshal()
		connLimiter := server.NewBotFilterConnLimiter(ratelimit, cooldown, normalUnverifyCooldown, listClearTime, disconPk)
		req := server.BackendRequest{
			Type:     mc.Login,
			Addr:     generateIPAddr(),
			Username: "backend",
		}
		connLimiter.Allow(req)
		req.Addr = generateIPAddr()
		connLimiter.Allow(req)
		time.Sleep(cooldown)
		req.Addr = generateIPAddr()
		_, ok := connLimiter.Allow(req)
		if ok {
			t.Fatal("expected to be limited but it was allowed")
		}
	})

	t.Run("when over ratelimit allow unverified connections again after cooldown", func(t *testing.T) {
		unverifyCooldown := time.Millisecond
		ratelimit := 1
		cooldown := time.Millisecond
		disconPk := mc.ClientBoundDisconnect{
			Reason: "You have been disconnected with the server",
		}.Marshal()
		connLimiter := server.NewBotFilterConnLimiter(ratelimit, cooldown, listClearTime, unverifyCooldown, disconPk)
		req := server.BackendRequest{
			Type:     mc.Login,
			Addr:     generateIPAddr(),
			Username: "backend",
		}
		connLimiter.Allow(req)
		req.Addr = generateIPAddr()
		connLimiter.Allow(req)

		time.Sleep(2 * cooldown)

		req.Addr = generateIPAddr()
		connLimiter.Allow(req)

		time.Sleep(2 * unverifyCooldown)
		req.Addr = generateIPAddr()
		_, ok := connLimiter.Allow(req)
		if !ok {
			t.Fatal("expected to be allowed but it was denied")
		}
	})

	t.Run("over rate limit allows second login request", func(t *testing.T) {
		ratelimit := 0
		connLimiter := server.NewBotFilterConnLimiter(ratelimit, normalCooldown, listClearTime, normalUnverifyCooldown, rateDisconPk)
		playerAddr := net.IPAddr{
			IP: net.IPv4(10, 10, 10, 10),
		}
		playerName := "backend"
		req := server.BackendRequest{
			Type:      mc.Login,
			Handshake: mc.ServerBoundHandshake{},
			Addr:      &playerAddr,
			Username:  playerName,
		}
		connLimiter.Allow(req)
		_, ok := connLimiter.Allow(req)
		if !ok {
			t.Fatal("expected to be allowed but it was denied")
		}
	})

	t.Run("second request with different name gets denied", func(t *testing.T) {
		ratelimit := 0
		connLimiter := server.NewBotFilterConnLimiter(ratelimit, normalCooldown, listClearTime, normalUnverifyCooldown, rateDisconPk)
		playerAddr := net.IPAddr{
			IP: net.IPv4(10, 10, 10, 10),
		}
		playerName1 := "backend"
		playerName2 := "uv"
		req := server.BackendRequest{
			Type:      mc.Login,
			Handshake: mc.ServerBoundHandshake{},
			Addr:      &playerAddr,
			Username:  playerName1,
		}
		req2 := req
		req2.Username = playerName2
		connLimiter.Allow(req)
		ans, ok := connLimiter.Allow(req2)
		if ok {
			t.Fatal("expected to be denied but it was allowed")
		}
		if ans.Action() != server.Close {
			t.Errorf("got %v - wanted %v", ans.Action(), server.Close)
		}
	})

	t.Run("when blocked extra request gets denied", func(t *testing.T) {
		ratelimit := 0
		connLimiter := server.NewBotFilterConnLimiter(ratelimit, normalCooldown, listClearTime, normalUnverifyCooldown, rateDisconPk)
		playerAddr := net.IPAddr{
			IP: net.IPv4(10, 10, 10, 10),
		}
		playerName1 := "backend"
		playerName2 := "uv"
		req := server.BackendRequest{
			Type:      mc.Login,
			Handshake: mc.ServerBoundHandshake{},
			Addr:      &playerAddr,
			Username:  playerName1,
		}
		req2 := req
		req2.Username = playerName2
		connLimiter.Allow(req)
		connLimiter.Allow(req2)
		_, ok := connLimiter.Allow(req)
		if ok {
			t.Fatal("expected to be denied but it was allowed")
		}
	})

	t.Run("when blocked after normal cooldown still blocked", func(t *testing.T) {
		ratelimit := 1
		cooldown := time.Millisecond
		connLimiter := server.NewBotFilterConnLimiter(ratelimit, cooldown, listClearTime, normalUnverifyCooldown, rateDisconPk)
		playerAddr := net.IPAddr{
			IP: net.IPv4(10, 10, 10, 10),
		}
		playerName1 := "backend"
		playerName2 := "uv"
		req := server.BackendRequest{
			Type:      mc.Login,
			Handshake: mc.ServerBoundHandshake{},
			Addr:      &playerAddr,
			Username:  playerName1,
		}
		req2 := req
		req2.Username = playerName2
		connLimiter.Allow(req)
		connLimiter.Allow(req)
		connLimiter.Allow(req2)
		time.Sleep(cooldown * 2)
		_, ok := connLimiter.Allow(req)
		if ok {
			t.Fatal("expected to be denied but it was allowed")
		}
	})

	t.Run("is NOT blocked anymore after listClear & unverify cooldown", func(t *testing.T) {
		unverifyCooldown := 10 * time.Microsecond
		ratelimit := 1
		cooldown := time.Millisecond
		listClearCooldown := 10 * time.Millisecond
		connLimiter := server.NewBotFilterConnLimiter(ratelimit, cooldown, listClearTime, unverifyCooldown, rateDisconPk)
		playerName1 := "backend"
		playerName2 := "uv"
		req := server.BackendRequest{
			Type:      mc.Login,
			Handshake: mc.ServerBoundHandshake{},
			Addr:      generateIPAddr(),
			Username:  playerName1,
		}
		req2 := req
		req2.Username = playerName2
		connLimiter.Allow(req)
		connLimiter.Allow(req2)
		time.Sleep(2 * listClearCooldown)
		t.Log("last check ----------------")
		_, ok := connLimiter.Allow(req)
		if !ok {
			t.Fatal("expected to be allowed but it was denied")
		}
	})
}
