package clock_test

import (
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
	"github.com/termofeed/termofeed-back/internal/clock"
)

func TestFakeClock_AdvanceAndTimers(t *testing.T) {
	start := time.Date(2026, 11, 18, 20, 0, 0, 0, time.UTC)
	fc := clock.NewFakeClock(start)

	assert.Equal(t, start, fc.Now())
	assert.Equal(t, time.Duration(0), fc.Since(start))

	timer := fc.NewTimer(10 * time.Second)

	select {
	case <-timer.C():
		t.Fatal("timer should not have fired yet")
	default:
	}

	fc.Advance(5 * time.Second)
	select {
	case <-timer.C():
		t.Fatal("timer should not have fired after 5s")
	default:
	}

	fc.Advance(5 * time.Second)
	select {
	case now := <-timer.C():
		assert.Equal(t, start.Add(10*time.Second), now)
	default:
		t.Fatal("timer should have fired after 10s")
	}
}

func TestFakeClock_ResetAndStop(t *testing.T) {
	start := time.Date(2026, 11, 18, 20, 0, 0, 0, time.UTC)
	fc := clock.NewFakeClock(start)

	timer := fc.NewTimer(10 * time.Second)
	assert.True(t, timer.Stop())
	assert.False(t, timer.Stop())

	fc.Advance(20 * time.Second)
	select {
	case <-timer.C():
		t.Fatal("stopped timer should not fire")
	default:
	}

	timer.Reset(5 * time.Second)
	fc.Advance(5 * time.Second)
	select {
	case <-timer.C():
	default:
		t.Fatal("reset timer should have fired")
	}
}

func TestRealClock(t *testing.T) {
	rc := clock.NewRealClock()
	now := rc.Now()
	assert.False(t, now.IsZero())
	assert.True(t, rc.Since(now) >= 0)
}
