package mixr

import (
	"sync"
	"time"
)

type diagnosticLimiter struct {
	mu       sync.Mutex
	interval time.Duration
	active   map[string]bool
	last     map[string]time.Time
}

func newDiagnosticLimiter(interval time.Duration) *diagnosticLimiter {
	return &diagnosticLimiter{
		interval: interval,
		active:   make(map[string]bool),
		last:     make(map[string]time.Time),
	}
}

func (l *diagnosticLimiter) allow(key string, now time.Time) bool {
	l.mu.Lock()
	defer l.mu.Unlock()
	l.active[key] = true
	if last := l.last[key]; !last.IsZero() && now.Sub(last) < l.interval {
		return false
	}
	l.last[key] = now
	return true
}

func (l *diagnosticLimiter) recovered(key string) bool {
	l.mu.Lock()
	defer l.mu.Unlock()
	wasActive := l.active[key]
	delete(l.active, key)
	delete(l.last, key)
	return wasActive
}
