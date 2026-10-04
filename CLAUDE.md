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
  Search API. `RunsCreatedSince` and `RunJobs` follow `Link: rel="next"`
  pagination; `LatestRunForWorkflow` is only used for the startup bootstrap.
- `internal/fetch` — generic `Fetcher[T]`, the caching layer both domains
  share. Each Fetcher polls on its own timer (the domain's TTL), not on
  scrape, so `Get` never blocks: it serves the latest result, an error
  until the first poll finishes, or an error past max-stale. Generic
  specifically because this logic would otherwise be copy-pasted per domain.
- `internal/runners`, `internal/orgstats` — `Build` functions that turn
  the client's raw responses into each domain's `Summary`. `orgstats.Build`
  swallows per-repo call failures deliberately (one repo the token can't
  see, or with Dependabot disabled, shouldn't blank out every other
  repo's data) — but propagates a failure of `Repos`/`RateLimit`
  themselves, since nothing else can proceed without those.
- `internal/collector` — adapts both `Summary` types into Prometheus
  metrics for `/metrics`, on one shared `prometheus.Collector`.
- `internal/config` — env var parsing (see README's Configuration table).

CI history comes from an incremental run feed (`orgstats.Feed`), not
per-workflow polling: per repo, per `ORG_CACHE_TTL` refresh, one
`RunsCreatedSince(watermark)` call plus one `RunJobs` call per newly
completed run; every finished job is observed once into the
`github_job_*` histograms, and Prometheus is the history store. The feed
also keeps the per-active-workflow `github_repo_ci_last_run_*` snapshot
(a failing workflow mustn't hide behind a later green one in the same
repo), seeded at startup with one `LatestRunForWorkflow` per workflow.
The watermark is pinned by the oldest in-progress run (capped at 24h) —
don't add `status=completed` to the runs query, or a slow run created
before a faster one gets skipped forever. Dedupe is by (run ID, attempt)
and job ID, kept 48h. A restart starts from "now"; nothing is replayed.
Measured on drumandbytes (26 repos, 156 active workflows): ~106 calls per
refresh vs ~236 with the old per-workflow polling.

## Build / test / run

```bash
go build ./...
go vet ./...
go test ./...
docker build -t github-actions-runner-exporter .
```

CI (`.github/workflows/validate.yml`) gates on the `drumandbytes/reusable-actions`
`go-ci.yml` lint job and a Docker smoke test (`/healthz` against fake credentials
— never a real GitHub org). The Trivy image scan is its own workflow
(`security.yml`: PRs plus a weekly run on main), kept out of `validate.yml` so
a CVE can't block every Dependabot merge. `build.yml` pushes
multi-arch images to GHCR with SLSA provenance attestation on push to
`main`/tags.

## Conventions

- Commits: Conventional Commits (`feat:`, `fix:`, `chore:`, …) — `release-please`
  reads them for versioning/changelog. No `Co-Authored-By` trailers.
- Requires a real GitHub org + PAT with the permissions in README's Token
  permissions section to exercise end-to-end; there's no mock/fixture
  server in this repo, so manual testing against `/metrics` needs
  `GITHUB_ORG`/`GITHUB_TOKEN` pointed at an actual org.
