package fetch

import (
	"context"
	"errors"
	"log/slog"
	"sync/atomic"
	"testing"
	"time"
)

func TestGet(t *testing.T) {
	var fail atomic.Bool
	n := 0
	f := &Fetcher[int]{maxStale: time.Hour, log: slog.New(slog.DiscardHandler), build: func(context.Context) (int, error) {
		if fail.Load() {
			return 0, errors.New("github down")
		}
		n++
		return n, nil
	}}

	if _, err := f.Get(); err == nil {
		t.Fatal("Get before the first poll returned no error")
	}
	fail.Store(true)
	f.refresh()
	if _, err := f.Get(); err == nil || err.Error() != "github down" {
		t.Fatalf("failed first poll: err = %v", err)
	}

	fail.Store(false)
	f.refresh()
	fail.Store(true)
	f.refresh()
	if v, err := f.Get(); err != nil || v != 1 {
		t.Fatalf("failed refresh should serve stale value 1: got %d, %v", v, err)
	}

	f.fetchedAt = time.Now().Add(-2 * time.Hour)
	if _, err := f.Get(); err == nil {
		t.Fatal("data past maxStale served")
	}
}

func TestRefreshesWithoutScrapes(t *testing.T) {
	var builds atomic.Int32
	f := New(func(context.Context) (int32, error) { return builds.Add(1), nil }, 10*time.Millisecond, time.Hour, slog.New(slog.DiscardHandler))

	deadline := time.Now().Add(2 * time.Second)
	for builds.Load() < 3 {
		if time.Now().After(deadline) {
			t.Fatalf("only %d builds in 2s with a 10ms interval", builds.Load())
		}
		time.Sleep(5 * time.Millisecond)
	}
	if v, err := f.Get(); err != nil || v < 2 {
		t.Fatalf("Get = %d, %v; want a refreshed value", v, err)
	}
}
