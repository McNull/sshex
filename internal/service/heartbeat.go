package service

import (
	"context"
	"sync"
	"time"
)

// HeartbeatMonitor tracks the last heartbeat of each connection and reports the
// ones that have gone silent. Last-seen values are kept in memory only: the
// monotonic reading in time.Time survives a host suspend, so a sleeping laptop
// does not falsely expire its own connections.
type HeartbeatMonitor struct {
	mu       sync.Mutex
	seen     map[string]time.Time
	interval time.Duration
	timeout  time.Duration
	now      func() time.Time
	onExpire func(connectionID string)
}

func NewHeartbeatMonitor(interval, timeout time.Duration, now func() time.Time, onExpire func(string)) *HeartbeatMonitor {
	if interval <= 0 {
		interval = 10 * time.Second
	}
	if timeout <= 0 {
		timeout = 3 * interval
	}
	if now == nil {
		now = time.Now
	}
	return &HeartbeatMonitor{
		seen:     make(map[string]time.Time),
		interval: interval,
		timeout:  timeout,
		now:      now,
		onExpire: onExpire,
	}
}

func (m *HeartbeatMonitor) Interval() time.Duration { return m.interval }

func (m *HeartbeatMonitor) Timeout() time.Duration { return m.timeout }

// Beat records a heartbeat for a connection.
func (m *HeartbeatMonitor) Beat(connectionID string) {
	m.mu.Lock()
	defer m.mu.Unlock()
	m.seen[connectionID] = m.now()
}

// Forget stops tracking a connection.
func (m *HeartbeatMonitor) Forget(connectionID string) {
	m.mu.Lock()
	defer m.mu.Unlock()
	delete(m.seen, connectionID)
}

// IsStale reports whether a tracked connection has missed its timeout.
func (m *HeartbeatMonitor) IsStale(connectionID string) bool {
	m.mu.Lock()
	defer m.mu.Unlock()
	last, ok := m.seen[connectionID]
	if !ok {
		return false
	}
	return m.now().Sub(last) > m.timeout
}

// Run reaps expired connections until the context is cancelled.
func (m *HeartbeatMonitor) Run(ctx context.Context) {
	ticker := time.NewTicker(m.interval)
	defer ticker.Stop()
	for {
		select {
		case <-ctx.Done():
			return
		case now := <-ticker.C:
			m.reap(now)
		}
	}
}

// reap removes every connection that has missed its timeout and hands it to the
// expiry callback. Entries are removed before the callback runs so a callback
// that blocks cannot cause a double expiry.
func (m *HeartbeatMonitor) reap(now time.Time) {
	m.mu.Lock()
	expired := make([]string, 0)
	for id, last := range m.seen {
		if now.Sub(last) > m.timeout {
			delete(m.seen, id)
			expired = append(expired, id)
		}
	}
	m.mu.Unlock()

	for _, id := range expired {
		if m.onExpire != nil {
			m.onExpire(id)
		}
	}
}
