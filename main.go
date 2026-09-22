// Command github-actions-runner-exporter exposes an organization's
// self-hosted GitHub Actions runner status (online/offline, busy/idle)
// as Prometheus metrics.
package main

import (
	"log/slog"
	"net/http"
	"os"

	"github.com/prometheus/client_golang/prometheus"
	"github.com/prometheus/client_golang/prometheus/promhttp"

	"github.com/drumandbytes/github-actions-runner-exporter/internal/collector"
	"github.com/drumandbytes/github-actions-runner-exporter/internal/config"
	"github.com/drumandbytes/github-actions-runner-exporter/internal/github"
	"github.com/drumandbytes/github-actions-runner-exporter/internal/summary"
)

func main() {
	log := slog.New(slog.NewJSONHandler(os.Stdout, nil))

	cfg, err := config.FromEnv()
	if err != nil {
		log.Error("invalid configuration", "error", err)
		os.Exit(1)
	}

	client := github.NewClient(cfg.GitHubOrg, cfg.GitHubToken, cfg.RequestTimeout)
	fetcher := summary.NewFetcher(client, cfg.CacheTTL, cfg.CacheMaxStale, log)

	registry := prometheus.NewRegistry()
	registry.MustRegister(collector.New(fetcher, cfg.CacheTTL))

	mux := http.NewServeMux()
	mux.Handle("/metrics", promhttp.HandlerFor(registry, promhttp.HandlerOpts{}))
	mux.HandleFunc("/healthz", func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusOK)
		_, _ = w.Write([]byte("ok"))
	})

	log.Info("listening", "addr", cfg.ListenAddr)
	if err := http.ListenAndServe(cfg.ListenAddr, mux); err != nil { //nolint:gosec // homelab-internal, timeouts not critical
		log.Error("server stopped", "error", err)
		os.Exit(1)
	}
}
