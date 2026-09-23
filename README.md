# github-actions-runner-exporter

A small Go exporter that polls the GitHub API for an organization's
self-hosted Actions runners, plus repo/CI health, PR counts and
Dependabot alerts, and exposes it all as Prometheus metrics.

## Why

GitHub doesn't publish any of this as metrics itself, and there's no
single well-maintained community exporter that covers it. Runner
up/busy state is real-time by nature; everything else (CI status, PR
counts, Dependabot alerts, rate limit) is polled on a much slower cache
TTL, deliberately - see [Two cache tiers](#two-cache-tiers) below.

CI status is tracked **per active workflow**, not per repo: checking
only the single most recent run across a repo's whole Actions history
would let a failing workflow hide behind a later, unrelated, successful
one (e.g. a broken `Validate` masked by a subsequent green `Build`).

Deliberately **not** included: per-job duration/queue-time metrics or
run history beyond the latest one (GitHub's REST API has no org-wide
"all workflow runs" endpoint, only per-repo/per-workflow, so anything
beyond "latest run" would mean pulling full run history for every
workflow in every repo).

## Two cache tiers

| | Runner status | Org/repo stats |
| --- | --- | --- |
| TTL | `RUNNER_CACHE_TTL` (default 30s) | `ORG_CACHE_TTL` (default 5m) |
| Why | Genuinely real-time - a runner picking up a job matters within seconds | CI/PR/Dependabot signals don't change that fast, and cost 3 + N calls per repo per refresh (N = active workflow count) - polling that on a 30s TTL across a few dozen repos would burn through GitHub's 5,000/hour rate limit for no benefit |

## Metrics

| Metric | Labels | Meaning |
| --- | --- | --- |
| `github_runners_up` | | 1 if the last runner-status poll succeeded, 0 if a stale cache is being served |
| `github_runner_up` | `runner`, `os` | 1 if the runner is registered and online, 0 if offline |
| `github_runner_busy` | `runner`, `os` | 1 if the runner is currently executing a job, 0 if idle |
| `github_org_up` | | 1 if the last org/repo stats poll succeeded, 0 if a stale cache is being served |
| `github_org_repos_total` | `visibility` | Number of non-archived repos, by `public`/`private` |
| `github_rate_limit_remaining` | | Remaining core API rate-limit budget |
| `github_rate_limit_limit` | | Total core API rate-limit budget |
| `github_repo_open_prs` | `repo` | Number of open pull requests |
| `github_repo_ci_last_run_conclusion` | `repo`, `workflow`, `url`, `conclusion` | Always 1 - an "info" metric. `conclusion` is GitHub's own string verbatim (`success`, `failure`, `cancelled`, `skipped`, `neutral`, `timed_out`, `action_required`, `stale`), not collapsed to pass/fail here - what counts as "actually broken" is a dashboard-level call. `url` links to the run on github.com. Absent if the workflow has never run |
| `github_repo_ci_last_run_timestamp_seconds` | `repo`, `workflow` | Unix timestamp of that workflow's latest completed run |
| `github_repo_ci_last_run_duration_seconds` | `repo`, `workflow` | Duration of that workflow's latest completed run |
| `github_repo_dependabot_alerts_open` | `repo`, `severity` | Open Dependabot alerts by severity. Absent for a severity with zero open alerts |

## Configuration

| Env var | Default | Required |
| --- | --- | --- |
| `GITHUB_ORG` | | yes |
| `GITHUB_TOKEN` | | yes — see [Token permissions](#token-permissions) |
| `LISTEN_ADDR` | `:9222` | |
| `GITHUB_REQUEST_TIMEOUT` | `10s` | |
| `RUNNER_CACHE_TTL` | `30s` | |
| `RUNNER_CACHE_MAX_STALE` | `5m` | |
| `ORG_CACHE_TTL` | `5m` | |
| `ORG_CACHE_MAX_STALE` | `30m` | |

## Token permissions

A fine-grained PAT needs:
- **Repository access: All repositories** — scoping it to a curated list means every repo-level metric (PRs, CI, Dependabot) silently only covers the repos you picked.
- Repository permissions: **Metadata** (read, auto-included), **Pull requests** (read), **Actions** (read), **Dependabot alerts** (read).
- Organization permissions: whatever GitHub's console offers for self-hosted runners (its REST API docs only document `admin:org` for the classic-PAT path to that endpoint — narrower fine-grained scoping isn't guaranteed available for every org plan).

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
