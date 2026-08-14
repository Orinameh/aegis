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
  <a href="https://github.com/Orinameh/aegis/actions"><img src="https://github.com/Orinameh/aegis/actions/workflows/ci.yml/badge.svg" alt="CI"></a>
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
aegis list --types containers    # only Docker containers
aegis list --kinds pods          # only Kubernetes pods
aegis list --types images,volumes --kinds jobs,pvcs
```

The Kubernetes tables come from whatever cluster Aegis is pointed at — locally it
uses your current `kubectl` context (works with AKS, EKS, GKE, minikube, kind, ...),
and inside a cluster it auto-detects the in-cluster config.

#### `aegis check` — safe to run unattended

Read-only. Checks disk usage and, if it exceeds the threshold, POSTs an alert to the
webhook configured under `notification:` in `config.yaml`. Zero destructive risk, so it's
safe to run every 5–15 minutes via a systemd timer or Kubernetes CronJob.

Exit codes: `0` = below threshold (or notifications disabled), `2` = threshold exceeded
(notification sent), `1` = a real error.

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
aegis --no-banner
```