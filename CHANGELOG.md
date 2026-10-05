# Changelog

## [1.3.3](https://github.com/drumandbytes/github-actions-runner-exporter/compare/v1.3.2...v1.3.3) (2026-10-05)


### Bug Fixes

* label job histograms with job_name and the workflow's own name ([#30](https://github.com/drumandbytes/github-actions-runner-exporter/issues/30)) ([fd8330f](https://github.com/drumandbytes/github-actions-runner-exporter/commit/fd8330f171e921c7865f305f168cc5c858713068))

## [1.3.2](https://github.com/drumandbytes/github-actions-runner-exporter/compare/v1.3.1...v1.3.2) (2026-10-04)


### Bug Fixes

* report the real API rate-limit budget from response headers ([#26](https://github.com/drumandbytes/github-actions-runner-exporter/issues/26)) ([3ac76f2](https://github.com/drumandbytes/github-actions-runner-exporter/commit/3ac76f268e9b81890e4ff5e90d549d5b3bfb2206))

## [1.3.1](https://github.com/drumandbytes/github-actions-runner-exporter/compare/v1.3.0...v1.3.1) (2026-10-04)


### Performance Improvements

* cross-compile arm64 instead of building under QEMU ([#23](https://github.com/drumandbytes/github-actions-runner-exporter/issues/23)) ([1ba2391](https://github.com/drumandbytes/github-actions-runner-exporter/commit/1ba2391931c199113802b8e204b73daea49124a1))

## [1.3.0](https://github.com/drumandbytes/github-actions-runner-exporter/compare/v1.2.0...v1.3.0) (2026-10-04)


### Features

* poll GitHub on a timer instead of on scrape ([#21](https://github.com/drumandbytes/github-actions-runner-exporter/issues/21)) ([ae644a6](https://github.com/drumandbytes/github-actions-runner-exporter/commit/ae644a670ebc72d6ad20e3b99209156036209b05))

## [1.2.0](https://github.com/drumandbytes/github-actions-runner-exporter/compare/v1.1.0...v1.2.0) (2026-10-04)


### Features

* record job queue/run time history from an incremental run feed ([#19](https://github.com/drumandbytes/github-actions-runner-exporter/issues/19)) ([2efef67](https://github.com/drumandbytes/github-actions-runner-exporter/commit/2efef672488e508ea745cdd1e36424c19f63fe30))

## [1.1.0](https://github.com/drumandbytes/github-actions-runner-exporter/compare/v1.0.1...v1.1.0) (2026-09-28)


### Features

* ship Prometheus alerting rules with unit tests ([#16](https://github.com/drumandbytes/github-actions-runner-exporter/issues/16)) ([4f97fef](https://github.com/drumandbytes/github-actions-runner-exporter/commit/4f97fef0176bbaa063a73f866de076d5a7c7d749))

## [1.0.1](https://github.com/drumandbytes/github-actions-runner-exporter/compare/v1.0.0...v1.0.1) (2026-09-23)


### Bug Fixes

* never block a Prometheus scrape on the upstream GitHub API ([dea5eca](https://github.com/drumandbytes/github-actions-runner-exporter/commit/dea5eca0feda14aec82af057245d2e75985093a2))

## [1.0.0](https://github.com/drumandbytes/github-actions-runner-exporter/compare/v0.4.1...v1.0.0) (2026-09-23)


### ⚠ BREAKING CHANGES

* github_repo_ci_success is gone; consumers need github_repo_ci_last_run_conclusion and their own conclusion mapping.

### Features

* expose the raw GitHub conclusion instead of a collapsed pass/fail bool ([9531198](https://github.com/drumandbytes/github-actions-runner-exporter/commit/953119862f93dcb936b746d1c75d16b802f01e26))

## [0.4.1](https://github.com/drumandbytes/github-actions-runner-exporter/compare/v0.4.0...v0.4.1) (2026-09-23)


### Bug Fixes

* don't count cancelled/skipped runs as CI failures ([09be5ab](https://github.com/drumandbytes/github-actions-runner-exporter/commit/09be5ab894ffce7b54f6415028504e1a5eb35404))

## [0.4.0](https://github.com/drumandbytes/github-actions-runner-exporter/compare/v0.3.0...v0.4.0) (2026-09-23)


### Features

* add run URL to github_repo_ci_success ([9b9f0f4](https://github.com/drumandbytes/github-actions-runner-exporter/commit/9b9f0f453a4078e03f2675ee0514bf80564c715c))

## [0.3.0](https://github.com/drumandbytes/github-actions-runner-exporter/compare/v0.2.0...v0.3.0) (2026-09-23)


### Features

* add repo/CI health, PR counts, Dependabot alerts and org stats ([2af6c6c](https://github.com/drumandbytes/github-actions-runner-exporter/commit/2af6c6c6671f7db516c45f2cd11c4116b0dca175))

## [0.2.0](https://github.com/drumandbytes/github-actions-runner-exporter/compare/v0.1.0...v0.2.0) (2026-09-22)


### Features

* initial release ([fd5dd14](https://github.com/drumandbytes/github-actions-runner-exporter/commit/fd5dd14c19c60e68db2be0f5aeca680521574b1c))
