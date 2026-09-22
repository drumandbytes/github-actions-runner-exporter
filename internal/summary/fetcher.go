package summary

import (
	"context"
	"log/slog"
	"sync"
	"time"

	"github.com/drumandbytes/github-actions-runner-exporter/internal/github"
)

// Fetcher caches the GitHub API result so bursts of concurrent
// Prometheus scrapes don't each trigger their own round trip - and,
// more importantly here, so a scrape cadence tighter than GitHub's
// rate-limit budget doesn't burn through it.
//
// On a refresh failure, the previous successful Summary keeps being
// served (up to maxStale) rather than surfacing an error immediately -
// a brief GitHub API hiccup shouldn't flap the runner-status metrics.
type Fetcher struct {
	client   *github.Client
	ttl      time.Duration
	maxStale time.Duration
	log      *slog.Logger

	mu        sync.Mutex
	cached    Summary
	fetchedAt time.Time
	haveData  bool
}

func NewFetcher(client *github.Client, ttl, maxStale time.Duration, log *slog.Logger) *Fetcher {
	return &Fetcher{client: client, ttl: ttl, maxStale: maxStale, log: log}
}

func (f *Fetcher) Get(ctx context.Context) (Summary, error) {
	f.mu.Lock()
	defer f.mu.Unlock()

	if f.haveData && time.Since(f.fetchedAt) < f.ttl {
		return f.cached, nil
	}

	fresh, err := Build(ctx, f.client)
	if err != nil {
		if f.haveData && time.Since(f.fetchedAt) < f.maxStale {
			f.log.Warn("refresh failed, serving stale cached data",
				"error", err, "age", time.Since(f.fetchedAt))
			return f.cached, nil
		}
		return Summary{}, err
	}

	f.cached = fresh
	f.fetchedAt = time.Now()
	f.haveData = true
	return f.cached, nil
}
