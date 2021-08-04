package ultraviolet

import "time"

type ConnectionLimiter interface {
	// The process answer is empty and should be ignored when
	//  it does allow the connection to happen
	Allow(req BackendRequest) (ProcessAnswer, bool)
}

func NewAbsConnLimiter(ratelimit int, cooldown time.Duration, limitStatus bool) ConnectionLimiter {
	return &absoluteConnlimiter{
		rateLimit:    ratelimit,
		rateCooldown: cooldown,
		limitStatus:  limitStatus,
	}
}

type absoluteConnlimiter struct {
	rateCounter   int
	rateStartTime time.Time
	rateLimit     int
	rateCooldown  time.Duration
	limitStatus   bool
}

func (r *absoluteConnlimiter) Allow(req BackendRequest) (ProcessAnswer, bool) {
	if time.Since(r.rateStartTime) >= r.rateCooldown {
		r.rateCounter = 0
		r.rateStartTime = time.Now()
	}
	if !r.limitStatus {
		return ProcessAnswer{}, true
	}
	if r.rateCounter < r.rateLimit {
		r.rateCounter++
		return ProcessAnswer{}, true
	}
	return NewCloseAnswer(), false
}

type AlwaysAllowConnection struct{}

func (limiter AlwaysAllowConnection) Allow(req BackendRequest) (ProcessAnswer, bool) {
	return ProcessAnswer{}, true
}
