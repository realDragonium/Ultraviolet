package ultraviolet

import (
	"errors"
	"net"
	"strings"
	"time"

	"github.com/realDragonium/Ultraviolet/mc"
)

var ErrOverConnRateLimit = errors.New("too many request within rate limit time frame")

func FilterIpFromAddr(addr net.Addr) string {
	s := addr.String()
	parts := strings.Split(s, ":")
	return parts[0]
}

type ConnectionLimiter interface {
	// The process answer is empty and should be ignored when it does allow the connection to happen
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

func NewBotFilterConnLimiter(ratelimit int, cooldown, clearTime time.Duration, disconnPk mc.Packet) ConnectionLimiter {
	return &botFilterConnLimiter{
		rateLimit:     ratelimit,
		rateCooldown:  cooldown,
		disconnPacket: disconnPk,
		listClearTime: clearTime,

		namesList: make(map[string]string),
		blackList: make(map[string]time.Time),
	}
}

type botFilterConnLimiter struct {
	rateCounter   int
	rateStartTime time.Time
	rateLimit     int
	rateCooldown  time.Duration
	disconnPacket mc.Packet
	listClearTime time.Duration

	blackList map[string]time.Time
	namesList map[string]string
}

func (limiter *botFilterConnLimiter) Allow(req BackendRequest) (ProcessAnswer, bool) {
	if req.Type == mc.STATUS {
		return ProcessAnswer{}, true
	}
	if time.Since(limiter.rateStartTime) >= limiter.rateCooldown {
		limiter.rateCounter = 0
		limiter.rateStartTime = time.Now()
	}
	limiter.rateCounter++
	ip := FilterIpFromAddr(req.Addr)
	blockTime, ok := limiter.blackList[ip]
	if time.Since(blockTime) >= limiter.listClearTime {
		delete(limiter.blackList, ip)
	} else if ok {
		return NewCloseAnswer(), false
	}

	if limiter.rateCounter > limiter.rateLimit {
		username, ok := limiter.namesList[ip]
		if !ok {
			limiter.namesList[ip] = req.Username
			return NewDisconnectAnswer(limiter.disconnPacket), false
		}
		if username != req.Username {
			limiter.blackList[ip] = time.Now()
			return NewCloseAnswer(), false
		}
	}
	return ProcessAnswer{}, true
}
