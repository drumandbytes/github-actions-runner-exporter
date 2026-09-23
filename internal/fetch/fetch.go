// Package fetch caches the result of a build function so bursts of
// concurrent Prometheus scrapes don't each trigger their own round trip,
// and - more importantly for this exporter - so a scrape cadence tighter
// than GitHub's rate-limit budget doesn't burn through it.
package fetch

import (
	"context"
	"fmt"
	"log/slog"
	"sync"
	"time"
)

// Fetcher caches whatever T a build function produces. Get never blocks
// a caller on an upstream round trip once it has anything cached at
// all - a stale cache triggers a refresh in the background and the
// stale value is returned immediately instead. This matters
// specifically because Get is called from a Prometheus scrape handler:
// blocking on the upstream API (as an earlier version of this did)
// coupled scrape response time to that API's latency, and once the TTL
// was close to the scrape interval, most scrapes ended up doing a live
// fetch inline - slow enough, often enough, to blow past Prometheus's
// own scrape timeout and show up as real gaps in the data.
//
// The one exception is the very first call ever, before anything has
// been fetched at all - there's nothing to serve yet, so that one has
// to block.
type Fetcher[T any] struct {
	build    func(context.Context) (T, error)
	ttl      time.Duration
	maxStale time.Duration
	log      *slog.Logger

	mu         sync.Mutex
	cached     T
	fetchedAt  time.Time
	haveData   bool
	refreshing bool // a background refresh is already in flight
}

func New[T any](build func(context.Context) (T, error), ttl, maxStale time.Duration, log *slog.Logger) *Fetcher[T] {
	return &Fetcher[T]{build: build, ttl: ttl, maxStale: maxStale, log: log}
}

func (f *Fetcher[T]) Get(ctx context.Context) (T, error) {
	f.mu.Lock()
	haveData := f.haveData
	cached := f.cached
	stale := !haveData || time.Since(f.fetchedAt) >= f.ttl
	tooStale := haveData && time.Since(f.fetchedAt) >= f.maxStale
	if stale && !f.refreshing {
		f.refreshing = true
		go f.backgroundRefresh()
	}
	f.mu.Unlock()

	if !haveData {
		return f.blockingRefresh(ctx)
	}
	if tooStale {
		var zero T
		return zero, fmt.Errorf("cached data is older than the %s max-stale window", f.maxStale)
	}
	return cached, nil
}

func (f *Fetcher[T]) backgroundRefresh() {
	defer func() {
		f.mu.Lock()
		f.refreshing = false
		f.mu.Unlock()
	}()

	fresh, err := f.build(context.Background())
	if err != nil {
		f.log.Warn("background refresh failed, serving stale cached data", "error", err)
		return
	}

	f.mu.Lock()
	f.cached = fresh
	f.fetchedAt = time.Now()
	f.haveData = true
	f.mu.Unlock()
}

func (f *Fetcher[T]) blockingRefresh(ctx context.Context) (T, error) {
	fresh, err := f.build(ctx)
	if err != nil {
		var zero T
		return zero, err
	}

	f.mu.Lock()
	f.cached = fresh
	f.fetchedAt = time.Now()
	f.haveData = true
	f.mu.Unlock()

	return fresh, nil
}
