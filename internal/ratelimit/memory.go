package ratelimit

import (
	"context"
	"sync"
	"time"
)

type memoryEntry struct {
	count     int
	expiresAt time.Time
}

// MemoryLimiter implements Limiter with in-process fixed-window counters.
// Safe for concurrent use. Expired keys are cleaned periodically to avoid leak.
type MemoryLimiter struct {
	mu      sync.Mutex
	entries map[string]*memoryEntry
	stopCh  chan struct{}
}

// NewMemoryLimiter creates a new in-memory rate limiter.
// It starts a background goroutine to clean expired keys every 5 minutes.
func NewMemoryLimiter() *MemoryLimiter {
	m := &MemoryLimiter{
		entries: make(map[string]*memoryEntry),
		stopCh:  make(chan struct{}),
	}
	go m.cleanupLoop()

	return m
}

// Stop stops the background cleanup goroutine. Call when shutting down.
func (m *MemoryLimiter) Stop() {
	close(m.stopCh)
}

// Allow implements Limiter.
func (m *MemoryLimiter) Allow(ctx context.Context, key string, limit int, window time.Duration) (allowed bool, retryAfter int, err error) {
	now := time.Now()
	expiresAt := now.Add(window)

	m.mu.Lock()
	defer m.mu.Unlock()

	e, ok := m.entries[key]
	if !ok || now.After(e.expiresAt) {
		m.entries[key] = &memoryEntry{count: 1, expiresAt: expiresAt}

		return true, 0, nil
	}

	e.count++
	if e.count > limit {
		sec := int(time.Until(e.expiresAt).Seconds())
		if sec < 0 {
			sec = 0
		}

		return false, sec, nil
	}

	return true, 0, nil
}

const cleanupInterval = 5 * time.Minute

func (m *MemoryLimiter) cleanupLoop() {
	ticker := time.NewTicker(cleanupInterval)
	defer ticker.Stop()
	for {
		select {
		case <-m.stopCh:
			return
		case <-ticker.C:
			m.cleanup()
		}
	}
}

func (m *MemoryLimiter) cleanup() {
	now := time.Now()
	m.mu.Lock()
	defer m.mu.Unlock()
	for k, e := range m.entries {
		if now.After(e.expiresAt) {
			delete(m.entries, k)
		}
	}
}
