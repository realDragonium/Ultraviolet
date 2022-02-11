package module_test

import (
	"fmt"
	"net"
	"testing"
	"time"

	"github.com/realDragonium/Ultraviolet/core"
	"github.com/realDragonium/Ultraviolet/mc"
	"github.com/realDragonium/Ultraviolet/module"
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
			req := core.RequestData{}
			connLimiter := module.NewAbsConnLimiter(tc.limit, tc.cooldown, tc.limitStatus)

			for i := 0; i < tc.limit; i++ {
				ok, _ := connLimiter.Allow(req)
				if !ok {
					t.Error("expected ok to be true but its false")
				}
			}
			ok, _ := connLimiter.Allow(req)
			if ok == tc.shouldLimit {
				t.Error("expected ok to be false but its true")
			}
		})
	}

}

func TestAbsoluteConnLimiter_AllowsNewConnectionsAfterCooldown(t *testing.T) {
	limit := 5
	cooldown := time.Millisecond
	req := core.RequestData{}
	connLimiter := module.NewAbsConnLimiter(limit, cooldown, true)

	for i := 0; i < limit+1; i++ {
		connLimiter.Allow(req)
	}

	time.Sleep(cooldown)
	ok, _ := connLimiter.Allow(req)
	if !ok {
		t.Error("expected ok to be true but its false")
	}

}

func TestAlwaysAllowConnection(t *testing.T) {
	limiter := module.AlwaysAllowConnection{}
	req := core.RequestData{}

	ok, _ := limiter.Allow(req)
	if !ok {
		t.Error("expected ok to be true but its false")
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
				connLimiter := module.NewBotFilterConnLimiter(tc.rateLimit, tc.cooldown, listClearTime, normalUnverifyCooldown, rateDisconPk)
				playerAddr := net.IPAddr{
					IP: net.IPv4(10, 10, 10, 10),
				}
				playerName := "backend"
				req := core.RequestData{
					Type:      tc.reqType,
					Handshake: mc.ServerBoundHandshake{},
					Addr:      &playerAddr,
					Username:  playerName,
				}

				ok, _ := connLimiter.Allow(req)
				if ok != tc.allowed {
					t.Errorf("expected: %v, got: %v", tc.allowed, ok)
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
		connLimiter := module.NewBotFilterConnLimiter(ratelimit, cooldown, listClearTime, normalUnverifyCooldown, disconPk)
		playerAddr := net.IPAddr{
			IP: net.IPv4(10, 10, 10, 10),
		}
		playerName := "backend"
		req := core.RequestData{
			Type:      mc.Login,
			Handshake: mc.ServerBoundHandshake{},
			Addr:      &playerAddr,
			Username:  playerName,
		}
		connLimiter.Allow(req)
		time.Sleep(cooldown)
		ok, _ := connLimiter.Allow(req)
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
		connLimiter := module.NewBotFilterConnLimiter(ratelimit, cooldown, normalUnverifyCooldown, listClearTime, disconPk)
		req := core.RequestData{
			Type:     mc.Login,
			Addr:     generateIPAddr(),
			Username: "backend",
		}
		connLimiter.Allow(req)
		req.Addr = generateIPAddr()
		connLimiter.Allow(req)
		time.Sleep(cooldown)
		req.Addr = generateIPAddr()
		ok, _ := connLimiter.Allow(req)
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
		connLimiter := module.NewBotFilterConnLimiter(ratelimit, cooldown, listClearTime, unverifyCooldown, disconPk)
		req := core.RequestData{
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
		ok, _ := connLimiter.Allow(req)
		if !ok {
			t.Fatal("expected to be allowed but it was denied")
		}
	})

	t.Run("over rate limit allows second login request", func(t *testing.T) {
		ratelimit := 0
		connLimiter := module.NewBotFilterConnLimiter(ratelimit, normalCooldown, listClearTime, normalUnverifyCooldown, rateDisconPk)
		playerAddr := net.IPAddr{
			IP: net.IPv4(10, 10, 10, 10),
		}
		playerName := "backend"
		req := core.RequestData{
			Type:      mc.Login,
			Handshake: mc.ServerBoundHandshake{},
			Addr:      &playerAddr,
			Username:  playerName,
		}
		connLimiter.Allow(req)
		ok, _ := connLimiter.Allow(req)
		if !ok {
			t.Fatal("expected to be allowed but it was denied")
		}
	})

	t.Run("second request with different name gets denied", func(t *testing.T) {
		ratelimit := 0
		connLimiter := module.NewBotFilterConnLimiter(ratelimit, normalCooldown, listClearTime, normalUnverifyCooldown, rateDisconPk)
		playerAddr := net.IPAddr{
			IP: net.IPv4(10, 10, 10, 10),
		}
		playerName1 := "backend"
		playerName2 := "uv"
		req := core.RequestData{
			Type:      mc.Login,
			Handshake: mc.ServerBoundHandshake{},
			Addr:      &playerAddr,
			Username:  playerName1,
		}
		req2 := req
		req2.Username = playerName2
		connLimiter.Allow(req)
		ok, _ := connLimiter.Allow(req2)
		if ok {
			t.Fatal("expected to be denied but it was allowed")
		}
	})

	t.Run("when blocked extra request gets denied", func(t *testing.T) {
		ratelimit := 0
		connLimiter := module.NewBotFilterConnLimiter(ratelimit, normalCooldown, listClearTime, normalUnverifyCooldown, rateDisconPk)
		playerAddr := net.IPAddr{
			IP: net.IPv4(10, 10, 10, 10),
		}
		playerName1 := "backend"
		playerName2 := "uv"
		req := core.RequestData{
			Type:      mc.Login,
			Handshake: mc.ServerBoundHandshake{},
			Addr:      &playerAddr,
			Username:  playerName1,
		}
		req2 := req
		req2.Username = playerName2
		connLimiter.Allow(req)
		connLimiter.Allow(req2)
		ok, _ := connLimiter.Allow(req)
		if ok {
			t.Fatal("expected to be denied but it was allowed")
		}
	})

	t.Run("when blocked after normal cooldown still blocked", func(t *testing.T) {
		ratelimit := 1
		cooldown := time.Millisecond
		connLimiter := module.NewBotFilterConnLimiter(ratelimit, cooldown, listClearTime, normalUnverifyCooldown, rateDisconPk)
		playerAddr := net.IPAddr{
			IP: net.IPv4(10, 10, 10, 10),
		}
		playerName1 := "backend"
		playerName2 := "uv"
		req := core.RequestData{
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
		ok, _ := connLimiter.Allow(req)
		if ok {
			t.Fatal("expected to be denied but it was allowed")
		}
	})

	t.Run("is NOT blocked anymore after listClear & unverify cooldown", func(t *testing.T) {
		unverifyCooldown := 10 * time.Microsecond
		ratelimit := 1
		cooldown := time.Millisecond
		listClearCooldown := 10 * time.Millisecond
		connLimiter := module.NewBotFilterConnLimiter(ratelimit, cooldown, listClearTime, unverifyCooldown, rateDisconPk)
		playerName1 := "backend"
		playerName2 := "uv"
		req := core.RequestData{
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
		ok, _ := connLimiter.Allow(req)
		if !ok {
			t.Fatal("expected to be allowed but it was denied")
		}
	})
}
