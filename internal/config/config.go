// Package config loads settings from environment variables. Env vars
// only (no flags/files) - this is meant to run as a container, where
// env vars are the natural configuration surface.
package config

import (
	"fmt"
	"os"
	"time"
)

type Config struct {
	GitHubOrg      string
	GitHubToken    string
	ListenAddr     string
	RequestTimeout time.Duration
	CacheTTL       time.Duration
	CacheMaxStale  time.Duration
}

func FromEnv() (Config, error) {
	c := Config{
		GitHubOrg:      os.Getenv("GITHUB_ORG"),
		GitHubToken:    os.Getenv("GITHUB_TOKEN"),
		ListenAddr:     envString("LISTEN_ADDR", ":9222"),
		RequestTimeout: envDuration("GITHUB_REQUEST_TIMEOUT", 10*time.Second),
		// GitHub's REST API rate limit (5000/hr authenticated) has ample
		// headroom at this interval even for many more runners than this
		// org currently has - no need to tune it down further.
		CacheTTL:      envDuration("CACHE_TTL", 30*time.Second),
		CacheMaxStale: envDuration("CACHE_MAX_STALE", 5*time.Minute),
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
