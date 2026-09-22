# CLAUDE.md

## Project Overview

github-actions-runner-exporter is a small Go exporter serving `/metrics`
(Prometheus, via `client_golang`) built from one shared cache
(`internal/summary.Fetcher`) around a single GitHub API call: listing an
org's self-hosted runners. The cache exists to keep the poll interval
comfortably inside GitHub's REST rate limit, not to coalesce concurrent
readers the way `pve-metrics-exporter` (this repo's sibling/template)
needs to for two independent output formats — there's only one output
here. On a refresh failure, the previous result keeps being served up to
`CACHE_MAX_STALE` before an error surfaces, so a transient GitHub API
hiccup doesn't flap the runner-status metrics.

## Architecture

- `internal/github` — minimal REST client for `GET /orgs/{org}/actions/runners`.
- `internal/summary` — `Build` normalizes the API response into a `Summary`;
  `Fetcher` is the shared cache described above.
- `internal/collector` — adapts `Summary` into Prometheus metrics for `/metrics`.
- `internal/config` — env var parsing (see README's Configuration table).

Deliberately out of scope: per-job/workflow duration and queue-time metrics.
GitHub's REST API has no org-wide "all workflow runs" endpoint, only
per-repo (`GET /repos/{owner}/{repo}/actions/runs`) — adding that would
mean enumerating every repo in the org. Runner up/busy already answers
"is it healthy and is it working" for a small fixed set of runners; revisit
only if job-level detail is actually needed.

## Build / test / run

```bash
go build ./...
go vet ./...
go test ./...
docker build -t github-actions-runner-exporter .
```

CI (`.github/workflows/validate.yml`) gates on the `drumandbytes/reusable-actions`
`go-ci.yml` lint job, a Docker smoke test (`/healthz` against fake credentials
— never a real GitHub org), and Trivy image scan. `build.yml` pushes
multi-arch images to GHCR with SLSA provenance attestation on push to
`main`/tags.

## Conventions

- Commits: Conventional Commits (`feat:`, `fix:`, `chore:`, …) — `release-please`
  reads them for versioning/changelog. No `Co-Authored-By` trailers.
- Requires a real GitHub org + PAT with runner-list permission to exercise
  end-to-end (see README's Configuration table); there's no mock/fixture
  server in this repo, so manual testing against `/metrics` needs
  `GITHUB_ORG`/`GITHUB_TOKEN` pointed at an actual org.
