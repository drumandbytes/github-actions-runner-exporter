# Contributing

## Commits and releases

Releases are automated with [release-please](https://github.com/googleapis/release-please).
It reads the commit history on `main`, keeps a rolling **release PR** with the next
version + `CHANGELOG.md`, and on merge tags `vX.Y.Z` + creates the GitHub Release.
`build.yml` then publishes the matching multi-arch image:
`ghcr.io/drumandbytes/github-actions-runner-exporter:X.Y.Z` and `:X.Y` (immutable
— there is no floating `:1`; `:latest` tracks `main` separately).

**Squash-merge every PR** with a
[Conventional Commits](https://www.conventionalcommits.org/) title:

| Prefix | Effect | Example |
| --- | --- | --- |
| `feat:` | minor bump | `feat: add per-label runner filtering` |
| `fix:` / `perf:` | patch bump | `fix: handle a runner with no labels` |
| `feat!:` or `BREAKING CHANGE:` in body | major bump | `feat!: rename the metric prefix` |
| `chore:` `docs:` `ci:` `test:` `refactor:` | no release | `docs: update the README` |

Dependabot prefixes Go and base-image bumps `fix(deps):` (→ patch release + new
image) and action bumps `ci(deps):` (CI only, no release).

Don't hand-edit `CHANGELOG.md` or tags — release-please owns them.
