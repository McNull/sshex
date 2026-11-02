package versioncheck

import (
	"context"
	"errors"
	"os"
	"path/filepath"
	"testing"
	"time"
)

func TestStoreRoundTrip(t *testing.T) {
	path := filepath.Join(t.TempDir(), StateFileName)
	want := State{CheckedAt: time.Date(2026, 11, 2, 12, 0, 0, 0, time.UTC), AvailableVersion: "1.2.3"}
	store := NewStore(path)
	if err := store.Save(want); err != nil {
		t.Fatalf("Save() error: %v", err)
	}
	got, err := store.Load()
	if err != nil {
		t.Fatalf("Load() error: %v", err)
	}
	if !got.CheckedAt.Equal(want.CheckedAt) || got.AvailableVersion != want.AvailableVersion || got.Error != want.Error {
		t.Fatalf("Load() = %+v, want %+v", got, want)
	}
	info, err := os.Stat(path)
	if err != nil {
		t.Fatal(err)
	}
	if info.Mode().Perm() != fileMode {
		t.Errorf("state file mode = %o, want %o", info.Mode().Perm(), fileMode)
	}
}

func TestStoreLoadMissing(t *testing.T) {
	state, err := NewStore(filepath.Join(t.TempDir(), StateFileName)).Load()
	if err != nil {
		t.Fatalf("Load() error: %v", err)
	}
	if !state.CheckedAt.IsZero() || state.AvailableVersion != "" {
		t.Fatalf("Load() = %+v, want empty state", state)
	}
}

func TestStartIfDue(t *testing.T) {
	now := time.Date(2026, 11, 2, 12, 0, 0, 0, time.UTC)
	path := filepath.Join(t.TempDir(), StateFileName)
	store := NewStore(path)
	launched := 0
	launch := func(gotPath string) error {
		launched++
		if gotPath != path {
			t.Errorf("launch path = %q, want %q", gotPath, path)
		}
		return nil
	}
	StartIfDue(path, launch, now)
	if launched != 1 {
		t.Fatalf("initial launch count = %d, want 1", launched)
	}
	if err := store.Save(State{CheckedAt: now.Add(-CheckInterval + time.Second)}); err != nil {
		t.Fatal(err)
	}
	StartIfDue(path, launch, now)
	if launched != 1 {
		t.Fatalf("launch count before 24h = %d, want 1", launched)
	}
	if err := store.Save(State{CheckedAt: now.Add(-CheckInterval)}); err != nil {
		t.Fatal(err)
	}
	StartIfDue(path, launch, now)
	if launched != 2 {
		t.Fatalf("launch count at 24h = %d, want 2", launched)
	}
}

type latestSourceFunc func(context.Context) (string, error)

func (f latestSourceFunc) LatestVersion(ctx context.Context) (string, error) { return f(ctx) }

func TestRunWorkerSavesAvailableVersion(t *testing.T) {
	path := filepath.Join(t.TempDir(), StateFileName)
	now := time.Date(2026, 11, 2, 12, 0, 0, 0, time.UTC)
	calls := 0
	source := latestSourceFunc(func(context.Context) (string, error) {
		calls++
		return "1.4.0", nil
	})
	clock := func() time.Time { return now }
	if err := RunWorker(context.Background(), path, source, clock); err != nil {
		t.Fatalf("RunWorker() error: %v", err)
	}
	state, err := NewStore(path).Load()
	if err != nil {
		t.Fatal(err)
	}
	if !state.CheckedAt.Equal(now) || state.AvailableVersion != "1.4.0" {
		t.Fatalf("worker state = %+v", state)
	}
	if calls != 1 {
		t.Fatalf("source called %d times, want 1", calls)
	}
	if err := RunWorker(context.Background(), path, source, clock); err != nil {
		t.Fatalf("second RunWorker() error: %v", err)
	}
	if calls != 1 {
		t.Fatalf("source called %d times after not-due worker, want 1", calls)
	}
}

func TestRunWorkerRecordsFailedAttemptAndKeepsKnownVersion(t *testing.T) {
	path := filepath.Join(t.TempDir(), StateFileName)
	now := time.Date(2026, 11, 2, 12, 0, 0, 0, time.UTC)
	previous := State{CheckedAt: now.Add(-CheckInterval), AvailableVersion: "1.4.0"}
	if err := NewStore(path).Save(previous); err != nil {
		t.Fatal(err)
	}
	wantErr := errors.New("offline")
	err := RunWorker(context.Background(), path, latestSourceFunc(func(context.Context) (string, error) {
		return "", wantErr
	}), func() time.Time { return now })
	if !errors.Is(err, wantErr) {
		t.Fatalf("RunWorker() error = %v, want %v", err, wantErr)
	}
	state, err := NewStore(path).Load()
	if err != nil {
		t.Fatal(err)
	}
	if !state.CheckedAt.Equal(now) || state.AvailableVersion != previous.AvailableVersion || state.Error != wantErr.Error() {
		t.Fatalf("state after failed check = %+v", state)
	}
}

func TestRunWorkerClearsPreviousErrorAfterSuccess(t *testing.T) {
	path := filepath.Join(t.TempDir(), StateFileName)
	now := time.Date(2026, 11, 2, 12, 0, 0, 0, time.UTC)
	if err := NewStore(path).Save(State{CheckedAt: now.Add(-CheckInterval), Error: "previous failure"}); err != nil {
		t.Fatal(err)
	}
	if err := RunWorker(context.Background(), path, latestSourceFunc(func(context.Context) (string, error) {
		return "1.5.0", nil
	}), func() time.Time { return now }); err != nil {
		t.Fatalf("RunWorker() error: %v", err)
	}
	state, err := NewStore(path).Load()
	if err != nil {
		t.Fatal(err)
	}
	if state.Error != "" || state.AvailableVersion != "1.5.0" {
		t.Fatalf("state after successful check = %+v", state)
	}
}

func TestIsNewer(t *testing.T) {
	tests := []struct {
		candidate string
		current   string
		want      bool
	}{
		{"1.10.0", "1.9.9", true},
		{"v1.2.0", "1.2.0", false},
		{"1.2.0", "1.2.0-rc.1", true},
		{"1.2.0-rc.2", "1.2.0-rc.1", true},
		{"1.2.0-rc.1", "1.2.0", false},
		{"not-a-version", "1.0.0", false},
	}
	for _, tt := range tests {
		t.Run(tt.candidate+"_vs_"+tt.current, func(t *testing.T) {
			if got := IsNewer(tt.candidate, tt.current); got != tt.want {
				t.Errorf("IsNewer(%q, %q) = %v, want %v", tt.candidate, tt.current, got, tt.want)
			}
		})
	}
}
