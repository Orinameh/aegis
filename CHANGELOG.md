# Changelog

All notable changes to this project will be documented in this file.

The format is based on [Keep a Changelog](https://keepachangelog.com/en/1.1.0/),
and this project adheres to [Semantic Versioning](https://semver.org/spec/v2.0.0.html).

## [Unreleased]

### Fixed
- ntfy notifications now POST a plain-text message to the configured topic URL. The previous JSON payload only worked when posted to the ntfy root URL with a `topic` field, so the documented `ntfy.sh/<topic>` webhook setup failed.
- Nested environment overrides such as `AEGIS_DOCKER_PRUNE_VOLUMES` and `AEGIS_KUBERNETES_JOB_RETENTION_DAYS` are now applied; previously only top-level keys could be overridden.
- Interactive prompts reuse a single stdin reader, so answers piped or typed ahead for consecutive prompts are no longer dropped.
- Protection denials are now detected with a typed error instead of matching error text, so unrelated operational errors are no longer silently treated as denials.
- The review queue file is replaced atomically (write to a temp file, then rename), so a crash mid-write can no longer truncate it.
- `aegis check` logs a warning instead of ignoring an unreadable review queue.
- Runtime errors from `aegis check`, `aegis review`, and `aegis list` no longer dump the full command usage; every subcommand now reports failures as a single `Error:` line.
- Stray positional arguments (e.g. `aegis check stray`, `aegis review clear`) are rejected instead of being silently ignored.
- `aegis list --types/--kinds` is honored even when that side is disabled in config; with both sides disabled and no filters, a hint is printed instead of silently listing nothing.

### Added
- Config validation rejects a negative `kubernetes.job_retention_days`.
- Unit tests for `cmd/aegis` helpers: clean scoping, list filters, byte formatting, and custom-rule conversion.

### Changed
- Updated dependencies: `k8s.io/*` v0.37.1, `github.com/moby/moby/client` v0.6.2, `golang.org/x/sys` v0.49.0, and transitive updates (OpenTelemetry v1.47, `golang.org/x/net` v0.61).

## [0.1.1] - 2026-08-14

### Added
- Scoped cleanup for `aegis clean` via `--docker-only`, `--k8s-only`, and per-resource flags (`--images-only`, `--containers-only`, `--volumes-only`, `--networks-only`, `--build-cache-only`, `--pods-only`, `--jobs-only`, `--pvcs-only`). At most one scope flag per run; without one, everything enabled in config is cleaned.
- README cookbook examples for `list --types/--kinds`, `check`, `clean`, and `review`.

### Changed
- Disabled the auto-generated Cobra completion command.
- Clarified docs on what Kubernetes resources are (and are not) cleaned.

## [0.1.0] - 2026-08-14

Initial release — a protected cleanup CLI for Docker and Kubernetes.

### Added
- Command set:
  - `aegis clean` — the destructive cleanup path, protected by guards.
  - `aegis check` — read-only disk usage check that notifies via webhook when over threshold.
  - `aegis list` — read-only inventory of Docker and Kubernetes resources.
  - `aegis review` — list/clear actions denied by protection that await review.
- Protection guard system for critical Docker resources and Kubernetes resources.
- Custom protection rules in `config.yaml` with `protection_level`, `override_allowed`, and `requires_approval`.
- Interactive approval prompts for strict/critical resources (`--interactive=false` enables unattended mode).
- Review queue for denials encountered in non-interactive mode.
- Webhook notifications (Slack, Discord, ntfy, generic) for `aegis check`.
- Disk usage threshold gating for cleanup, configurable via `max_disk_usage_percent` or the `--threshold` flag.
- Non-root, distroless Docker image with socket-group-aware `make docker-run` targets.
- GitHub Actions CI (test, lint, release) plus community docs (CONTRIBUTING, CODE_OF_CONDUCT, SECURITY, issue/PR templates).
- Test coverage for protection guards, config loading, disk checking, table rendering, and Docker/Kubernetes logic.

### Changed
- Kubernetes sweeper corrected:
  - Evicted pods are now recognized via `phase=Failed` + `reason=Evicted` (previously mishandled by the PodFailed path).
  - `delete_completed_jobs` now also removes **failed** jobs (previously duplicated `delete_succeeded_jobs`).
- Docker pruner corrected:
  - Networks are inspected individually before deletion to avoid removing networks with attached containers.
  - Volumes with missing usage data are skipped instead of panicking.
- Config load treats a missing `--config` file as fatal only when explicitly provided; otherwise falls back to defaults.
- Logging and linting hardened (errcheck, unused variable removal, consistent formatting).
- Full **Windows** support: disk usage reporting now has a Windows implementation, and release
  binaries include `windows-amd64` (a platform-specific `statfs` was replaced with
  `GetDiskFreeSpaceEx` under a build tag).

### Security
- Docker image runs as non-root (`65532`) using a distroless base with no shell.
- Webhook URLs are no longer logged.
- Every deletion is confirmed interactively unless explicitly auto-approved.