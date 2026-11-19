package ratelimit

import (
	"math"
	"sync"
	"time"

	"github.com/termofeed/termofeed-back/internal/clock"
)

type IPRateLimiter struct {
	mu           sync.Mutex
	clock        clock.Clock
	requests     map[string][]time.Time
	roomCreates  map[string][]time.Time
	reqLimit     int
	reqWindow    time.Duration
	createLimit  int
	createWindow time.Duration
}

func NewIPRateLimiter(c clock.Clock) *IPRateLimiter {
	return &IPRateLimiter{
		clock:        c,
		requests:     make(map[string][]time.Time),
		roomCreates:  make(map[string][]time.Time),
		reqLimit:     120,
		reqWindow:    1 * time.Minute,
		createLimit:  10,
		createWindow: 1 * time.Hour,
	}
}

// AllowRequest checks whether an IP is allowed to make a general request (120 req/min).
func (l *IPRateLimiter) AllowRequest(ip string) (bool, int) {
	l.mu.Lock()
	defer l.mu.Unlock()

	now := l.clock.Now()
	threshold := now.Add(-l.reqWindow)

	// Filter out expired timestamps
	var valid []time.Time
	for _, t := range l.requests[ip] {
		if t.After(threshold) {
			valid = append(valid, t)
		}
	}

	if len(valid) >= l.reqLimit {
		oldest := valid[0]
		remaining := oldest.Add(l.reqWindow).Sub(now)
		retryAfter := int(math.Ceil(remaining.Seconds()))
		if retryAfter < 1 {
			retryAfter = 1
		}
		l.requests[ip] = valid
		return false, retryAfter
	}

	valid = append(valid, now)
	l.requests[ip] = valid
	return true, 0
}

// AllowCreateRoom checks whether an IP is allowed to create a new room (10 rooms/hour).
func (l *IPRateLimiter) AllowCreateRoom(ip string) (bool, int) {
	l.mu.Lock()
	defer l.mu.Unlock()

	now := l.clock.Now()
	threshold := now.Add(-l.createWindow)

	var valid []time.Time
	for _, t := range l.roomCreates[ip] {
		if t.After(threshold) {
			valid = append(valid, t)
		}
	}

	if len(valid) >= l.createLimit {
		oldest := valid[0]
		remaining := oldest.Add(l.createWindow).Sub(now)
		retryAfter := int(math.Ceil(remaining.Seconds()))
		if retryAfter < 1 {
			retryAfter = 1
		}
		l.roomCreates[ip] = valid
		return false, retryAfter
	}

	valid = append(valid, now)
	l.roomCreates[ip] = valid
	return true, 0
}

// Cleanup removes entries for IPs that haven't been active past the maximum window.
func (l *IPRateLimiter) Cleanup() {
	l.mu.Lock()
	defer l.mu.Unlock()

	now := l.clock.Now()
	reqThreshold := now.Add(-l.reqWindow)
	for ip, list := range l.requests {
		var valid []time.Time
		for _, t := range list {
			if t.After(reqThreshold) {
				valid = append(valid, t)
			}
		}
		if len(valid) == 0 {
			delete(l.requests, ip)
		} else {
			l.requests[ip] = valid
		}
	}

	createThreshold := now.Add(-l.createWindow)
	for ip, list := range l.roomCreates {
		var valid []time.Time
		for _, t := range list {
			if t.After(createThreshold) {
				valid = append(valid, t)
			}
		}
		if len(valid) == 0 {
			delete(l.roomCreates, ip)
		} else {
			l.roomCreates[ip] = valid
		}
	}
}
