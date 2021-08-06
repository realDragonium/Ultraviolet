package ultraviolet_test

import (
	"fmt"
	"net"
	"testing"
	"time"

	ultraviolet "github.com/realDragonium/Ultraviolet"
	"github.com/realDragonium/Ultraviolet/mc"
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
			states:      []mc.HandshakeState{mc.LOGIN, mc.STATUS},
			shouldLimit: true,
		},
		{
			limit:       3,
			limitStatus: false,
			cooldown:    time.Minute,
			states:      []mc.HandshakeState{mc.STATUS},
			shouldLimit: false,
		},
	}

	for _, tc := range tt {
		name := fmt.Sprintf("limits on: %v, limit status: %v, cooldown: %v", tc.limit, tc.limitStatus, tc.cooldown)
		t.Run(name, func(t *testing.T) {
			req := ultraviolet.BackendRequest{}
			connLimiter := ultraviolet.NewAbsConnLimiter(tc.limit, tc.cooldown, tc.limitStatus)

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
			if tc.shouldLimit && ans.Action() != ultraviolet.CLOSE {
				t.Error("Got a different answer then expected")
				t.Errorf("expected this answer: %v", ultraviolet.NewCloseAnswer())
				t.Errorf("got this instead: %v", ans)
			}
		})
	}

}

func TestAbsoluteConnLimiter_AllowsNewConnectionsAfterCooldown(t *testing.T) {
	limit := 5
	cooldown := time.Millisecond
	req := ultraviolet.BackendRequest{}
	connLimiter := ultraviolet.NewAbsConnLimiter(limit, cooldown, true)

	for i := 0; i < limit+1; i++ {
		connLimiter.Allow(req)
	}

	time.Sleep(cooldown)
	ans, ok := connLimiter.Allow(req)
	if !ok {
		t.Error("expected ok to be true but its false")
	}
	if ans.Action() != ultraviolet.ERROR {
		t.Errorf("expected answer to be empty but its not? %v", ans)
	}

}

func TestAlwaysAllowConnection(t *testing.T) {
	limiter := ultraviolet.AlwaysAllowConnection{}
	req := ultraviolet.BackendRequest{}

	ans, ok := limiter.Allow(req)
	if !ok {
		t.Error("expected ok to be true but its false")
	}
	if ans.Action() != ultraviolet.ERROR {
		t.Errorf("expected answer to be empty but its not? %v", ans)
	}

}

func TestBotFilterConnLimiter(t *testing.T) {
	disconMsg := "You have been disconnected"
	rateDisconPk := mc.ClientBoundDisconnect{
		Reason: mc.String(disconMsg),
	}.Marshal()
	listClearTime := time.Minute
	normalCooldown := time.Second
	tt := []struct {
		rateLimit int
		cooldown  time.Duration
		reqType   mc.HandshakeState
		allowed   bool
	}{
		{
			rateLimit: 2,
			cooldown:  time.Second,
			reqType:   mc.LOGIN,
			allowed:   true,
		},
		{
			rateLimit: 0,
			cooldown:  time.Second,
			reqType:   mc.STATUS,
			allowed:   true,
		},
		{
			rateLimit: 1,
			cooldown:  time.Second,
			reqType:   mc.LOGIN,
			allowed:   true,
		},
		{
			rateLimit: 0,
			cooldown:  time.Second,
			reqType:   mc.LOGIN,
			allowed:   false,
		},
	}
	for _, tc := range tt {
		name := fmt.Sprintf("type:%v allowed:%v ratelimit:%d", tc.reqType, tc.allowed, tc.rateLimit)
		t.Run(name, func(t *testing.T) {
			connLimiter := ultraviolet.NewBotFilterConnLimiter(tc.rateLimit, tc.cooldown, listClearTime, rateDisconPk)
			playerAddr := net.IPAddr{
				IP: net.IPv4(10, 10, 10, 10),
			}
			playerName := "ultraviolet"
			req := ultraviolet.BackendRequest{
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
			if ans.Action() != ultraviolet.DISCONNECT {
				t.Errorf("got %v - wanted %v", ans.Action(), ultraviolet.DISCONNECT)
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

	t.Run("clears counter when cooldown is over", func(t *testing.T) {
		ratelimit := 1
		cooldown := time.Millisecond
		disconMsg := "You have been disconnected with the server"
		disconPk := mc.ClientBoundDisconnect{
			Reason: mc.String(disconMsg),
		}.Marshal()
		connLimiter := ultraviolet.NewBotFilterConnLimiter(ratelimit, cooldown, listClearTime, disconPk)
		playerAddr := net.IPAddr{
			IP: net.IPv4(10, 10, 10, 10),
		}
		playerName := "ultraviolet"
		req := ultraviolet.BackendRequest{
			Type:      mc.LOGIN,
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

	t.Run("over rate limit allows second login request", func(t *testing.T) {
		ratelimit := 0
		connLimiter := ultraviolet.NewBotFilterConnLimiter(ratelimit, normalCooldown, listClearTime, rateDisconPk)
		playerAddr := net.IPAddr{
			IP: net.IPv4(10, 10, 10, 10),
		}
		playerName := "ultraviolet"
		req := ultraviolet.BackendRequest{
			Type:      mc.LOGIN,
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
		connLimiter := ultraviolet.NewBotFilterConnLimiter(ratelimit, normalCooldown, listClearTime, rateDisconPk)
		playerAddr := net.IPAddr{
			IP: net.IPv4(10, 10, 10, 10),
		}
		playerName1 := "ultraviolet"
		playerName2 := "uv"
		req := ultraviolet.BackendRequest{
			Type:      mc.LOGIN,
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
		if ans.Action() != ultraviolet.CLOSE {
			t.Errorf("got %v - wanted %v", ans.Action(), ultraviolet.CLOSE)
		}
	})

	t.Run("when blocked extra request gets denied", func(t *testing.T) {
		ratelimit := 0
		connLimiter := ultraviolet.NewBotFilterConnLimiter(ratelimit, normalCooldown, listClearTime, rateDisconPk)
		playerAddr := net.IPAddr{
			IP: net.IPv4(10, 10, 10, 10),
		}
		playerName1 := "ultraviolet"
		playerName2 := "uv"
		req := ultraviolet.BackendRequest{
			Type:      mc.LOGIN,
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
		connLimiter := ultraviolet.NewBotFilterConnLimiter(ratelimit, cooldown, listClearTime, rateDisconPk)
		playerAddr := net.IPAddr{
			IP: net.IPv4(10, 10, 10, 10),
		}
		playerName1 := "ultraviolet"
		playerName2 := "uv"
		req := ultraviolet.BackendRequest{
			Type:      mc.LOGIN,
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

	t.Run("is blocked anymore after list clear cooldown", func(t *testing.T) {
		ratelimit := 1
		cooldown := time.Millisecond
		listClearCooldown := 10 * time.Millisecond
		connLimiter := ultraviolet.NewBotFilterConnLimiter(ratelimit, cooldown, listClearCooldown, rateDisconPk)
		playerAddr := net.IPAddr{
			IP: net.IPv4(10, 10, 10, 10),
		}
		playerName1 := "ultraviolet"
		playerName2 := "uv"
		req := ultraviolet.BackendRequest{
			Type:      mc.LOGIN,
			Handshake: mc.ServerBoundHandshake{},
			Addr:      &playerAddr,
			Username:  playerName1,
		}
		req2 := req
		req2.Username = playerName2
		connLimiter.Allow(req)
		connLimiter.Allow(req)
		connLimiter.Allow(req2)
		time.Sleep(listClearCooldown)
		_, ok := connLimiter.Allow(req)
		if !ok {
			t.Fatal("expected to be allowed but it was denied")
		}
	})
}
