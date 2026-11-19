package ratelimit_test

import (
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
	"github.com/termofeed/termofeed-back/internal/clock"
	"github.com/termofeed/termofeed-back/internal/ratelimit"
)

func TestIPRateLimiter_GeneralRequests(t *testing.T) {
	start := time.Date(2026, 11, 18, 20, 0, 0, 0, time.UTC)
	fc := clock.NewFakeClock(start)
	limiter := ratelimit.NewIPRateLimiter(fc)

	ip := "192.168.1.10"

	// 120 requests should be allowed
	for i := 0; i < 120; i++ {
		allowed, retryAfter := limiter.AllowRequest(ip)
		assert.True(t, allowed, "request %d should be allowed", i)
		assert.Equal(t, 0, retryAfter)
	}

	// 121st request should be rejected
	allowed, retryAfter := limiter.AllowRequest(ip)
	assert.False(t, allowed)
	assert.True(t, retryAfter > 0)
	assert.Equal(t, 60, retryAfter)

	// Different IP should still be allowed
	allowed2, _ := limiter.AllowRequest("192.168.1.11")
	assert.True(t, allowed2)

	// Advance 61 seconds
	fc.Advance(61 * time.Second)

	// Now requests should be allowed again
	allowed, retryAfter = limiter.AllowRequest(ip)
	assert.True(t, allowed)
	assert.Equal(t, 0, retryAfter)
}

func TestIPRateLimiter_RoomCreation(t *testing.T) {
	start := time.Date(2026, 11, 18, 20, 0, 0, 0, time.UTC)
	fc := clock.NewFakeClock(start)
	limiter := ratelimit.NewIPRateLimiter(fc)

	ip := "10.0.0.1"

	// 10 creations should be allowed in 1 hour
	for i := 0; i < 10; i++ {
		allowed, retryAfter := limiter.AllowCreateRoom(ip)
		assert.True(t, allowed, "creation %d should be allowed", i)
		assert.Equal(t, 0, retryAfter)
	}

	// 11th creation should be rejected
	allowed, retryAfter := limiter.AllowCreateRoom(ip)
	assert.False(t, allowed)
	assert.True(t, retryAfter > 0)

	// Advance 1 hour and 1 second
	fc.Advance(3601 * time.Second)

	allowed, retryAfter = limiter.AllowCreateRoom(ip)
	assert.True(t, allowed)
	assert.Equal(t, 0, retryAfter)
}
