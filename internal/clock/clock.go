package clock

import (
	"sync"
	"time"
)

// Timer abstracts time.Timer for testing.
type Timer interface {
	C() <-chan time.Time
	Stop() bool
	Reset(d time.Duration) bool
}

// Clock abstracts time operations so business logic can be tested deterministically.
type Clock interface {
	Now() time.Time
	Since(t time.Time) time.Duration
	Until(t time.Time) time.Duration
	After(d time.Duration) <-chan time.Time
	NewTimer(d time.Duration) Timer
}

// RealClock implements Clock using the standard time package.
type RealClock struct{}

func NewRealClock() *RealClock {
	return &RealClock{}
}

func (c *RealClock) Now() time.Time {
	return time.Now().UTC()
}

func (c *RealClock) Since(t time.Time) time.Duration {
	return c.Now().Sub(t)
}

func (c *RealClock) Until(t time.Time) time.Duration {
	return t.Sub(c.Now())
}

func (c *RealClock) After(d time.Duration) <-chan time.Time {
	return time.After(d)
}

type realTimer struct {
	t *time.Timer
}

func (r *realTimer) C() <-chan time.Time {
	return r.t.C
}

func (r *realTimer) Stop() bool {
	return r.t.Stop()
}

func (r *realTimer) Reset(d time.Duration) bool {
	return r.t.Reset(d)
}

func (c *RealClock) NewTimer(d time.Duration) Timer {
	return &realTimer{t: time.NewTimer(d)}
}

// FakeClock is a deterministic clock for tests.
type FakeClock struct {
	mu     sync.Mutex
	now    time.Time
	timers []*fakeTimer
}

func NewFakeClock(initial time.Time) *FakeClock {
	return &FakeClock{
		now: initial.UTC(),
	}
}

func (f *FakeClock) Now() time.Time {
	f.mu.Lock()
	defer f.mu.Unlock()
	return f.now
}

func (f *FakeClock) Since(t time.Time) time.Duration {
	f.mu.Lock()
	defer f.mu.Unlock()
	return f.now.Sub(t)
}

func (f *FakeClock) Until(t time.Time) time.Duration {
	f.mu.Lock()
	defer f.mu.Unlock()
	return t.Sub(f.now)
}

func (f *FakeClock) After(d time.Duration) <-chan time.Time {
	timer := f.NewTimer(d)
	return timer.C()
}

func (f *FakeClock) NewTimer(d time.Duration) Timer {
	f.mu.Lock()
	defer f.mu.Unlock()

	ft := &fakeTimer{
		clock:    f,
		ch:       make(chan time.Time, 1),
		deadline: f.now.Add(d),
		active:   true,
	}

	if d <= 0 {
		ft.active = false
		ft.ch <- f.now
	} else {
		f.timers = append(f.timers, ft)
	}

	return ft
}

// Advance moves the fake clock forward by d and fires expired timers.
func (f *FakeClock) Advance(d time.Duration) {
	f.mu.Lock()
	defer f.mu.Unlock()

	f.now = f.now.Add(d)
	f.triggerTimersLocked()
}

// Set sets the fake clock to a specific time and fires expired timers.
func (f *FakeClock) Set(t time.Time) {
	f.mu.Lock()
	defer f.mu.Unlock()

	f.now = t.UTC()
	f.triggerTimersLocked()
}

func (f *FakeClock) triggerTimersLocked() {
	var remaining []*fakeTimer
	for _, t := range f.timers {
		if t.active && !f.now.Before(t.deadline) {
			t.active = false
			select {
			case t.ch <- f.now:
			default:
			}
		} else if t.active {
			remaining = append(remaining, t)
		}
	}
	f.timers = remaining
}

type fakeTimer struct {
	clock    *FakeClock
	ch       chan time.Time
	deadline time.Time
	active   bool
}

func (t *fakeTimer) C() <-chan time.Time {
	return t.ch
}

func (t *fakeTimer) Stop() bool {
	t.clock.mu.Lock()
	defer t.clock.mu.Unlock()

	wasActive := t.active
	t.active = false
	return wasActive
}

func (t *fakeTimer) Reset(d time.Duration) bool {
	t.clock.mu.Lock()
	defer t.clock.mu.Unlock()

	wasActive := t.active
	t.deadline = t.clock.now.Add(d)
	t.active = true

	if d <= 0 {
		t.active = false
		select {
		case t.ch <- t.clock.now:
		default:
		}
	} else {
		found := false
		for _, existing := range t.clock.timers {
			if existing == t {
				found = true
				break
			}
		}
		if !found {
			t.clock.timers = append(t.clock.timers, t)
		}
	}

	return wasActive
}
