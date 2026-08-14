---
name: Pull request
about: Describe your changes
title: ""
labels: ""
assignees: ""
---

## Description

What does this PR do? Please summarize the change and link related issues.

## Type of change

- [ ] Bug fix
- [ ] New feature
- [ ] Documentation update
- [ ] Refactor / internal change
- [ ] Dependency update
- [ ] CI / build change

## Checklist

- [ ] I have run `go vet ./...` and `make lint` without new issues
- [ ] I have run `make test` and all tests pass
- [ ] I have added tests for new behavior (if code changed)
- [ ] Destructive paths pass through the protection `guard` (no direct deletes)
- [ ] No secrets/credentials are introduced (config samples use placeholders)

## Safety considerations

If this PR touches anything that can delete or modify resources, describe the
protection measures (guard levels, dry-run, confirmation, review queue):

## Screenshots / logs

If applicable, add screenshots or sample output to help explain the change.