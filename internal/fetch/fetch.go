// Package fetch caches build results so scrape bursts and a tight scrape
// interval don't burn GitHub's rate limit.
package fetch

import (
	"context"
	"errors"
	"fmt"
	"log/slog"
	"sync"
	"time"
)

// Fetcher rebuilds T every ttl in the background, independent of scrapes, so
// a scrape always reads data at most one ttl old and never blocks on GitHub
// (blocking used to blow Prometheus's scrape timeout, and the first org build
// takes minutes). Until the first build finishes, Get returns an error.
type Fetcher[T any] struct {
	build    func(context.Context) (T, error)
	maxStale time.Duration
	log      *slog.Logger

	mu        sync.Mutex
	cached    T
	fetchedAt time.Time
	haveData  bool
	lastErr   error
}

// New starts the refresh loop; it runs for the life of the process.
func New[T any](build func(context.Context) (T, error), ttl, maxStale time.Duration, log *slog.Logger) *Fetcher[T] {
	f := &Fetcher[T]{build: build, maxStale: maxStale, log: log}
	go f.loop(ttl)
	return f
}

func (f *Fetcher[T]) loop(ttl time.Duration) {
	f.refresh()
	for range time.Tick(ttl) {
		f.refresh()
	}
}

func (f *Fetcher[T]) refresh() {
	fresh, err := f.build(context.Background())

	f.mu.Lock()
	defer f.mu.Unlock()
	if err != nil {
		f.lastErr = err
		if f.haveData {
			f.log.Warn("refresh failed, serving stale cached data", "error", err)
		}
		return
	}
	f.cached, f.fetchedAt, f.haveData, f.lastErr = fresh, time.Now(), true, nil
}

func (f *Fetcher[T]) Get() (T, error) {
	var zero T
	f.mu.Lock()
	defer f.mu.Unlock()
	switch {
	case !f.haveData && f.lastErr != nil:
		return zero, f.lastErr
	case !f.haveData:
		return zero, errors.New("first poll still running")
	case time.Since(f.fetchedAt) >= f.maxStale:
		return zero, fmt.Errorf("cached data is older than the %s max-stale window", f.maxStale)
	}
	return f.cached, nil
}
