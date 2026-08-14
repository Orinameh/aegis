---
name: Bug report
about: Report a bug to help us improve Aegis
title: "[BUG] "
labels: bug
assignees: ""
---

**Describe the bug**
A clear and concise description of what the bug is.

**Command used**
```bash
aegis <subcommand> <flags>
```

**Configuration**
`config.yaml` (redact webhook URLs, tokens, and any secrets):

```yaml
# ...
```

**Expected behavior**
What you expected to happen.

**Actual behavior**
What actually happened. Include logs / the table output.

**Environment**
- Aegis version (`aegis --version`):
- Go version (`go version`):
- OS / architecture:
- Docker version (if relevant):
- Kubernetes provider & version (local minikube/kind, or hosted AKS/EKS/GKE):

**Did you run with `--dry-run`?**
What did the dry run report?

**If this involved deletion**
What was deleted, what was expected to be protected, and which protection level
applied? This is critical to reproduce safely.

**Additional context**
Add any other context, screenshots, or terminal output here.