// Package config reads env vars only; it runs as a container.
package config

import (
	"fmt"
	"os"
	"strings"
	"time"
)

type Config struct {
	GitHubOrg      string
	GitHubToken    string
	ListenAddr     string
	RequestTimeout time.Duration

	// runner status is real-time: short poll interval
	RunnerCacheTTL      time.Duration
	RunnerCacheMaxStale time.Duration

	// ~4 API calls per repo per refresh: long interval to stay inside the rate limit
	OrgCacheTTL      time.Duration
	OrgCacheMaxStale time.Duration

	// repos (names within GITHUB_ORG) to track adoption for; empty disables it
	AdoptionRepos         []string
	AdoptionCacheTTL      time.Duration
	AdoptionCacheMaxStale time.Duration
}

func FromEnv() (Config, error) {
	c := Config{
		GitHubOrg:      os.Getenv("GITHUB_ORG"),
		GitHubToken:    os.Getenv("GITHUB_TOKEN"),
		ListenAddr:     envString("LISTEN_ADDR", ":9222"),
		RequestTimeout: envDuration("GITHUB_REQUEST_TIMEOUT", 10*time.Second),

		RunnerCacheTTL:      envDuration("RUNNER_CACHE_TTL", 15*time.Second),
		RunnerCacheMaxStale: envDuration("RUNNER_CACHE_MAX_STALE", 5*time.Minute),

		OrgCacheTTL:      envDuration("ORG_CACHE_TTL", 5*time.Minute),
		OrgCacheMaxStale: envDuration("ORG_CACHE_MAX_STALE", 30*time.Minute),

		AdoptionRepos:         splitList(os.Getenv("ADOPTION_REPOS")),
		AdoptionCacheTTL:      envDuration("ADOPTION_CACHE_TTL", time.Hour),
		AdoptionCacheMaxStale: envDuration("ADOPTION_CACHE_MAX_STALE", 3*time.Hour),
	}

	if c.GitHubOrg == "" {
		return Config{}, fmt.Errorf("GITHUB_ORG is required")
	}
	if c.GitHubToken == "" {
		return Config{}, fmt.Errorf("GITHUB_TOKEN is required")
	}
	return c, nil
}

func envString(key, def string) string {
	if v := os.Getenv(key); v != "" {
		return v
	}
	return def
}

func envDuration(key string, def time.Duration) time.Duration {
	v := os.Getenv(key)
	if v == "" {
		return def
	}
	d, err := time.ParseDuration(v)
	if err != nil {
		return def
	}
	return d
}

func splitList(v string) []string {
	var out []string
	for _, p := range strings.Split(v, ",") {
		if p = strings.TrimSpace(p); p != "" {
			out = append(out, p)
		}
	}
	return out
}
