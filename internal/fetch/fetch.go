// Package fetch caches build results so scrape bursts and a tight scrape
// interval don't burn GitHub's rate limit.
package fetch

import (
	"context"
	"fmt"
	"log/slog"
	"sync"
	"time"
)

// Fetcher caches a build function's T. Once anything is cached, Get never
// blocks: a stale value is returned and refreshed in the background. Blocking
// in the scrape handler used to blow Prometheus's scrape timeout. Only the very
// first call blocks.
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
