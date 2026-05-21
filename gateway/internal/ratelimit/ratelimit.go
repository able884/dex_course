package ratelimit

import (
	"sync"
	"time"
)

// Limiter defines a simple in-memory sliding window limiter.
type Limiter struct {
	mu      sync.Mutex
	buckets map[string]*bucket
}

type bucket struct {
	count   int
	expires time.Time
}

// New returns a Limiter instance.
func New() *Limiter {
	return &Limiter{
		buckets: make(map[string]*bucket),
	}
}

// Allow reports whether the given key is permitted under the provided limit and window.
// When limit <= 0, the limiter always allows the request.
func (l *Limiter) Allow(key string, limit int, window time.Duration) bool {
	if limit <= 0 {
		return true
	}
	now := time.Now()

	l.mu.Lock()
	defer l.mu.Unlock()

	bkt := l.buckets[key]
	if bkt == nil || now.After(bkt.expires) {
		l.buckets[key] = &bucket{
			count:   1,
			expires: now.Add(window),
		}
		return true
	}

	if bkt.count >= limit {
		return false
	}

	bkt.count++
	return true
}
