```
   ╔═══════════════════════════════════════════╗
   ║                                         ║
   ║    █████╗ ███████╗ ██████╗ ██╗███████╗  ║
   ║   ██╔══██╗██╔════╝██╔════╝ ██║██╔════╝  ║
   ║   ███████║█████╗  ██║  ███╗██║███████╗  ║
   ║   ██╔══██║██╔══╝  ██║   ██║██║╚════██║  ║
   ║   ██║  ██║███████╗╚██████╔╝██║███████║  ║
   ║   ╚═╝  ╚═╝╚══════╝ ╚═════╝ ╚═╝╚══════╝  ║
   ║                                         ║
   ║   Protected Infrastructure Cleaning     ║
   ╚═══════════════════════════════════════════╝
```

# 🛡️ Aegis

**Protected Infrastructure Cleaning Utility**

Aegis is a unified cloud-native cleanup utility that safely prunes Docker and Kubernetes resources while protecting critical components from accidental deletion.

<p align="center">
  <a href="https://github.com/Orinameh/aegis/actions/workflows/ci.yaml"><img src="https://img.shields.io/github/actions/workflow/status/Orinameh/aegis/ci.yaml?branch=main&label=CI&logo=github" alt="CI"></a>
  <a href="https://github.com/Orinameh/aegis/releases"><img src="https://img.shields.io/github/v/release/Orinameh/aegis" alt="Release"></a>
  <img src="https://img.shields.io/github/go-mod/go-version/Orinameh/aegis" alt="Go version">
  <a href="https://go.dev/report"><img src="https://img.shields.io/badge/go%20report-A+-brightgreen" alt="Go Report"></a>
  <a href="LICENSE"><img src="https://img.shields.io/github/license/Orinameh/aegis" alt="License: MIT"></a>
</p>

## ✨ Features

- **Docker Pruning**: Safely removes stopped containers, dangling images, and build cache
- **Kubernetes Sweeping**: Cleans failed/evicted pods, completed jobs, and orphaned PVCs
- **Protection Guards**: Prevents accidental deletion of sensitive resources with 4 protection levels
- **Interactive Mode**: Manual approval for protected resources
- **Audit Logging**: Complete audit trail of all operations
- **Dry Run**: Preview changes without making modifications
- **Configurable**: YAML-based configuration with environment variable overrides
- **Structured Logging**: JSON logs with configurable levels

## 🗺️ How It Works

```text
┌────────────┐     ┌───────────────────────────┐
│   Aegis    │────▶│   Docker Daemon (local)   │
│  (CLI)     │     │  containers · images      │
│            │     │  volumes · networks       │
└────────────┘     └───────────────────────────┘
   │      │
   │      └────────▶┌───────────────────────────┐
   │                │   Kubernetes Cluster      │
   │                │  pods · jobs · pvcs       │
   │                │  (local minikube/kind OR  │
   │                │   hosted AKS/EKS/GKE)     │
   │                └───────────────────────────┘
   ▼
┌───────────────────────────┐
│   Protection Guard        │
│  ⚠️ warning   🚫 strict   │
│  🔴 critical  ✅ approved │
└───────────────────────────┘
   │
   ├─▶ dry-run ──▶ report only
   ├─▶ interactive ──▶ Y/N confirmation per resource
   └─▶ auto-approve ──▶ delete (use with caution)
```

## 📁 Project Structure

```text
aegis/
├── cmd/aegis/               # CLI entrypoint (cobra)
│   └── main.go              #   ─ check / clean / list / review subcommands
├── internal/                # private Go packages (not importable outside)
│   ├── banner/              #   ASCII-art banner printed at startup
│   ├── config/              #   config loading, defaults, validation
│   ├── docker/              #   Docker pruner (delete) + list.go (inventory)
│   ├── k8s/                 #   Kubernetes sweeper (delete) + list.go (inventory)
│   ├── guard/               #   protection guard, audit log, review queue
│   ├── notify/              #   webhook notifications (Slack/Discord/ntfy)
│   ├── system/              #   disk-usage checks
│   └── table/               #   dependency-free ASCII table renderer
├── pkg/logger/              # shared (importable) zap logger setup
├── .github/
│   ├── workflows/ci.yaml    #   GitHub Actions: build, test, lint, release
│   ├── ISSUE_TEMPLATE/      #   bug & feature request templates
│   └── PULL_REQUEST_TEMPLATE.md
├── config.yaml              # example configuration (secrets go in config.local.yaml)
├── Makefile                 # build, test, lint, install, release targets
├── Dockerfile               # container image for in-cluster runs
├── .golangci.yaml           # golangci-lint config (v2 format)
├── SECURITY.md              # vulnerability reporting & threat model
├── CONTRIBUTING.md          # contributor guide
└── CODE_OF_CONDUCT.md       # community standards
```

## 🚀 Quick Start

### Prerequisites

- **Go 1.26+** — to build from source
- **Docker** or **Kubernetes** access (optional) — needed only for the corresponding cleanup modules

Aegis works on **macOS (Intel & Apple Silicon)** and **Linux**.

#### Local and hosted Kubernetes (AKS, EKS, GKE, etc.)

The Kubernetes module works in both environments automatically:

- **Local**: uses your `~/.kube/config` (whichever cluster your current `kubectl`
  context points to — including managed services like AKS, EKS, or GKE).
- **In-cluster**: when run as a pod inside a cluster, it auto-detects the in-cluster
  config, so it works on any hosted/managed Kubernetes service.

### Installation

```bash
# Option 1: Install with Go (puts the binary in $(go env GOPATH)/bin)
go install github.com/orinameh/aegis/cmd/aegis@latest

# Option 2: Build from source and install to your PATH (works on macOS & Linux)
git clone https://github.com/orinameh/aegis.git
cd aegis
make install
```

`make install` builds the binary and copies it to the first writable directory that's on
your `PATH`, preferring `$(go env GOPATH)/bin`, then `~/.local/bin`,
`/opt/homebrew/bin` (Apple Silicon), then `/usr/local/bin`. If none qualify, it prints
the list of directories you need to add to your `PATH` (or install to manually):

```bash
# Manual install if you prefer
make build
cp bin/aegis /usr/local/bin/
```

### Running with Docker

Build the image (there is no published registry image yet, so build it first):

```bash
docker build -t aegis:latest .
# or
make docker-build   # tags it aegis:<version>
```

The image runs as a **non-root** user. It's distroless (no shell, no package
manager) with the runtime user UID/GID `65532` (distroless's standard `nonroot`
user), so the container has no privileges of its own.

```bash
docker run --rm -v "$PWD/config.yaml:/config/config.yaml" aegis:latest check
```

Mount your config at `/config/config.yaml`. Aegis also writes its audit log and
review queue relative to its working directory `/config` — mount a writable volume
there to persist them:

```bash
docker run --rm \
  -v "$PWD/config.yaml:/config/config.yaml" \
  -v "$PWD/logs:/config/logs" \
  aegis:latest clean --interactive=false
```

Two permission notes:

- **Kubernetes (in-cluster)**: works out of the box — Aegis reads the mounted
  service-account token and uses in-cluster config. Nothing extra needed.
- **Docker socket**: the container needs the host's Docker socket group to talk to
  the daemon, since it runs as non-root. Pass the socket's group ID (this is how
  `docker` CLI containers do it):

  ```bash
  docker run --rm \
    --user 65532:65532 \
    --group-add "$(stat -c %g /var/run/docker.sock)" \
    -v /var/run/docker.sock:/var/run/docker.sock \
    -v "$PWD/config.yaml:/config/config.yaml" \
    aegis:latest clean
  ```

  On macOS (Docker Desktop) the socket is owned by the Docker VM, so grant access
  with `--privileged` or run the socket mount with the `docker` group instead.

For a read-only review of what would be cleaned, use `aegis list` or
`aegis clean --dry-run` inside the container.

### Usage

Aegis has three subcommands designed around a **check → clean → review** workflow:

```bash
aegis                 # alias for `aegis clean` (backward compatible)
aegis clean           # run destructive cleanup with protection guards
aegis check           # read-only disk check; notifies via webhook when over threshold
aegis list            # read-only inventory of all Docker & Kubernetes resources
aegis review          # list actions denied by protection that await review
aegis review --clear  # empty the review queue
```

#### `aegis list` — read-only resource inventory

Shows all Docker containers, images, volumes, and networks, plus all Kubernetes pods,
jobs, and PVCs, rendered as aligned tables. Performs **no mutations**, so it's always safe
to run:

```bash
aegis list                       # everything (Docker + Kubernetes)
aegis list --types containers    # only Docker containers (no Kubernetes)
aegis list --kinds pods          # only Kubernetes pods (no Docker)
aegis list --types images,volumes --kinds jobs,pvcs
```

Passing `--types` shows only the Docker side; passing `--kinds` shows only the
Kubernetes side. Omit both for everything, or combine them to filter each side.

The Kubernetes tables come from whatever cluster Aegis is pointed at — locally it
uses your current `kubectl` context (works with AKS, EKS, GKE, minikube, kind, ...),
and inside a cluster it auto-detects the in-cluster config.

#### `aegis check` — safe to run unattended

Read-only. Checks disk usage and, if it exceeds the threshold, POSTs an alert to the
webhook configured under `notification:` in `config.yaml`. Zero destructive risk, so it's
safe to run every 5–15 minutes via a systemd timer or Kubernetes CronJob.

Exit codes: `0` = below threshold (or notifications disabled), `2` = threshold exceeded
(notification sent), `1` = a real error.

##### Setting the threshold

The disk usage threshold (percent) controls both `aegis check` and `aegis clean`. Default
is `85` (`max_disk_usage_percent` in `config.yaml`, valid range 1–100):

```yaml
# config.yaml
max_disk_usage_percent: 90   # only alert / clean once disk is 90% full
```

Override it per-run without editing the config via the `--threshold` flag (same range):

```bash
aegis check --threshold 90    # alert only when disk hits 90%
aegis clean --threshold 90    # only start deleting once disk hits 90%
```

`clean` **skips all deletions while disk usage is below the threshold** — the threshold is
the gate that decides *when* cleanup is worth running. Setting it higher (e.g. `95`)
postpones cleanup until the disk is more full; setting it lower (e.g. `70`) makes Aegis
reclaim space more eagerly. Combine with `--dry-run` to preview:

```bash
aegis clean --threshold 80 --dry-run   # preview what would be deleted at 80%
```

```yaml
# config.yaml
notification:
  enabled: true
  webhook_url: "https://hooks.slack.com/services/..."   # Slack, Discord, or ntfy.sh URL
  provider: "generic"   # generic, slack, discord, or ntfy
  timeout: 10s
```

#### `aegis clean` — human or unattended

- **Manually**: run interactively; strict-protected resources prompt for approval.
- **Unattended** (`--interactive=false`): strict resources without `override_allowed`
  are **denied and queued** rather than hanging on stdin. Warning-level rules auto-approve;
  critical always denies. Queued denials are written to the `protection.review_queue_path`
  file and surfaced in check notifications.

```bash
aegis clean                       # interactive, prompts for protected resources
aegis clean --interactive=false   # unattended: strict denials go to the review queue
aegis clean --dry-run             # preview only
aegis clean --auto-approve        # bypass prompts (use with caution)
aegis clean --threshold 90        # only delete once disk is 90% full
```

#### `aegis review` — the pending-review queue

When a strict rule fires in non-interactive mode, instead of blocking, Aegis records it
in a JSON review queue and keeps working. Later, a human reviews and either resolves it
or cleans with an override:

```bash
aegis review          # shows denied items awaiting human judgment
aegis review --clear  # clear the queue after review
```

Other flags (persistent on all commands):

```bash
aegis clean --config path/to/config.yaml
aegis check --log-level debug
aegis clean --threshold 90
aegis --no-banner
```

### Make targets

Development and release helpers (run `make help` for a full list):

```bash
make build            # build bin/aegis
make install          # build + install onto your PATH
make test             # tests with race detector + coverage
make lint             # golangci-lint run ./...
make fmt              # go fmt ./...
make mod              # tidy + vendor modules

make run              # go run with config.yaml (CONFIG=... to override)
make run-dry          # dry-run mode
make run-debug        # debug logging
make run-noninteractive  # clean without prompts (denials go to review queue)
make run-auto         # auto-approve everything (y/N confirmation built in)
make run-override     # run with an override token (prompts for it)

make docker-build     # docker build -t aegis:<version>
make docker-run       # run the non-root image with socket + kubeconfig mounts
make docker-run-dry   # same, but --dry-run

make release          # cross-compile release binaries into bin/release/
make version          # show current version
make check            # is bin/aegis built?
make clean            # remove build artifacts, logs, coverage
```

The Docker make targets handle the non-root image for you: they pass the
host Docker socket group, mount `~/.kube/config` into the container's home
(`/config/.kube/config`), and persist logs to `./logs`.