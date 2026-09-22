# github-actions-runner-exporter

A small Go exporter that polls the GitHub REST API for an organization's
self-hosted Actions runners and exposes their online/busy state as
Prometheus metrics.

## Why

GitHub doesn't publish these as metrics itself, and there's no single
well-maintained community exporter for it. This one is intentionally
narrow: runner up/busy state only. GitHub's REST API has no org-wide
"all workflow runs" endpoint (only per-repo), so per-job duration/queue-time
metrics aren't included — they'd mean enumerating every repo in the org
just to watch a couple of runners.

## Metrics

| Metric | Labels | Meaning |
| --- | --- | --- |
| `github_up` | | 1 if the last GitHub API poll succeeded, 0 if a stale cache is being served |
| `github_runner_up` | `runner`, `os` | 1 if the runner is registered and online, 0 if offline |
| `github_runner_busy` | `runner`, `os` | 1 if the runner is currently executing a job, 0 if idle |

## Configuration

| Env var | Default | Required |
| --- | --- | --- |
| `GITHUB_ORG` | | yes |
| `GITHUB_TOKEN` | | yes — a PAT with permission to list the org's self-hosted runners (GitHub's REST API docs require `admin:org` for a classic PAT; use a fine-grained PAT scoped read-only to organization administration if narrower scoping is available) |
| `LISTEN_ADDR` | `:9222` | |
| `GITHUB_REQUEST_TIMEOUT` | `10s` | |
| `CACHE_TTL` | `30s` | |
| `CACHE_MAX_STALE` | `5m` | |

## Build / test / run

```bash
go build ./...
go vet ./...
go test ./...
docker build -t github-actions-runner-exporter .
```

CI (`.github/workflows/validate.yml`) gates on the `drumandbytes/reusable-actions`
`go-ci.yml` lint job, a Docker smoke test (`/healthz` against fake credentials —
never a real GitHub org), and Trivy image scan. `build.yml` pushes multi-arch
images to GHCR with SLSA provenance attestation on push to `main`/tags.
