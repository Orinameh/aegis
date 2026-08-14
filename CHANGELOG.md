# Changelog

All notable changes to this project will be documented in this file.

The format is based on [Keep a Changelog](https://keepachangelog.com/en/1.1.0/),
and this project adheres to [Semantic Versioning](https://semver.org/spec/v2.0.0.html).

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

### Security
- Docker image runs as non-root (`65532`) using a distroless base with no shell.
- Webhook URLs are no longer logged.
- Every deletion is confirmed interactively unless explicitly auto-approved.