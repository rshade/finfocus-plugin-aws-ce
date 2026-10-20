# Contributing to the FinFocus AWS Cost Explorer plugin

Thank you for contributing. This guide covers local development, testing, and
submitting changes to the AWS Cost Explorer plugin.

## Development setup

### Prerequisites

- Git and Make.
- [mise](https://mise.jdx.dev/getting-started.html) to install the development tools.

[mise.toml](mise.toml) pins Go 1.27.2, golangci-lint, goreleaser, Node.js,
Markdown and prose linters, and the vulnerability scanner. The Go version must
match [go.mod](go.mod). The [workflows](.github/workflows) use the same tool pins.
The commitlint dependencies use [package-lock.json](package-lock.json).

Ordinary tests use synthetic credentials and local fake endpoints. Only
explicitly enabled live AWS checks need AWS credentials.

### Getting started

Fork the repository on GitHub, then clone your fork. Replace `YOUR-USERNAME`
with your GitHub username:

```bash
git clone https://github.com/YOUR-USERNAME/finfocus-plugin-aws-ce.git
cd finfocus-plugin-aws-ce
git remote add upstream https://github.com/rshade/finfocus-plugin-aws-ce.git
git fetch upstream
git switch --no-track -c docs/contribution-guide upstream/main

mise trust
mise install
mise exec -- npm ci
mise exec -- vale sync
mise exec -- make develop
mise exec -- make build
mise exec -- make test
mise exec -- make lint
```

`mise exec --` puts the configured tools on `PATH` for a command. If you activate
mise in your shell, you can run the `make` commands below directly. Otherwise,
prefix them with `mise exec --`. The mise tasks, such as `mise run test`, delegate
to the same Makefile targets.

Choose a branch name that describes your change. `make develop` runs `make deps`
and creates the build directory. `make deps` runs `go mod tidy` and
`go mod download`. Review any changes to `go.mod` and `go.sum` before committing.
The build produces `bin/finfocus-plugin-aws-ce`.

For local installation and configuration, see the
[quick-start guide](docs/QUICKSTART.md). For project structure and design
constraints, read [AGENTS.md](AGENTS.md) and [architectural context](CONTEXT.md).

## Code style

### Go code

- Follow standard Go formatting. Run `make fmt` after editing Go files.
- Use descriptive names and keep functions focused.
- Document exported functions and types.
- Handle errors explicitly and preserve useful error context.
- Use the `pluginsdk` environment, mapping, validation, and FOCUS builder helpers
  where applicable. Follow the SDK conventions documented in [AGENTS.md](AGENTS.md).
- Use `zerolog` and the SDK logging helpers for structured logs.
- Keep AWS operations read-only and derive billing values from AWS responses.
- Add or update tests for changed behavior, including error cases.

### Markdown and prose

Use descriptive headings, fenced code blocks with language names, and relative
links to repository files. Run Vale after each documentation edit, including
specification and implementation-plan edits. Fix findings before starting the
next implementation step, then rerun the checks before committing or pushing:

```bash
make lint-md
vale sync
vale --minAlertLevel=suggestion CONTRIBUTING.md
```

Replace `CONTRIBUTING.md` with the files you changed. `make lint-md` checks all
Markdown files using the repository configuration, including its exclusions.
Require zero errors, warnings, and suggestions in changed documents. Vale and
reviewdog skip `CHANGELOG.md` because Release Please generates it. Vale can
exit successfully with suggestions, and reviewdog posts those suggestions to
the pull request. Inspect the diagnostic counts instead of relying on the exit
status. Keep the configured rules active and correct the prose.

For a specification change, also compare each acceptance criterion with the
implementation and its tests. Distinguish implemented test infrastructure from
executed verification, especially for live AWS checks. Vale checks writing style,
so a clean report alone doesn't establish technical accuracy.

### Commit messages

Use conventional commits. Continuous integration checks the commits in each pull request.

| Prefix | Purpose |
| --- | --- |
| `feat:` | New features |
| `fix:` | Bug fixes |
| `docs:` | Documentation changes |
| `test:` | Test additions or changes |
| `chore:` | Maintenance tasks |
| `refactor:` | Code changes that preserve behavior |

For example:

```text
docs: add contributor setup and testing guide
```

Validate the commits on your branch with the locked local tool:

```bash
mise exec -- npx --no-install commitlint --from upstream/main --to HEAD
```

## Dependency updates

[renovate.json](renovate.json) follows the AWS public plugin's update policy:
weekly updates before 04:00 Coordinated Universal Time on Monday, Go restricted to the 1.27 release
line, separate groups for Go major and minor or patch updates, and no updates
to test fixtures under `testdata`. Renovate runs `go mod tidy` when updating Go
modules. Its recommended preset also enables dependency discovery for mise,
GitHub Actions, and npm. Maintainers review dependency pull requests before merging.

When changing Go manually, update both `go.mod` and `mise.toml`, then check the
setup instructions in this guide, [README.md](README.md), and the
[quick-start guide](docs/QUICKSTART.md). Run `mise install` after changing tool
pins and `npm ci` after changes to `package-lock.json`.

## Testing

### Unit tests and the default suite

```bash
make test
```

This runs `go test -count=1 -v ./...`. The suite includes unit tests and the
default integration tests. It doesn't query live AWS services.

### Integration and protocol tests

```bash
make test-integration
```

This launches plugin processes and uses local fake Cost Explorer endpoints.
Run the real FinFocus executable and official SDK suite separately:

```bash
make core-install
make e2e
make conformance
```

`make core-install` builds the revision pinned by the Makefile. Set
`FINFOCUS_CORE_VERSION` to override that revision, or `FINFOCUS_CORE_BINARY`
to use a core executable you already built. Tests never download core themselves.
`make e2e` and its alias `make test-e2e` invoke core with an isolated plugin
registry, synthetic credentials, and a local fake AWS endpoint. Assertions require
non-empty cost results with the expected identity, currency, period, and totals.
`make conformance` runs the official Standard suite against the actual-only
capabilities advertised by the plugin.

### Live AWS checks

Follow [AWS verification](docs/AWS-VERIFICATION.md) for credentials, permissions,
and `make e2e-live`. Live checks require the `awslive` build tag,
`FINFOCUS_AWS_LIVE=true`, and a configured core executable. Cost Explorer requests
are billable. Keep credentials out of source files, fixtures, logs, and reports.
Live execution is separate from offline acceptance and remains unverified for
the current foundation change.

### Coverage and vulnerability checks

```bash
make test-coverage
make vuln
```

The coverage command writes `coverage.out` and `coverage.html`. Open
`coverage.html` in a browser to inspect uncovered code. `make vuln` runs
`govulncheck ./...` and requires that tool on your `PATH`.

## Pull request process

1. Fork the repository and create a descriptive branch from the current `main`.
2. Make a focused change and update relevant tests and documentation.
3. Run `make build`, `make test`, and `make lint` for Go changes.
   Run Markdown and prose checks for documentation changes.
4. Commit using conventional commit messages and push the branch to your fork.
5. Open a pull request against this repository's `main` branch. Link the issue,
   describe the resulting behavior, and include validation results and any limits.
6. Address review feedback and check the workflow results before requesting a merge.

The project uses GitHub Copilot for automated code review. Address its findings
alongside maintainer feedback.

The [continuous integration workflow](.github/workflows/ci.yml) runs separate lint, test, and build
jobs. The test job runs the suite with race detection, integration checks,
the real core test, and official conformance.
Reproduce its checks locally with:

```bash
go test -count=1 -v -race ./...
make test-integration
make core-install
make e2e
make conformance
```

The lint job also runs `make vuln`. That scan allows failures, so
inspect its output when reviewing dependency changes. Separate workflows check
Markdown, prose, and commit messages.

### Pull request checklist

- [ ] Relevant tests and build checks pass locally.
- [ ] Go lint passes for Go changes.
- [ ] Documentation reflects the change and Markdown lint passes.
- [ ] Commit messages follow conventional commits.
- [ ] The description explains the change and records the checks performed.
- [ ] The description explains any skipped checks or remaining failures.

## Reporting issues

Search the [issue tracker](https://github.com/rshade/finfocus-plugin-aws-ce/issues)
for an existing report before opening a new issue. For a bug report, include:

- Output of `go version`, operating system, and architecture.
- Plugin version or commit used to build it.
- Steps to reproduce the problem, with a minimal example when possible.
- Expected and actual behavior.
- Relevant error messages and logs with credentials and sensitive billing data removed.

Describe the use case and expected behavior in feature requests. Include
the related AWS operation when proposing a new billing capability.

## License

By contributing, you agree to license your contributions under the project's
[Apache License 2.0](LICENSE).
