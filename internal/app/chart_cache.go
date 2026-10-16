package app

import (
	"context"
	"sync"
	"time"
)

// Only historical chart points are cached. Authentication, current state,
// incidents and notification decisions never use this cache.
type pointCache struct {
	mu      sync.Mutex
	entries map[string]*pointEntry
}
type pointEntry struct {
	ready   chan struct{}
	expires time.Time
	points  []Point
	err     error
}

func (c *pointCache) get(ctx context.Context, key string, load func() ([]Point, error)) ([]Point, error) {
	c.mu.Lock()
	if c.entries == nil {
		c.entries = map[string]*pointEntry{}
	}
	if e := c.entries[key]; e != nil {
		select {
		case <-e.ready:
			if time.Now().Before(e.expires) {
				c.mu.Unlock()
				return e.points, e.err
			}
			delete(c.entries, key)
		default:
			c.mu.Unlock()
			select {
			case <-ctx.Done():
				return nil, ctx.Err()
			case <-e.ready:
				if e.err != nil && ctx.Err() == nil {
					return c.get(ctx, key, load)
				}
				return e.points, e.err
			}
		}
	}
	// Bound both completed entries and in-flight coalescing records.
	if len(c.entries) >= 32 {
		for k, e := range c.entries {
			select {
			case <-e.ready:
				delete(c.entries, k)
			default:
			}
		}
	}
	if len(c.entries) >= 32 {
		c.mu.Unlock()
		return load()
	}
	e := &pointEntry{ready: make(chan struct{})}
	c.entries[key] = e
	c.mu.Unlock()
	points, err := load()
	c.mu.Lock()
	e.points, e.err, e.expires = points, err, time.Now().Add(2*time.Second)
	if err != nil {
		delete(c.entries, key)
	}
	close(e.ready)
	c.mu.Unlock()
	return points, err
}
