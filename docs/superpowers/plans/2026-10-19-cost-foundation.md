# Actual-cost foundation implementation plan

Independent file groups can proceed in parallel under the implementation and review skills.

**Goal:** complete the current actual-cost foundation for issues #11 and #31 with real core integration, official capability-aware conformance, and a bounded live AWS pipeline.

**Architecture:** preserve the existing actual-cost query and cache contracts. Test a locally built plugin through the real FinFocus executable with an isolated registry and fake AWS endpoint. Keep live AWS comparisons separately gated and authenticate through the existing Pulumi ESC environment.

**Tech stack:** Go 1.27.2, FinFocus SDK, AWS SDK v2, GitHub Actions, mise, Pulumi ESC.

**Spec:** current contract in `README.md` and `CONTEXT.md`. This plan supersedes historical grouping, pricing, and cache-persistence requirements in spec 004 for this task.

## Global constraints

- Actual costs only. Preserve daily queries, explicit supported resource types, and memory-cache identity isolation.
- Use SDK helpers and structured logging for production changes.
- Never fabricate pricing responses or filter mandatory failures to make conformance pass.
- Correct the official SDK suite upstream to respect declared capabilities, as explicitly approved by the owner.
- Keep offline tests independent of AWS credentials. Never create cloud resources for verification.
- Live execution requires explicit opt-in and the existing `tailscale-phase-2/aws-oidc` environment.
- Bound live query dates to seven days, AWS attempts, pagination, and execution time. Missing authorization is not a successful cost comparison.
- Integrate a reproducible upstream SDK commit or release; do not commit a local filesystem replacement.

## Review priorities

- EC2 resource group IDs must not become service names.
- Invalid FOCUS inputs must return a meaningful error, not an incomplete successful record.
- Core command success with empty results must fail the E2E assertions.
- An advertised capability must pass its checks; unsupported capabilities must return the correct protocol status.
- No AWS credentials or inherited endpoint overrides may leak into offline tests or logs.

## Task 1: correct current FOCUS translation

**Files:** `internal/pricing/aggregate.go`, `internal/pricing/calculator.go`, relevant pricing tests, and client mapping only if required.

- [x] Add a failing regression for an EC2 response grouped by instance ID: known EC2 service name, preserved group key, and FOCUS resource ID.
- [x] Add failing regressions for FOCUS input that lacks currency or identity.
- [x] Implement error propagation and correct resource identity without changing account/service queries or cache keys.
- [x] Run pricing and client tests and lint.

## Task 2: exercise the real core executable offline

**Files:** `test/e2e/e2e_test.go`, a reusable core process helper under `internal/testutil/`, and test fixtures as needed.

- [x] Replace the stale fake-only gRPC path with a real core command-line tool invocation and isolated registry.
- [x] Require an explicit core binary built from a pinned source revision; do not silently download from tests.
- [x] Handle reservation/Savings Plans discovery and cost queries in the fake endpoint and assert their parameters.
- [x] Validate row identity, adapter, currency, dates, exact expected total, and source breakdown.
- [x] Prove missing plugin data or AWS failures cannot yield a successful test solely from a zero exit code.
- [x] Run the gated offline E2E test against the pinned core binary.

## Task 3: integrate official conformance

**Files:** upstream `finfocus-spec/sdk/go/testing` suite and its regression tests; plugin `test/conformance`, `go.mod`, and `go.sum`.

- [x] Add upstream regressions for actual-only, projected-only, and falsely advertised capabilities.
- [x] Make mandatory suite checks respect advertised capabilities while checking unsupported statuses and preserving legacy behavior.
- [x] Validate upstream changes and commit them on an isolated feature branch.
- [x] Run the official Standard suite through a transparent client-to-server adapter against the plugin binary and fake AWS endpoint.
- [x] Require the suite result to pass without manufactured responses or local result exclusions.
- [x] Consume the reviewed upstream commit through a reproducible module version.

## Task 4: live verification and developer commands

**Files:** `test/live`, `Makefile`, `mise.toml`, `.github/workflows/ci.yml`, a dedicated E2E workflow, `docs/AWS-VERIFICATION.md`, and contributor/roadmap documentation.

- [x] Correct stale protocol-version checks and the independent live cost baseline to match documented commitment/residual metrics.
- [x] Exercise the real core path with live AWS data as well as direct protocol/FOCUS validation.
- [x] Add `make e2e`, `make test-e2e`, and `make conformance` with clear offline/live gating.
- [x] Add offline core E2E and official conformance to pull-request checks.
- [x] Add a trusted, explicitly enabled live workflow using Pulumi ESC. Fork pull requests must not obtain credentials.
- [x] Verify local live execution if existing environment access is available; otherwise record the precise external blocker.
- [x] Update stale spec-004 acceptance criteria and documentation to the approved current scope.
- [x] Run build, full tests, race tests, lint, actionlint, Markdown/prose checks, and independent review before pushing.

## Verification notes

The owner requested offline continuation after the Pulumi session could not open the existing environment. Live execution remains unverified; the workflow stays off pending maintainer configuration and an authorized run.

Independent reviews covered the core harness, conformance adapter, production translation and identity mapping, and live workflow. The date-window review finding was fixed with a recent UTC window.

The upstream change is [FinFocus SDK PR #652](https://github.com/rshade/finfocus-spec/pull/652). The plugin consumes `v0.7.9-0.20261020013049-bc3a94391af7` without a local replacement. Official Standard conformance passes all 22 applicable checks; two Advanced checks remain outside the requested level.
