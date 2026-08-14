# Security

Thanks for taking security seriously! Because Aegis deletes resources, we
appreciate careful eyes on the parts that keep those deletions safe.

## Reporting a vulnerability

Please **don't open a public issue** for security problems. Instead, report
privately through GitHub's vulnerability reporting:

- https://github.com/Orinameh/aegis/security/advisories/new

This keeps the issue quiet until we've had a chance to look and release a fix.

Helpful details to include:

- What happened (a short description).
- How to reproduce it (commands and config, with any secrets redacted).
- If it seems to bypass protection guards, the confirmation prompt, or the
  override-token check, mention that — it's the most important thing we'd want
  to know.
- Which Aegis version you're using.

We'll try to acknowledge reports within a few business days.

## What we consider when assessing reports

A few things worth knowing about how Aegis is designed:

- **It runs with your permissions.** When you run `aegis`, it uses your Docker
  socket and Kubernetes credentials. Its guards and prompts are there to stop
  mistakes — they aren't meant to stand in the way of a hostile user on the
  same machine.
- **The override token isn't a password.** Its format (`type/namespace/name`
  with `*/*/*` wildcards) is easy to guess, so treat it as a convenience flag,
  not access control.
- **No network server.** Aegis doesn't listen on any port or accept remote
  commands. The only outbound network call is the optional webhook
  notification in `aegis check`.
- **The safest paths are the defaults.** Dry-run and interactive mode exist so
  nothing gets deleted without you knowing. Anything that skips confirmation
  before a real delete is a trade-off, not an upgrade.

## The deletion guard

Every deletion runs through a protection guard that requires permission first.
No Docker or Kubernetes resource is ever deleted directly.

1. **Protection check** — the guard matches the resource against protection
   rules (critical, strict, warning) and the permission must be approved before
   anything can proceed. Critical resources are always denied.
2. **Confirmation prompt** — before destroying a resource, Aegis shows what
   it's about to delete and asks `Are you sure you want to delete this
   resource? (y/N):`. It only proceeds on an explicit `y`/`yes`.
3. **Deliberate opt-outs** — the prompt is skipped only when you ask for it:
   `--auto-approve`, `--interactive=false` (unattended runs; denials go to a
   review queue instead), or `--dry-run` (nothing is deleted at all).

So by default, in interactive mode, **every** deletion is confirmed with a
yes/no prompt — this is the intended safety behavior, not an afterthought.

If something you found seems to involve one of the points above, it's probably
worth reporting even if you're not sure.

## Supported versions

| Version | Status |
| ------- | ------ |
| Latest release | Actively supported |
| Older releases | Best-effort only |

Security fixes are released on `main` and tagged as patch releases.