# CLAUDE.md

## Project Overview

github-actions-runner-exporter is a small Go exporter serving `/metrics`
(Prometheus, via `client_golang`) built around two independently cached
domains, not one shared cache — unlike `pve-metrics-exporter` (this
repo's sibling/template), which coalesces two *output formats* around
one cache. Here it's two *data domains* with genuinely different
staleness tolerances: runner online/busy state is real-time by nature,
while repo/CI/PR/Dependabot stats cost ~3 GitHub API calls per repo per
refresh and would blow through the rate limit if polled that often
across a few dozen repos. Do not collapse these back into one cache TTL
— that either makes runner status sluggish or burns the rate limit,
depending on which TTL "wins".

## Architecture

- `internal/github` — REST client for every GitHub endpoint this exporter
  calls (`orgs/{org}/actions/runners`, `orgs/{org}/repos`,
  `repos/{owner}/{repo}/actions/workflows`, `.../actions/workflows/{id}/runs`,
  `.../pulls`, `.../dependabot/alerts`, `/rate_limit`). `OpenPRCount` reads
  the page count off the `Link` response header instead of paginating —
  deliberate: it's one request regardless of how many open PRs a repo has,
  and stays on the *core* rate limit rather than the separately-throttled
  Search API. `LatestRunForWorkflow` takes a workflow ID rather than
  checking the repo's most recent run overall — see orgstats below for why.
- `internal/fetch` — generic `Fetcher[T]`, the caching layer both domains
  share. Generic specifically because the cache/stale-fallback logic
  would otherwise be copy-pasted per domain.
- `internal/runners`, `internal/orgstats` — `Build` functions that turn
  the client's raw responses into each domain's `Summary`. `orgstats.Build`
  swallows per-repo call failures deliberately (one repo the token can't
  see, or with Dependabot disabled, shouldn't blank out every other
  repo's data) — but propagates a failure of `Repos`/`RateLimit`
  themselves, since nothing else can proceed without those.
- `internal/collector` — adapts both `Summary` types into Prometheus
  metrics for `/metrics`, on one shared `prometheus.Collector`.
- `internal/config` — env var parsing (see README's Configuration table).

CI status is tracked per active workflow (`orgstats.buildWorkflowCI`),
not per repo — checking only the single most recent run across a
repo's whole Actions history would let a failing workflow hide behind
a later, unrelated, successful one. Cost: one `Workflows` call plus one
`LatestRunForWorkflow` call per active workflow, per repo, per
`ORG_CACHE_TTL` refresh — still comfortably inside the rate limit at
this org's scale (a few dozen repos, a handful of workflows each), but
don't add finer granularity than that without re-checking the budget.

Deliberately out of scope: per-job duration/queue-time metrics and any
run history beyond the latest one per workflow (no org-wide "all runs"
endpoint, only per-repo/per-workflow — pulling more would mean fetching
full run history for every workflow in every repo).

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
- Requires a real GitHub org + PAT with the permissions in README's Token
  permissions section to exercise end-to-end; there's no mock/fixture
  server in this repo, so manual testing against `/metrics` needs
  `GITHUB_ORG`/`GITHUB_TOKEN` pointed at an actual org.
