# Quickstart: CI/CD Infrastructure

**Feature**: Establish CI/CD Infrastructure

## Overview

This feature establishes the CI/CD pipeline for the project. It includes automated testing, linting, and release management.

## Local Development

To verify your changes locally before pushing:

```bash
# Run all tests
make test

# Run linting
make lint

# Build binary locally (for your current platform)
make build

# Update dependencies
make ensure
```

## Continuous Integration (CI)

On every push to the `main` branch or pull request:
1. **Test Workflow** (`.github/workflows/test.yml`): Runs `golangci-lint` and `go test`. Must pass for merge.

## Release Process

Releases are automated via Release Please and Goreleaser:

1. **Commit Changes**: Use Conventional Commits (e.g., `feat:`, `fix:`).
2. **Pull Request**: Open a PR and get it merged.
3. **Release PR**: The "release-please" bot will open/update a Release PR with the next version and changelog.
4. **Merge Release PR**: When ready to release, merge this PR.
5. **Tag & Build**: GitHub Actions tags the release, and `goreleaser` builds/uploads binaries to the GitHub Release page.
