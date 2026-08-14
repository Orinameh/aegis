# Contributing to Aegis

Thanks for taking the time to contribute! This document lays out the ground
rules so we can keep the project maintainable and — above all — **safe**, since
this tool deletes infrastructure resources.

## Table of contents

- [Development setup](#development-setup)
- [Making changes](#making-changes)
- [Coding standards](#coding-standards)
- [Testing](#testing)
- [Commits & pull requests](#commits--pull-requests)
- [Reporting bugs](#reporting-bugs)
- [Security](#security)

## Development setup

```bash
git clone https://github.com/Orinameh/aegis.git
cd aegis

# Build
make build

# Run the test suite
make test

# Lint
make lint
```

Prerequisites: **Go 1.26+**. Docker and/or a Kubernetes cluster are only
needed to test the corresponding modules live — unit tests run headless.

## Making changes

1. **Fork the repo** and create a branch: `git checkout -b fix/describe-fix`.
2. Make your change with focused commits (see [Commits](#commits--pull-requests)).
3. Add or update tests for any behavior change.
4. Run `make fmt`, `go vet ./...`, `make lint`, and `make test`.
5. Open a pull request describing *what* and *why*.

## Coding standards

- Follow standard Go idioms and the [Effective Go](https://go.dev/doc/effective_go)
  conventions already used in the codebase.
- Keep third-party dependencies minimal. If you add a dependency, explain why
  it's necessary in the PR.
- Prefer small, focused packages over one big ball of code.
- Write useful log messages (structured fields, not `fmt.Sprintf` into a string).
- **Destructive paths must always pass through `guard.CheckAndExecute`** — never
  call a Docker/Kubernetes delete directly without a protection check.

## Testing

- Every feature or bug fix should ship with tests.
- Run the full suite before opening a PR:
  ```bash
  make test     # go test -v -race -coverprofile=coverage.out ./...
  ```
- If you touch the guard, review queue, or notification code, cover the
  interactive/non-interactive and dry-run variants.

## Commits & pull requests

- Use **conventional commit** messages: `feat:`, `fix:`, `docs:`, `test:`,
  `refactor:`, `chore:`, `ci:`.
- One logical change per commit.
- Keep PRs small and reviewable. A PR that mixes two unrelated features is hard
  to review and likely to be closed.
- Rebase off the latest `main` before finalizing.

## Reporting bugs

Open an issue using the [bug report template](.github/ISSUE_TEMPLATE/bug_report.md).
Include:

- The exact command you ran (with flags).
- Your `config.yaml` (redact webhook URLs, tokens, and secrets).
- Aegis version (`aegis --version`).
- Whether you ran with `--dry-run` and what it reported.
- Anything destructive: what *actually* happened vs. what you expected.

## Security

Find a security issue? Do **not** open a public issue. See
[SECURITY.md](SECURITY.md) for the private reporting path. This is especially
important for a tool that can delete resources.