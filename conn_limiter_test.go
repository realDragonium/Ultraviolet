package ultraviolet_test

import (
	"fmt"
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
