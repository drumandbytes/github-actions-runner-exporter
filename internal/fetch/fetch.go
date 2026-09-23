// Package fetch caches the result of a build function so bursts of
// concurrent Prometheus scrapes don't each trigger their own round trip,
// and - more importantly for this exporter - so a scrape cadence tighter
// than GitHub's rate-limit budget doesn't burn through it.
package fetch

import (
	"context"
	"log/slog"
	"sync"
	"time"
)

// Fetcher caches whatever T a build function produces. On a refresh
// failure, the previous successful value keeps being served (up to
// maxStale) rather than surfacing an error immediately - a brief GitHub
// API hiccup shouldn't flap the metrics.
type Fetcher[T any] struct {
	build    func(context.Context) (T, error)
	ttl      time.Duration
	maxStale time.Duration
	log      *slog.Logger

	mu        sync.Mutex
	cached    T
	fetchedAt time.Time
	haveData  bool
}

func New[T any](build func(context.Context) (T, error), ttl, maxStale time.Duration, log *slog.Logger) *Fetcher[T] {
	return &Fetcher[T]{build: build, ttl: ttl, maxStale: maxStale, log: log}
}

func (f *Fetcher[T]) Get(ctx context.Context) (T, error) {
	f.mu.Lock()
	defer f.mu.Unlock()

	if f.haveData && time.Since(f.fetchedAt) < f.ttl {
		return f.cached, nil
	}

	fresh, err := f.build(ctx)
	if err != nil {
		if f.haveData && time.Since(f.fetchedAt) < f.maxStale {
			f.log.Warn("refresh failed, serving stale cached data",
				"error", err, "age", time.Since(f.fetchedAt))
			return f.cached, nil
		}
		var zero T
		return zero, err
	}

	f.cached = fresh
	f.fetchedAt = time.Now()
	f.haveData = true
	return f.cached, nil
}
