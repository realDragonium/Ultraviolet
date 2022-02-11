package module

import (
	"net"
	"strings"
	"time"

	"github.com/realDragonium/Ultraviolet/core"
	"github.com/realDragonium/Ultraviolet/mc"
)

func FilterIpFromAddr(addr net.Addr) string {
	s := addr.String()
	parts := strings.Split(s, ":")
	return parts[0]
}

type ConnectionLimiter interface {
	Allow(req core.RequestData) (allowed bool, err error)
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

func (r *absoluteConnlimiter) Allow(req core.RequestData) (bool, error) {
	if time.Since(r.rateStartTime) >= r.rateCooldown {
		r.rateCounter = 0
		r.rateStartTime = time.Now()
	}
	if !r.limitStatus {
		return true, nil
	}
	if r.rateCounter < r.rateLimit {
		r.rateCounter++
		return true, nil
	}
	return false, core.ErrOverConnRateLimit
}

type AlwaysAllowConnection struct{}

func (limiter AlwaysAllowConnection) Allow(req core.RequestData) (bool, error) {
	return true, nil
}

func NewBotFilterConnLimiter(ratelimit int, cooldown, clearTime, unverify time.Duration, disconnPk mc.Packet) ConnectionLimiter {

	return &botFilterConnLimiter{
		lastTimeAboveLimit: time.Now(),
		unverifyCooldown:   unverify,
		rateLimit:          ratelimit,
		rateCooldown:       cooldown,
		disconnPacket:      disconnPk,
		listClearTime:      clearTime,

		namesList: make(map[string]string),
		blackList: make(map[string]time.Time),
	}
}

type botFilterConnLimiter struct {
	limiting         bool
	unverifyCooldown time.Duration

	rateCounter        int
	rateStartTime      time.Time
	lastTimeAboveLimit time.Time
	rateLimit          int
	rateCooldown       time.Duration
	disconnPacket      mc.Packet
	listClearTime      time.Duration

	blackList map[string]time.Time
	namesList map[string]string
}

// TODO: Fix hidden race condition (something with the timing of the lastTimeAboveLimit time)
func (l *botFilterConnLimiter) Allow(req core.RequestData) (allowed bool, err error) {
	if req.Type == mc.Status {
		allowed = true
		return
	}
	if time.Since(l.rateStartTime) >= l.rateCooldown {
		if l.rateCounter > l.rateLimit {
			l.lastTimeAboveLimit = l.rateStartTime
		}
		if l.limiting && time.Since(l.lastTimeAboveLimit) >= l.unverifyCooldown {
			l.limiting = false
		}
		l.rateCounter = 0
		l.rateStartTime = time.Now()
	}

	l.rateCounter++
	ip := FilterIpFromAddr(req.Addr)
	blockTime, ok := l.blackList[ip]
	if time.Since(blockTime) >= l.listClearTime {
		delete(l.blackList, ip)
	} else if ok {
		allowed = false
		return
	}

	l.limiting = l.limiting || l.rateCounter > l.rateLimit
	if l.limiting {
		username, ok := l.namesList[ip]
		if !ok {
			allowed = false
			l.namesList[ip] = req.Username
			return
		}
		if username != req.Username {
			allowed = false
			l.blackList[ip] = time.Now()
			return
		}
	}
	allowed = true
	return
}
