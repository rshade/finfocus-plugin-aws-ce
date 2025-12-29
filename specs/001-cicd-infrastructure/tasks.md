# Tasks: Establish CI/CD Infrastructure

**Status**: In Progress
**Spec**: [spec.md](./spec.md) | **Plan**: [plan.md](./plan.md)

## 1. Setup Phase
> **Goal**: Initialize project structure and prepare for configuration.

- [x] T001 Verify project root and git initialization `.`

## 2. Foundational Phase
> **Goal**: Essential prerequisites for CI/CD.

- [x] T002 Ensure Go module files are valid and tidy `go.mod`

## 3. User Story 1: Automated Testing Pipeline (P1)
> **Goal**: As a developer, I want code changes to automatically run tests and linting so that I can catch issues before merging.
> **Independent Test**: Push code to a branch and verify the `test` workflow executes successfully in GitHub Actions.

- [x] T003 [US1] Create GitHub Actions test workflow in `.github/workflows/test.yml`

## 4. User Story 2: Automated Release Process (P2)
> **Goal**: As a maintainer, I want releases to be created automatically when pull requests are merged so that new versions are published consistently.
> **Independent Test**: Merge a release PR and verify binaries are built and published to GitHub Releases.

- [x] T004 [US2] [P] Create Goreleaser configuration for single-binary builds in `.goreleaser.yaml`
- [x] T005 [US2] [P] Create Release Please configuration in `release-please-config.json`
- [x] T006 [US2] [P] Create Release Please manifest in `.release-please-manifest.json`
- [x] T007 [US2] Create GitHub Actions release workflow in `.github/workflows/release.yml`
- [x] T008 [US2] Create GitHub Actions release-please workflow in `.github/workflows/release-please.yml`

## 5. User Story 3: Local Development Commands (P3)
> **Goal**: As a developer, I want consistent local build/test commands so that I can verify changes before pushing.
> **Independent Test**: Run `make test`, `make lint`, `make build` locally and verify success.

- [x] T009 [US3] Create Makefile with test, lint, build, ensure targets in `Makefile`

## 6. Polish & Documentation Phase
> **Goal**: Update documentation and cross-cutting concerns.

- [x] T010 [P] Update Agent documentation with CI/CD commands in `AGENTS.md`
- [x] T011 [P] Update Gemini documentation with CI/CD commands in `GEMINI.md`
- [x] T012 [P] Update Claude documentation with CI/CD commands in `CLAUDE.md`
- [x] T013 Update README with build and test instructions in `README.md`

## Dependencies

1. **Setup & Foundational** (T001-T002) MUST complete before any User Stories.
2. **User Story 1** (Testing) is P1 and should be implemented first to ensure quality.
3. **User Story 2** (Release) depends on the existence of config files (T004, T005, T006) before workflows (T007, T008) can be fully effective.
4. **User Story 3** (Makefile) can be implemented independently but effectively standardizes the commands used in CI.
5. **Polish** (T010-T013) should be done last to reflect the final state of the infrastructure.

## Implementation Strategy

1. **MVP**: Implement US1 (Testing Pipeline) to establish the quality gate.
2. **Expansion**: Implement US2 (Release Process) to enable automated delivery.
3. **Developer Experience**: Implement US3 (Makefile) to standardize local workflows.
4. **Finalization**: Update documentation to match the new infrastructure.
