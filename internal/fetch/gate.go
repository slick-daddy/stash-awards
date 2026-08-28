package fetch

import (
	"context"
	"sync"
	"time"
)

// gate serialises requests to one source and keeps them at least delay apart.
// Holding a plain mutex for the whole request is what limits a source to one
// request in flight, which matters more than throughput here: these are other
// people's servers.
type gate struct {
	mu    sync.Mutex
	delay time.Duration
	last  time.Time
}

func (g *gate) setDelay(d time.Duration) {
	g.mu.Lock()
	defer g.mu.Unlock()
	g.delay = d
}

// enter blocks until the caller may issue a request, and returns the function
// that releases the gate. The returned function must always be called.
func (g *gate) enter(ctx context.Context, sleep func(context.Context, time.Duration) error) (func(), error) {
	g.mu.Lock()

	if wait := g.waitFor(time.Now()); wait > 0 {
		if err := sleep(ctx, wait); err != nil {
			g.mu.Unlock()
			return nil, err
		}
	}
	if err := ctx.Err(); err != nil {
		g.mu.Unlock()
		return nil, err
	}

	return func() {
		// Space requests by their end time. Measuring from the end means a slow
		// response never causes a burst of immediate follow-ups.
		g.last = time.Now()
		g.mu.Unlock()
	}, nil
}

// waitFor reports how long to hold off, given the last request time. Caller holds
// the lock.
func (g *gate) waitFor(now time.Time) time.Duration {
	if g.delay <= 0 || g.last.IsZero() {
		return 0
	}
	elapsed := now.Sub(g.last)
	if elapsed >= g.delay {
		return 0
	}
	return g.delay - elapsed
}
