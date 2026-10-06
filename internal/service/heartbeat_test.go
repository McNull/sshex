package service

import (
	"testing"
	"time"
)

func TestHeartbeatMonitorDefaults(t *testing.T) {
	m := NewHeartbeatMonitor(0, 0, nil, nil)
	if m.Interval() != 10*time.Second {
		t.Fatalf("Interval() = %v, want 10s", m.Interval())
	}
	if m.Timeout() != 30*time.Second {
		t.Fatalf("Timeout() = %v, want 30s", m.Timeout())
	}
}

func TestHeartbeatMonitorIsStale(t *testing.T) {
	now := time.Unix(0, 0)
	m := NewHeartbeatMonitor(10*time.Second, 30*time.Second, func() time.Time { return now }, nil)

	if m.IsStale("c1") {
		t.Fatal("untracked connection reported stale")
	}
	m.Beat("c1")
	if m.IsStale("c1") {
		t.Fatal("fresh connection reported stale")
	}

	now = now.Add(31 * time.Second)
	if !m.IsStale("c1") {
		t.Fatal("connection past timeout not reported stale")
	}

	m.Forget("c1")
	if m.IsStale("c1") {
		t.Fatal("forgotten connection reported stale")
	}
}

func TestHeartbeatMonitorReap(t *testing.T) {
	now := time.Unix(0, 0)
	var expired []string
	m := NewHeartbeatMonitor(10*time.Second, 30*time.Second, func() time.Time { return now }, func(id string) {
		expired = append(expired, id)
	})

	m.Beat("c1")
	now = now.Add(5 * time.Second)
	m.Beat("c2")

	now = now.Add(20 * time.Second) // c1 age 25s (<30), c2 age 20s
	m.reap(now)
	if len(expired) != 0 {
		t.Fatalf("reap() expired %v too early", expired)
	}

	now = now.Add(11 * time.Second) // c1 age 36s, c2 age 31s
	m.reap(now)
	if len(expired) != 2 {
		t.Fatalf("reap() expired %v, want c1 and c2", expired)
	}

	// Expired entries are forgotten, so a second reap is a no-op.
	expired = nil
	m.reap(now)
	if len(expired) != 0 {
		t.Fatalf("second reap() expired %v, want none", expired)
	}
}
