# AWS Cost Explorer Plugin v0.1.0 - Task Breakdown

<!-- markdownlint-disable MD013 MD060 -->

**Goal:** Release v0.1.0 as a production-ready actual-cost plugin backed by AWS Cost Explorer, with spec compliance, testing and documentation. Docker packaging is deferred in this run.

**Roadmap:** Phase-by-phase implementation, skipping PR gates until a working v0.1.0 exists.

---

## Release and tag rules (family standard, updated 2026-10-06)

Everything below was learned the hard way on opencost and azure-public. Follow it for the first release; do not
rediscover it.

### Release Please settings

Every release-please config needs these on the `.` package, and a test that fails when any is wrong:

1. `"include-component-in-tag": false`. Without it a config that sets `package-name` creates tags like
   `finfocus-plugin-<name>-v0.1.0`. `release.yml` runs on `release: created`, GoReleaser parses the tag as semver,
   fails, and the release has no binaries (opencost, 2026-10-06; azure-public sets the key).
2. `"initial-version": "0.1.0"`. Without it the first release PR can propose 1.0.0 (opencost issue 77).
3. `"bump-patch-for-minor-pre-major": true`, as in aws-public. With `false`, a `feat:` commit before 1.0 bumps the
   minor on every release (flexera, 2026-10-06).
4. Every other key matches aws-public's `release-please-config.json` exactly: `bump-minor-pre-major` true, the same
   `changelog-sections`, `release-type` go. Only `package-name`, `include-component-in-tag` and `initial-version`
   are added. Diff the file against aws-public before opening the PR.
5. `.release-please-manifest.json` keeps `"."` at `0.0.0` until the first release PR merges. A `0.1.0` before then
   records 0.1.0 as already shipped. After that, release PRs bump it every time, so a test must accept any valid
   `MAJOR.MINOR.PATCH` at or above 0.1.0 and never pin an exact version.
6. The generated `CHANGELOG.md` fails markdownlint (MD012, MD004). Put it in `.markdownlintignore`.
7. Wrap commit bodies to 72 columns: commitlint `body-max-line-length` is 100 and CI checks every commit in the PR.

Use `googleapis/release-please-action@v5.0.0`, not v4.

### GoReleaser settings

The release uploads archives and `checksums.txt` only, as aws-public and azure-public do:

- No `dockers`, `dockers_v2`, `homebrew_casks`, `brews` or `nfpms` section, and no deb or rpm. `release.yml` has
  no registry login, no `packages: write` and no tap token. opencost's config had all three and its package
  scripts never existed.
- No other workflow publishes a container image. vantage kept a tag-triggered `docker.yml` that built a missing
  Dockerfile, so every release run was red even though GoReleaser succeeded. Add a test that fails on
  `docker/build-push-action` in any workflow and on those GoReleaser sections.
- Use only template fields GoReleaser defines. opencost's `{{if .Dirty}}` failed both release runs with
  `map has no entry for key "Dirty"`; use `{{ .GitTreeState }}`.
- Use current keys: `goreleaser check` must exit 0, so no deprecated `archives.format`; use `formats`.
- Names say `finfocus-plugin-<name>`, never `pulumicost`.

Proof before the first tag, on the pinned GoReleaser version, in the repo or a copy:

```bash
GORELEASER_CURRENT_TAG=v0.1.0 goreleaser release --snapshot --clean --skip=publish
goreleaser check
```

Both must succeed, and the build must write archives and `checksums.txt`. A config that was never built is not a
working config. Add a test that reads the config and the manifest and fails when a setting above is wrong. Break
check: reintroduce each defect in turn and the test fails.

### Workflow files and the release token (`RELEASE_PLEASE_TOKEN`)

The pattern is aws-public's, from release-please to GoReleaser. Copy the files, do not reinvent them:

- `release-please.yml`: copy aws-public's. `googleapis/release-please-action@v5.0.0` with
  `token: ${{ secrets.RELEASE_PLEASE_TOKEN }}`, triggers `push` to `main` and `workflow_dispatch`, permissions
  `contents`, `issues` and `pull-requests` write. azure-public and opencost append `|| secrets.GITHUB_TOKEN`; both
  shapes are the family pattern. A probe step (as in `finfocus-spec`) is optional hardening, not required.
- `release.yml` (single-binary plugins): copy opencost's or azure-public's. Triggers `release: types: [created]`
  and `workflow_dispatch` with a required `tag` input, `permissions: contents: write`, checkout at
  `ref: ${{ inputs.tag || github.event.release.tag_name }}` with `fetch-depth: 0`, `actions/setup-go@v7` with
  `go-version-file: go.mod`, `goreleaser/goreleaser-action@v7` with `args: release --clean`, and `GITHUB_TOKEN` only.
  aws-public's `release.yml` is the multi-region variant and is not the template for a single-binary plugin.
- Not allowed: a `push: tags` trigger (release-please creates the tag through the API, so it never fires),
  `--rm-dist` (removed in GoReleaser v2), `actions/checkout` older than v7, `goreleaser-action` older than v7,
  `release-please-action` v4, and `google-apis/release-please-action` (the org is `googleapis`).

Why the token matters: events created with `GITHUB_TOKEN` do not trigger other workflows, so a release PR opened that
way never runs `Test` or `Commitlint`, and a published release never fires `release.yml`. The secret is a
fine-grained token limited to this repo with Contents, Pull requests and Issues read and write, and Metadata read.
The agent never reads or sets it. The check is a state check, not a decision: before the first release PR the owner
confirms the secret exists (Settings, Secrets) and that a `workflow_dispatch` run of `Release Please` does not fail
with `Input required and not supplied: token` or `Bad credentials`. An expired token is a non-empty string, so
`||` does not rescue it (opencost: 16 of 16 runs failed). Document the token, its scopes, its expiry and how to rotate
it in the repo's `CLAUDE.md` or `CONTRIBUTING.md`.

### Release steps

- Merge the release PR, then check: the tag is plain `vX.Y.Z`, the GoReleaser run is green, and
  `gh release view vX.Y.Z --json assets --jq '.assets | length'` is above 0. A merged release PR is not a release.
- Recovery if the tag or the run is wrong: never hand-create a tag. The owner deletes the bad release and tag,
  then re-runs `release.yml` with the existing tag (`workflow_dispatch`). To make Release Please create the release
  again, its docs describe re-triggering with the labels `autorelease: pending` and `release-please:force-run` on
  the merged release PR (documented for a PR stuck at `autorelease:closed`; not confirmed for `tagged`, so
  treat it as untested). A tag points at the release PR's merge commit, so fixes merged afterwards ship in the next release.
- Release tooling (goreleaser, release-please, the release workflow) is ask-first: change it only when the task or
  the invocation says so. The agent never deletes or moves a tag or release.

### Registry entry (after the release has assets)

Open a pull request to `rshade/finfocus`, modelled on #1697 (azure-public) and #1720 (opencost):

- `internal/registry/registry.json`: one entry. Provider, capabilities from the allowed list in
  `registry_json_test.go`, `asset_prefix` `finfocus-plugin-<name>` with `version_prefix: false` for plain tags,
  `min_spec_version` equal to the spec in the release's `go.mod`. Replace a stale entry for the same plugin: the
  registry has no alias field. Update tests that name the old entry.
- A docs page `docs/src/content/docs/plugins/<name>.md`, a row in the compatibility matrix, the FAQ plugin table, the
  docs table of contents, and the `finfocus-install` agent skill references. Only claim what the plugin README and
  manifest say; do not label a profile or feature experimental unless the owner chose that.
- Validate with a binary built from the branch and a fresh temporary `FINFOCUS_HOME`: `plugin list --available`
  shows the entry, `plugin install <name>` prints `Checksum verified (SHA256)`, `plugin list` shows the version.
  Also run `plugin inspect <name> <type>`: opencost failed with "capability discovery not implemented", so record a
  failure as a plugin gap in the PR.

State at the start of run 2 (2026-10-06): main was at `77b1c17` with spec
v0.7.0 and the owner task block. The package name was already `finfocus-plugin-aws-ce`
and the release manifest was already `0.0.0`. Release config keys, token wiring,
workflow actions and deprecated archive keys needed correction. REL-1 to REL-4
now provide the configuration guards, copied family workflows, snapshot archives
and spec v0.7.5. This run makes local commits only; no tag, release or push.

Run 2 scope: REL-1 to REL-4, CE-6.5, CE-6.6, CE-6.10, CE-6.11,
CE-2.1 to CE-2.3, CE-3.1 and CE-4.3. CE-3.1 Docker remains blocked on the
excluded CE-3.2 owner decision. CE-3.2 to CE-3.4, CE-4.1, CE-4.2, CE-5.1,
CE-1.7 and Phase 7 are deferred with owner PM. Live CE-6.1 and CE-5.1 are
blocked on credentials. Registry changes in core and release publication are
outside the write boundary. See the run report's not-delivered register.

## Scope (decided 2026-10-01)

**v0.1.0 is a complete, correct actual-cost plugin (Tier A).** Everything else is written up
below as Tier B tasks (CE-7.x) for a second run, so all 40 open issues are covered here. Seven
issues are duplicates and two are stale: they are marked in the Issue Index and no GitHub
change is made by this plan. The owner closes or merges them.

| Tier | Issues | Tasks |
| --- | --- | --- |
| A, v0.1.0 | #11, #12, #31, #37, #40 to #48, #50 to #53, #55 | CE-1.x to CE-5.x and CE-6.x |
| B, second run | #8, #13, #21, #27, #29, #30, #32, #33, #34, #36, #38, #49, #54 | CE-7.x |
| Duplicates | #19, #20, #28 (of #37), #22 (of #13), #24 (of #8), #25 (of #36), #26 (of #38) | none, see Issue Index |
| Stale | #3 (cites `pulumicost-spec` and a field that does not exist), #23 (satisfied by spec v0.7.0) | none |

### Historical research findings (2026-10-01; code fixes tracked below)

Sources and confidence tags are in `finfocus-pm` research notes (`aws-ce-api-facts.md`,
`aws-ce-issue-reconcile.md`). The ones that change the work:

- `GetCostAndUsage` **cannot group or filter by `RESOURCE_ID`** as the current code assumes.
  Per-resource cost needs `GetCostAndUsageWithResources`, which needs an opt-in by the management
  account, covers **the last 14 days only**, is daily (hourly for EC2 only), and lags up to 48
  hours. The "14 months" lookback is for account and service totals. Whether it works for non-EC2
  resources is unverified. The old code filtered on the **full ARN**; CE-6.7 now uses the EC2 instance id. See
  CE-6.7.
- Each paginated request costs **$0.01**, there is no published rate limit, only
  `LimitExceededException`, and AWS recommends a cache layer. See CE-6.8.
- `ActualCostResult` has **no `metadata` field** (issue #52's code does not compile). Use
  `FocusCostRecord.extended_columns`.
- The SDK already provides a health endpoint and web/CORS config (`WebConfig`). Do not hand-roll
  #43; wire the SDK's.
- Spec blockers cited by #36 and #38 (`finfocus-spec` #314, #315) are closed.
- Silent-failure defects in the original code are listed in CE-6.2 to CE-6.3 with file and line.
- **No live AWS account exists and no sandbox, test data or free tier was found.** Moto returns
  only canned results; LocalStack Cost Explorer is a paid tier. Verification therefore uses the
  contract fixtures in `internal/client/testdata/ce-contract/` (CE-6.1) and an opt-in live test.

---

## Current State

### Checkout vs. GitHub Mismatch

| Item | Current | Should Be | Action |
|------|---------|-----------|--------|
| Local directory | `finfocus-plugin-aws-ce` | N/A (local dir name doesn't matter) | None |
| Remote URL | `git@github.com:rshade/finfocus-plugin-aws-ce.git` | `git@github.com:rshade/finfocus-plugin-aws-ce.git` | None; already named correctly |
| `go.mod` module | `github.com/rshade/finfocus-plugin-aws-ce` | `github.com/rshade/finfocus-plugin-aws-ce` | None; migration complete |
| Logging component | `finfocus-plugin-aws-ce` | `finfocus-plugin-aws-ce` | None; migration complete |

**Drift Summary:** GitHub repo is renamed to `finfocus-plugin-aws-ce`, and local checkout, go.mod, and all references have been updated to use the new name.

### Toolchain Baseline (Completed 2026-09-30)

✅ **COMPLETE** - All ecosystem baseline tooling installed and configured:

| Component | Version | Status | Notes |
|-----------|---------|--------|-------|
| **Language & Build** | | | |
| Go | 1.27.1 | ✅ | Updated in go.mod and mise.toml |
| `finfocus-spec` | v0.7.5 | ✅ | Unblocks per-request credentials + FOCUS 1.4 |
| ax-go | v0.7.0+ | ✅ | Arrives transitively via spec |
| goreleaser | 2.18.2 | ✅ | Multi-platform release build configuration updated |
| **Code Quality & Testing** | | | |
| golangci-lint | 2.14.0 | ✅ | Matches finfocus baseline |
| govulncheck | 1.8.0 | ✅ | Workflow integrated (non-blocking for GO-2026-6443) |
| **Documentation & Linting** | | | |
| markdownlint-cli2 | 0.23.3 | ✅ | .markdownlint.json configured for 120-char lines |
| vale | 3.20.0 | ✅ | .vale.ini configured for Google style |
| actionlint | 1.7.12 | ✅ | All workflows validated |
| commitlint | 20.5.2 (config-conventional 20.5.3) | ✅ | Conventional commit validation |
| **Naming Migration** | | | |
| Legacy name → FinFocus | — | ✅ | All tracked files renamed; 0 remaining in tracked code |
| `.github/workflows` | — | ✅ | Updated to use mise-action with explicit tool pins |
| Manifest files | — | ✅ | Updated descriptions and paths |
| **Configuration** | | | |
| mise.toml | Latest | ✅ | Tool versioning pinned to ecosystem baseline |
| .goreleaser.yaml | Updated | ✅ | Asset naming fixed for plugin installer compatibility |
| .github/workflows/test.yml | Updated | ✅ | go@1.27.1, golangci-lint@2.14.0, govulncheck@1.8.0 |
| .github/workflows/release.yml | Updated | ✅ | Single-binary family template; release-created and manual-tag triggers |
| .github/workflows/commitlint.yml | Created | ✅ | Conventional commit validation on PRs |
| .github/workflows/prose-lint.yml | Created | ✅ | Vale + markdownlint via reviewdog |

**Validation Results (2026-09-30):**

- ✅ `go build ./...` - Pass
- ✅ `go vet ./...` - Pass
- ✅ `go test -count=1 ./...` - All packages pass
- ✅ `actionlint .github/workflows/*.yml` - All workflows pass
- ✅ Remaining legacy references in tracked files: **0** (all removed)
- ⚠️ Legacy references in untracked specs/004-core-cost-e2e/: 4 files (intentionally not edited per safety rules)

### Historical Build & Test Results (2026-09-30 Post-Toolchain)

```bash
$ go build ./...
# ✅ Pass

$ go vet ./...
# ✅ Pass

$ go test -count=1 ./...
# ✅ All packages pass
?       github.com/rshade/finfocus-plugin-aws-ce/cmd/plugin             [no test files]
ok      github.com/rshade/finfocus-plugin-aws-ce/internal/client        0.064s
ok      github.com/rshade/finfocus-plugin-aws-ce/internal/pricing       0.016s
ok      github.com/rshade/finfocus-plugin-aws-ce/internal/test          0.004s
ok      github.com/rshade/finfocus-plugin-aws-ce/test/e2e               0.005s
```

### RPC Implementation Status (run 2)

| RPC | Status | Notes |
| --- | --- | --- |
| `GetActualCost()` | Implemented | Service totals and EC2 resource queries; fake endpoint verification |
| `GetProjectedCost()` | Outside release scope | Returns `codes.Unimplemented` with CE-6.10 |
| `Supports()` | Implemented | AWS identity validation, actual-only capabilities |
| `GetPluginInfo()` | Implemented | Discovery metadata, spec v0.7.5, actual-only capabilities |
| `BatchCost` | SDK fallback implemented | Validated batch limits and concurrent actual-cost requests |
| `GetRecommendations()` | Deferred | Phase 7 |

### AWS Cost Explorer API Usage

- **Operations used:** `GetCostAndUsage` for service totals and
  `GetCostAndUsageWithResources` for EC2 instances.
- **Retry logic:** Exponential backoff with configurable retry count.
- **Lookback limits:** 14 months for service totals; 14 days for resource data.
- **Request cost:** $0.01 per paginated request; page counting and an optional
  per-minute request budget are implemented.
- **Resource filtering:** Bare EC2 id plus the EC2 service filter; no full ARN
  goes into `RESOURCE_ID`. Request `tags` remain ignored and documented.
- **Caching:** Memory and disk with hashed file names; recent/estimated TTL
  15 minutes and closed historical TTL 24 hours; `expires_at` supplied.
- **Verification:** Offline contract fixtures and local fake HTTP only;
  live AWS account behavior remains blocked on credentials.

### Open Issues Summary

- **Total:** 40 open issues (8 closed, 48 total)
- **In Scope for v0.1.0 (Tier A):** 18 issues (CE-1 to CE-6)
- **Second run (Tier B):** 13 issues (CE-7)
- **Duplicates / stale:** 7 and 2, marked in the Issue Index

### Current release limitations

1. Live account and core E2E verification remain blocked on credentials.
2. Resource attribution is implemented for EC2 only, with account opt-in and
   a 14-day window. Billing data lags by 24 hours or more.
3. FOCUS requires caller `billing_account_id`; commitment and invoice detail
   fields not present in queried AWS data remain unset.
4. Container packaging, health endpoint, full deployment docs and contribution
   guide are deferred to the PM; release workflows publish archives only.
5. Whole-repository Markdown lint retains frozen-history errors. Vulnerability
   scanning retains the known gRPC `GO-2026-6443` finding.

---

## Phase 1: Spec Conformance (CE-1.x)

### CE-1.1: Upgrade finfocus-spec to v0.7.0 and Go to 1.27.1

**Status:** DONE, `grep -q 'finfocus-spec v0.7.0' go.mod && grep -q '^go 1.27.1' go.mod && go build ./... && go test -count=1 ./...`, already satisfied (go.mod go 1.27.1, finfocus-spec v0.7.0, ax-go v0.7.0 transitive, `go mod tidy` clean, build+tests pass); break check: same grep for v0.9.9 exits 1.

**ID:** CE-1.1
**Description:** Update `go.mod` to use finfocus-spec v0.7.0 and Go 1.27.1. This unblocks per-request credentials (new `PerRequestCredentialConsumer` interface), FOCUS 1.4 billing columns (`invoice_detail_id`, `commitment_program_eligibility_details`), and ax-go v0.7.0 integration.

**Files Modified:**

- `go.mod` - Update Go version to 1.27.1, update finfocus-spec to v0.7.0
- `go.sum` - Auto-updated by `go mod tidy`

**Acceptance Criteria:**

- Go version in go.mod is 1.27.1
- finfocus-spec version in go.mod is v0.7.0 (or later)
- `go mod tidy` succeeds
- `go build ./...` succeeds with Go 1.27.1
- All tests still pass: `go test ./...`
- Binary builds without deprecation warnings
- ax-go v0.7.0 arrives transitively (plugins do not import ax-go directly)

**Related Issues:**

- [#23](https://github.com/rshade/finfocus-plugin-aws-ce/issues/23) - Update finfocus-spec to enable gRPC reflection

---

### CE-1.2: Implement Supports() RPC with Custom Logic

**Status:** DONE, `go test -count=1 ./...`, all packages pass (pricing 0.128s); break check: Supports returned true for provider gcp, TestSupports/provider_gcp and TestSupports_GRPC failed, then the provider check was restored.

**ID:** CE-1.2  
**Description:** Add custom `Supports()` method to Calculator to check if a resource is supported by aws-ce. AWS Cost Explorer only supports AWS resources with proper resource IDs or ARNs, so Supports() must validate request data quality.

**Files Modified:**

- `internal/pricing/calculator.go` - Add `Supports()` method

**Acceptance Criteria:**

- `Supports()` method added to Calculator struct
- Returns true for "aws" provider only
- Returns true when ResourceId or ARN is present and valid
- Returns false for resources without identifiers
- Returns proto ErrorCode for invalid input
- Unit tests cover: valid ARN, valid ResourceId, missing identifiers, non-AWS resources
- All tests pass: `go test ./...`

**Related Issues:**

- [#41](https://github.com/rshade/finfocus-plugin-aws-ce/issues/41) - Add Supports() RPC implementation

---

### CE-1.3: Implement GetPluginInfo() RPC

**Status:** DONE, `go test -count=1 ./...`, all packages pass (pricing 0.137s); break check: GetPluginInfo name set to not-aws-ce, TestGetPluginInfo and TestGetPluginInfo_GRPC failed, then the name was restored.

**ID:** CE-1.3  
**Description:** Add `GetPluginInfo()` method to Calculator to return plugin metadata (name, version, supported providers, supported RPCs).

**Files Modified:**

- `internal/pricing/calculator.go` - Add `GetPluginInfo()` method
- `cmd/plugin/main.go` - Update version constant (optional)

**Acceptance Criteria:**

- `GetPluginInfo()` method returns PluginInfo proto message
- Plugin name: "aws-ce"
- Plugin version: "0.1.0" (or auto-detected from git tag)
- Supported providers: ["aws"]
- Supported RPCs: ["GetActualCost", "Supports", "GetPluginInfo", "GetProjectedCost"]
- Unit test verifies response structure
- All tests pass: `go test ./...`

**Related Issues:**

- [#40](https://github.com/rshade/finfocus-plugin-aws-ce/issues/40) - Add GetPluginInfo() RPC implementation

---

### CE-1.4: Implement Trace ID Propagation

**Status:** DONE, `go test -count=1 ./...`, all packages pass (pricing 0.120s); break check: `traceLogger` returned `c.logger` without `WithTrace`, `TestSupports_LogTraceID`, `TestGetPluginInfo_LogTraceID`, `TestGetActualCost_LogTraceID`, and `TestGetActualCost_MalformedARNLogTraceID` failed, then `WithTrace` was restored.

**ID:** CE-1.4  
**Description:** Extract and log `trace_id` from gRPC metadata to enable distributed tracing. Trace ID should appear in all structured log entries for this request.

**Files Modified:**

- `internal/pricing/calculator.go` - Extract trace_id from context metadata
- All RPC handler methods - Pass trace_id to structured logger

**Acceptance Criteria:**

- Trace ID extracted from gRPC metadata key `finfocus-trace-id`
- Trace ID added to all log entries for this request
- When trace ID is missing, use empty string (graceful degradation)
- Integration test verifies trace ID in stderr JSON logs
- All tests pass: `go test ./...`

**Related Issues:**

- [#46](https://github.com/rshade/finfocus-plugin-aws-ce/issues/46) - Implement trace ID propagation for distributed tracing

---

### CE-1.5: Use Proto ErrorCode Enum for Standardized Errors

**Status:** DONE, `go test -count=1 ./...`, all packages pass (pricing 0.158s, including TestCEContractFixtures); break check: time-range ErrorDetail set to ERROR_CODE_UNSPECIFIED, TestGetActualCost_ErrorDetails/invalid_time_range failed, then ERROR_CODE_INVALID_TIME_RANGE was restored.

**ID:** CE-1.5  
**Description:** Replace custom error strings with proto ErrorCode enum values. All errors returned by RPC methods should use the standardized error codes defined in finfocus-spec.

**Files Modified:**

- `internal/pricing/calculator.go` - Use ErrorCode enum
- `internal/client/client.go` - Standardize error codes

**Acceptance Criteria:**

- All error returns use proto ErrorCode (not custom strings)
- `ERROR_CODE_UNSUPPORTED_REGION` for region mismatches
- `ERROR_CODE_INVALID_RESOURCE` for missing identifiers
- `ERROR_CODE_INVALID_ARGUMENT` for bad input (date range, ARN format)
- Unit tests verify error codes in responses
- All tests pass: `go test ./...`

**Related Issues:**

- [#47](https://github.com/rshade/finfocus-plugin-aws-ce/issues/47) - Use proto ErrorCode enum for standardized error handling

---

### CE-1.6: Implement Per-Request Credentials Support (v0.7.0+)

**Status:** DONE, `go test -count=1 ./...`, all packages pass (pricing 0.107s, including TestCEContractFixtures); break check: second per-request call reused the first client, TestGetActualCost_PerRequestCredentials failed (serving clients = 2 and 0 calls), then a distinct client was restored.

**ID:** CE-1.6  
**Description:** Implement opt-in per-request AWS credentials support via the `PerRequestCredentialConsumer` interface (new in finfocus-spec v0.7.0). This allows hosts to pass AWS credentials (API keys, STS tokens, role ARNs) per-request via gRPC metadata (`x-finfocus-credential-*` headers), enabling multi-account and multi-credential scenarios without environment variables.

**Background:**

AWS Cost Explorer supports multiple authentication methods: environment variables (current), CLI profiles, and temporary credentials. Per-request credentials enable plugins to work across multiple AWS accounts dynamically without restart.

**Files Modified:**

- `internal/pricing/calculator.go` - Implement `PerRequestCredentialConsumer` interface
- `internal/client/client.go` - Extract and use per-request credentials when available
- `cmd/plugin/main.go` - Register `PerRequestCredentialConsumer` capability

**Acceptance Criteria:**

- Calculator implements empty `ConsumesPerRequestCredentials()` method (marks opt-in)
- `GetPluginInfo()` response includes metadata `supports_per_request_credentials: "true"`
- Cost queries extract credentials via `pluginsdk.ExtractCredentials(ctx)`
- Per-request credentials take precedence over environment variables
- Falls back gracefully to default credential chain when no per-request creds provided
- Request credentials names validated: alphanumeric + hyphens, max 64 chars, max 16 entries total
- Credentials values masked in logs (never logged, only used)
- Integration test verifies multi-account scenarios with different credentials per request
- All tests pass: `go test ./...`

**Note:** This coexists with the existing default AWS credential chain (env vars, ~/.aws/credentials, IMDSv2). Per-request credentials are opt-in and take priority when provided. Plugins remain backward-compatible: without credentials in the request, they use the default chain.

**Related Issues:**

- Links to be added based on finfocus-plugin ecosystem credentials task

---

### CE-1.7: Support FOCUS 1.4 Billing Period and Invoice Detail Columns (v0.7.0+, Post-v0.1.0)

**ID:** CE-1.7  
**Description:** Add optional support for FOCUS 1.4 cost-and-usage columns: `invoice_detail_id` (field 67) and `commitment_program_eligibility_details` (field 68) on actual cost results. AWS Cost Explorer provides line-item invoice data and commitment eligibility information; enriching actual cost responses with these fields enables downstream FinOps reporting on commitment utilization and invoice reconciliation.

**Status:** Post-v0.1.0 (scheduled for v0.2.0 after invoice RPC support is added to finfocus-spec)

**Background:**

FOCUS 1.4 adds invoice-level detail tracking. AWS Cost Explorer's `GetCostAndUsageWithResources` API (14-day resource-level limit) includes invoice line item references. Plugins can map those to `invoice_detail_id` and optionally infer commitment eligibility from pricing and RI/SP coverage.

**Open Questions (Needs Decision):**

1. **Invoice Detail Mapping:** Should the plugin extract `invoice_detail_id` directly from Cost Explorer API responses, or require a downstream invoice-lookup RPC call (spec v0.7.0+)? Current design assumes mapping from CE response metadata.

2. **Commitment Eligibility:** How should the plugin determine `commitment_program_eligibility_details` JSON? Options:
   - Use RI/SP coverage data from Cost Explorer (if available)
   - Leave as null/empty for v0.2.0, add in v0.3.0 once RI/SP coverage is mapped
   - Reference external commitment datasets (out of scope for v0.1.0)

3. **Backward Compatibility:** Should these fields be optional in responses (graceful degradation if not populated)?

**Tentative Approach (for v0.2.0):**

- Populate `invoice_detail_id` from Cost Explorer metadata when available
- Leave `commitment_program_eligibility_details` empty/null in v0.2.0, implement in v0.3.0 after RI/SP research
- Tests verify both fields are included in schema-validated responses

**Related Issues:**

- Links to be added based on FOCUS 1.4 roadmap

---

## Phase 2: Testing & Validation (CE-2.x)

### CE-2.1: Add Integration Tests for gRPC Server

**Status:** DONE, `go test -count=1 -v ./test/integration/... && golangci-lint run ./...`, exit 0; break check: forcing Supports false fails the built-process AWS support assertion.

**ID:** CE-2.1  
**Description:** Add integration tests that start the gRPC server in a subprocess and make live RPC calls. Tests should verify GetActualCost with mocked Cost Explorer responses and trace ID propagation.

**Files Modified:**

- `test/integration/gRPC_test.go` (new file)
- `internal/client/client_test.go` - Add mock Cost Explorer API client

**Acceptance Criteria:**

- Integration tests in new file `test/integration/gRPC_test.go`
- Test: Server starts on ephemeral port
- Test: GetActualCost returns results with mock data
- Test: Supports() rejects unsupported resources
- Test: Trace ID propagates through gRPC metadata to logs
- Test: Graceful shutdown on context cancellation
- Tests run with: `go test -v ./test/integration/...`
- All tests pass

**Related Issues:**

- [#45](https://github.com/rshade/finfocus-plugin-aws-ce/issues/45) - Add integration tests for gRPC server

---

### CE-2.2: Add Config Parsing Tests for main.go

**Status:** DONE, `go test -count=1 ./cmd/... && golangci-lint run ./... && markdownlint-cli2 CLAUDE.md TASKS.md`, exit 0; break check: restoring port-zero environment fallback fails explicit CLI-zero subprocess startup.

**ID:** CE-2.2  
**Description:** Add unit tests for CLI flag and environment variable parsing in `cmd/plugin/main.go`. Tests should verify port detection, log level parsing, and graceful error handling.

**Files Modified:**

- `cmd/plugin/main_test.go` (new file)
- `cmd/plugin/main.go` - Export parseLogLevel() function

**Acceptance Criteria:**

- Tests for `parseLogLevel()` with valid/invalid inputs
- Tests for port flag parsing (CLI override, env var fallback)
- Tests for context cancellation and graceful shutdown
- Test: Flag precedence (CLI flag > env var > default)
- All tests pass: `go test ./cmd/...`

**Related Issues:**

- [#51](https://github.com/rshade/finfocus-plugin-aws-ce/issues/51) - Add config parsing tests for main.go

---

### CE-2.3: Plugin Conformance Testing

**Status:** DONE, `go test -count=1 -v ./test/conformance/... && golangci-lint run ./...`, exit 0; break check: mapping missing resource to Internal fails InvalidArgument and typed `ErrorDetail` assertion over the built process.

**ID:** CE-2.3  
**Description:** Create test suite to verify plugin conformance with finfocus-spec. Tests should validate proto message structure, error handling, and protocol compliance.

**Files Modified:**

- `test/conformance/conformance_test.go` (new file)
- `test/conformance/fixtures.go` (new file)

**Acceptance Criteria:**

- Test: Proto message round-trip (marshal/unmarshal)
- Test: gRPC error codes match proto ErrorCode enum
- Test: Required fields present in responses
- Test: Optional fields handled gracefully
- Test: Request validation per spec
- Tests run with: `go test -v ./test/conformance/...`
- All tests pass

**Related Issues:**

- [#31](https://github.com/rshade/finfocus-plugin-aws-ce/issues/31) - Plugin Conformance Testing

---

## Phase 3: Build & CI/CD (CE-3.x)

### CE-3.1: Add Missing Makefile Targets

**Status:** BLOCKED-ON-INPUT, Docker image build depends on excluded CE-3.2 and an owner decision; develop, test-integration, install-local and PHONY recipes verified by `go test -count=1 ./internal/test -run TestMake`; break check: removing test from PHONY skips the recipe and fails the execution guard.

**ID:** CE-3.1  
**Description:** Add Makefile targets for development workflows: `develop`, `test-integration`, `docker`, and `install-local`.

**Files Modified:**

- `Makefile` - Add new targets

**Acceptance Criteria:**

- `make develop` - Installs dependencies and prepares dev environment
- `make test-integration` - Runs integration tests
- `make docker` - Builds Docker image
- `make install-local` - Installs binary to `~/.finfocus/plugins/aws-ce/0.1.0/`
- All targets have help text (visible in `make help`)
- `make lint` and `make test` still work

**Related Issues:**

- [#50](https://github.com/rshade/finfocus-plugin-aws-ce/issues/50) - Add missing Makefile targets (develop, test-integration, docker)

---

### CE-3.2: Add Docker Support with Multi-Stage Build

**ID:** CE-3.2  
**Description:** Create Dockerfile with multi-stage build (build stage with Go toolchain, runtime stage with minimal image). Docker image should be built as `finfocus-plugin-aws-ce:v0.1.0`.

**Files Modified:**

- `Dockerfile` (new file)
- `.dockerignore` (new file)
- `Makefile` - Add docker targets

**Acceptance Criteria:**

- Multi-stage Dockerfile (builder + runtime)
- Base image for runtime: `alpine:latest` or `gcr.io/distroless/base`
- Binary size < 50MB
- Docker build succeeds: `docker build -t finfocus-plugin-aws-ce:v0.1.0 .`
- Container runs and responds to port probe
- ENV vars passed through correctly (AWS_REGION, etc.)

**Related Issues:**

- [#42](https://github.com/rshade/finfocus-plugin-aws-ce/issues/42) - Add Docker support with multi-stage build

---

### CE-3.3: Add HTTP Health Endpoint for Container Orchestration

**Correction (2026-10-01):** the SDK already provides a health endpoint (`WebConfig.EnableHealthEndpoint`, served at `/healthz`) and a `HealthChecker` interface. Wire those: do not hand-roll `/health` and `/ready` unless the SDK cannot do what the Dockerfile needs, and say why in the report.

**ID:** CE-3.3  
**Description:** Add optional HTTP health endpoint (e.g., `/healthz`) for Kubernetes/container orchestration liveness probes. Endpoint should verify gRPC server health without requiring gRPC client.

**Files Modified:**

- `internal/pricing/calculator.go` - Add health check logic
- `cmd/plugin/main.go` - Start HTTP server alongside gRPC

**Acceptance Criteria:**

- Health endpoint at `http://127.0.0.1:8080/healthz` (configurable port)
- Returns 200 OK when gRPC server is healthy
- Returns 503 Service Unavailable when gRPC server down
- Controlled by env var `FINFOCUS_HEALTH_ENDPOINT` (default: enabled)
- Both gRPC and HTTP servers shut down gracefully together

**Related Issues:**

- [#43](https://github.com/rshade/finfocus-plugin-aws-ce/issues/43) - Add HTTP health endpoint for container orchestration

---

### CE-3.4: Standardize CI/CD Workflows

**ID:** CE-3.4  
**Description:** Add GitHub Actions workflows for test, lint, build, and release. Workflows should follow finfocus standard patterns (from aws-public plugin).

**Files Modified:**

- `.github/workflows/test.yml` (new file)
- `.github/workflows/lint.yml` (new file)
- `.github/workflows/build.yml` (new file)
- `.github/workflows/release.yml` (new file)

**Acceptance Criteria:**

- CI runs on push to main and PRs
- Test workflow: `make test` on Go 1.25+
- Lint workflow: `make lint` with golangci-lint
- Build workflow: `go build ./...` with matrix for multiple regions (if applicable)
- Release workflow: Triggered on git tag, builds binaries, creates GitHub release
- All workflows pass on main branch

**Related Issues:**

- [#48](https://github.com/rshade/finfocus-plugin-aws-ce/issues/48) - Standardize workflow names and add missing `CI/CD` workflows

---

## Phase 4: Documentation & UX (CE-4.x)

### CE-4.1: Create Documentation Directory with API & Deployment Guides

**ID:** CE-4.1  
**Description:** Add comprehensive documentation in `docs/` directory: API reference, deployment guide, configuration guide, troubleshooting.

**Files Modified:**

- `docs/API.md` (new file) - Proto API reference
- `docs/DEPLOYMENT.md` (new file) - Docker, Kubernetes, local setup
- `docs/CONFIGURATION.md` (new file) - Env vars, CLI flags, profiles
- `docs/TROUBLESHOOTING.md` (new file) - Common errors and fixes
- `README.md` - Update with links to docs

**Acceptance Criteria:**

- Docs cover: installation, configuration, usage, troubleshooting
- API reference documents all RPC methods and proto messages
- Deployment guide covers Docker, Kubernetes, local binary
- Configuration guide lists all env vars with defaults and descriptions
- README updated with quick-start link
- Docs pass markdownlint (if available)

**Related Issues:**

- [#44](https://github.com/rshade/finfocus-plugin-aws-ce/issues/44) - Create documentation directory with API and deployment guides

---

### CE-4.2: Create CONTRIBUTING.md with Development Guidelines

**ID:** CE-4.2  
**Description:** Add `CONTRIBUTING.md` with development setup, testing, and contribution workflow.

**Files Modified:**

- `CONTRIBUTING.md` (new file)
- `.github/PULL_REQUEST_TEMPLATE.md` (new file - optional)

**Acceptance Criteria:**

- Development setup: Prerequisites, `make develop` instructions
- Testing: How to run unit tests, integration tests, E2E tests
- Contribution workflow: Branch naming, commit message format, PR requirements
- Code style: Go conventions, error handling, logging
- Minimum test coverage and lint requirements
- Clear instructions for first-time contributors

**Related Issues:**

- [#53](https://github.com/rshade/finfocus-plugin-aws-ce/issues/53) - Create CONTRIBUTING.md with development guidelines

---

### CE-4.3: Polish Installation & Documentation (Out-of-the-Box Experience)

**Status:** DONE, `go test -count=1 ./... && golangci-lint run ./... && markdownlint-cli2 README.md ROADMAP.md CONTEXT.md TASKS.md docs/CREDENTIALS.md docs/QUICKSTART.md && python3 -m json.tool examples/plan.json`, exit 0; break check: removing region validation or masking its safe per-request error fails setup-action regression tests.

**ID:** CE-4.3  
**Description:** Ensure first-time user experience is smooth: clear README, quick-start guide, example commands, error messages.

**Files Modified:**

- `README.md` - Update with current status
- `docs/QUICKSTART.md` (new file) - Get started in 5 minutes
- `internal/client/client.go` - Improve error messages

**Acceptance Criteria:**

- README has "Quick Start" section
- Quick-start guide covers: install, authenticate (AWS credentials), run
- Error messages are actionable (e.g., "Missing AWS_REGION, set it with: export AWS_REGION=us-east-1")
- Binary announcement on startup is clear
- Example Pulumi plan JSON included for testing

**Related Issues:**

- [#12](https://github.com/rshade/finfocus-plugin-aws-ce/issues/12) - Polish: Installation & Documentation (Out-of-the-Box Experience)

---

## Phase 5: Core Plugin Implementation (CE-5.x)

*These tasks complete the v0.1.0 release:*

### CE-5.1: Implement Core Cost Plugin (Spec 001) & E2E Testing

**ID:** CE-5.1  
**Description:** Ensure all v0.1.0 requirements are met: Supports(), GetPluginInfo(), GetActualCost with real AWS Cost Explorer calls, error handling, trace ID propagation, and end-to-end test against finfocus core.

**Files Modified:**

- All above tasks (cumulative)
- `test/e2e/e2e_test.go` - Actual E2E test against finfocus core

**Acceptance Criteria:**

- All CE-1, CE-2, CE-3, CE-4 tasks completed
- `make build` succeeds
- `make test` - All unit tests pass
- `make test-integration` - All integration tests pass
- E2E test with finfocus core: `make test-e2e` passes (requires AWS credentials)
- Binary runs: `./bin/finfocus-plugin-aws-ce --port 50051`
- Manual test against finfocus: `finfocus cost actual --pulumi-json plan.json`

**Related Issues:**

- [#11](https://github.com/rshade/finfocus-plugin-aws-ce/issues/11) - Implement Core Cost Plugin (Spec 001) & E2E Testing

---

## Phase 6: Correctness and enrichment (CE-6.x), v0.1.0

*Found by the 2026-10-01 audit of the code and the Cost Explorer documentation. CE-6.1 runs
first, because its failures decide the order of the rest.*

### CE-6.1: Contract fixtures gate (run first)

**Status:** DONE, `go test -count=1 ./...`, all packages pass; money cases PASS via `amount_decimal`; float64 aggregation break check failed `three_pages_token_chain` then decimal summing was restored.

**ID:** CE-6.1  
**Description:** `internal/client/testdata/ce-contract/ce-contract.json` holds 26 Cost Explorer
contract cases with independently computed expectations (official samples, pagination, Estimated
flag, credits, precision, real zero, missing and unparseable amounts, empty results, mixed
currencies, UTC date ranges, resource id forms). Write the test its `README.md` specifies: a fake
HTTP Cost Explorer endpoint, a real gRPC server, every case. Add an endpoint override if the client
has none (read the SDK for `BaseEndpoint`). The fixture files are read-only.

**Acceptance Criteria:**

- results table in `.superpowers/contract-results.md` and in the report
- every `ok` case matches exactly as a decimal, every `error` case returns the stated code and never
  a zero, `request_period` cases give the same answer under three `TZ` values
- break check: change the amount parser to `float64` and watch the precision case fail
- an opt-in live test exists (skipped by default, never in CI) and is marked BLOCKED-ON-CREDENTIALS

### CE-6.2: Stop silent zeros and panics in the response parser

**Status:** DONE, `go test -count=1 ./...`, all packages pass; ParseFloat break check failed `amount_unparseable` and `precision_small_and_large`, then the decimal parser was restored.

**ID:** CE-6.2  
**Description:** `client.go:363-365` and `:398-400` drop `Sscanf` errors, so an unparseable amount
becomes 0, and the amount pointer is dereferenced without a nil check. `calculator.go:270-271` sets
`UsageAmount` to 0 and `UsageUnit` to the currency code, though `UsageQuantity` is requested.
Parse with a decimal type, return an explicit error for a missing or unparseable amount, return
mixed currencies as an error, fill `UsageAmount` and its unit from `UsageQuantity` when present.

**Acceptance Criteria:** the `amount_missing`, `amount_unparseable`, `mixed_currencies`,
`precision_small_and_large` and `real_zero` contract cases pass.

### CE-6.3: Dates, time zones and the exclusive end

**Status:** DONE, `go test -count=1 ./...`, all packages pass including TestCEContractFixtures; time.Local start format break check failed Pacific/Auckland (`period:2026-09-01..2026-09-02:2` sent 2026-09-02), then UTC was restored.

**ID:** CE-6.3  
**Description:** `calculator.go:119-120` and `client.go:239-240` format dates in the host's local
time zone and ignore that the Cost Explorer `End` is exclusive. Use UTC, apply the policy in the
contract README (partial last day rounds up, `start >= end` is `InvalidArgument`), and clamp or
reject by the real limit for the query kind (14 months for totals, 14 days for resources).

**Acceptance Criteria:** all `request_period` cases pass under `TZ=UTC`,
`TZ=America/Los_Angeles` and `TZ=Pacific/Auckland`.

### CE-6.4: RI and Savings Plan data in the FOCUS record (#37)

**Status:** DONE, `go test -count=1 ./...`, all packages pass including TestCEContractFixtures (pricing 0.195s); break check selected UnblendedCost for the 2.50/3.00 reservation row, TestCommitmentLineMapsToFocusRecord failed (`amount_decimal "3", want 2.50`), then AmortizedCost was restored.

**ID:** CE-6.4  
**Description:** Fill the `FocusCostRecord` commitment fields (`commitment_discount_category`,
`_id`, `_name`, `_status`, `_type`, `_quantity`, `_unit`) that the spec already carries. Choose the
metric deliberately: `UnblendedCost`, or `AmortizedCost` when commitments exist; never `BlendedCost`
per resource. Say in the code and the docs which metric each field uses. The existing client methods
for reservation and Savings Plans data (`client.go:423-462`) are not wired and have no pagination:
wire them with pagination or remove them. This task supersedes duplicates #19, #20 and #28.

**Acceptance Criteria:** a fixture with a commitment line maps to a record that passes
`ValidateFocusRecord` (read the real signature in the spec first); a break check changes the metric
and fails a test; no commitment data returns the fields unset, not zero.

**Related Issues:** [#37](https://github.com/rshade/finfocus-plugin-aws-ce/issues/37),
[#19](https://github.com/rshade/finfocus-plugin-aws-ce/issues/19),
[#20](https://github.com/rshade/finfocus-plugin-aws-ce/issues/20),
[#28](https://github.com/rshade/finfocus-plugin-aws-ce/issues/28)

### CE-6.5: Metadata enrichment through the right field (#52)

**Status:** DONE, `go test -count=1 ./... && golangci-lint run ./... && markdownlint-cli2 README.md TASKS.md`, exit 0; break check: forced estimated=false; both TestActualCostMetadata `lookback` cases failed, restored.

**ID:** CE-6.5  
**Description:** The issue's code assumes a `metadata` map on `ActualCostResult`, which does not
exist (fields 1 to 9 only). Carry data source, granularity, metric, `Estimated` and lookback through
`FocusCostRecord.extended_columns`, or `lineage` where the spec defines it. If a response-level
map is wanted, that is NOT a spec change today: no other plugin needed one, and currency already reaches the core through `focus_record.billing_currency`. Use the FOCUS record and `extended_columns`, document the keys, and list "no response-level metadata map" in the register. Do not file a spec issue.

**Acceptance Criteria:** an `Estimated=true` fixture produces an extended column saying so; a test
reads the real proto field; no field is invented.

**Related Issues:** [#52](https://github.com/rshade/finfocus-plugin-aws-ce/issues/52)

### CE-6.6: Batch configuration and a race-free client (#55)

**Status:** DONE, `go test -count=1 -race -run "TestConcurrentBatchClientInitialization|TestClientInitializationFailureCooldown" ./internal/pricing && go test -count=1 ./... && golangci-lint run ./... && markdownlint-cli2 README.md CLAUDE.md TASKS.md`, exit 0; break check: removed init lock; concurrent BatchCost initialized 10 clients and race detector failed, restored; full race suite also passed.

**ID:** CE-6.6  
**Description:** Use `ServeConfig.MaxBatchSize` and `BatchWorkers` (the SDK serves a per-resource
`BatchCost` fallback). The issue's `GetActualCostBatch` RPC does not exist: do not add it. The env
var names are a plugin choice: document them. `initClient` (`calculator.go:61-74`) mutates
`c.ceClient` without a lock and retries a failed init, with a credential-chain load, on every
request: guard it, and cache the failure for a short time.

**Acceptance Criteria:** `go test -race` passes a concurrent `BatchCost` test; invalid batch
settings fail at start with a clear message.

**Related Issues:** [#55](https://github.com/rshade/finfocus-plugin-aws-ce/issues/55)

### CE-6.7: Per-resource cost the way Cost Explorer really does it

**Status:** DONE, `go test -count=1 ./...`, all packages pass including TestCEContractFixtures (pricing 0.191s); break check sent bare i-0abc123def4567890 to GetCostAndUsage, TestPerResourceCostClassification/bare_i-0abc123def4567890 failed, then GetCostAndUsageWithResources was restored.

**ID:** CE-6.7  
**Description:** Per-resource cost needs `GetCostAndUsageWithResources` and the `RESOURCE_ID`
dimension, not `GetCostAndUsage`. Switch resource-level requests to it, map an ARN to the resource
id form Cost Explorer uses (the instance id for EC2: verify per service, mark the rest Unverified),
return an explicit, documented error where resource-level cost is not available, and tell users
about the opt-in and the 14-day window in the docs and in `Supports`. Honour `tags` in the request
(`GetActualCostRequest.tags`) or document that they are ignored. Rows labelled "No resource ID" do
not sum to a service total: say so in the notes.

**Acceptance Criteria:** the `resource_id:*` contract cases pass or are recorded as named findings;
no request sends a full ARN as an EC2 `RESOURCE_ID`.

### CE-6.8: Cache that is safe, honest and cheap

**Status:** DONE, `go test -count=1 ./...`, all packages pass including TestCEContractFixtures (pricing 0.210s); break check wrote the raw key as the cache file name, TestCacheKeyStaysInDirectory failed because rel was `../../x:arn:aws:ec2:us-east-1:123456789012:instance:i-0abc.json`, then the SHA-256 file name was restored.

**ID:** CE-6.8  
**Description:** `calculator.go:136` and `cache.go:87` use the request key as a file name: an ARN
puts `/` and `:` in it and a `..` segment could leave the cache directory. Hash the key, include
every parameter that changes the answer (granularity, group by, metric, tags), set `expires_at` on
the response, and use a short TTL when the range includes recent days or any day is `Estimated`
(a fixed 24 hours serves revised data stale). Count requests to Cost Explorer (each is $0.01),
log the count, and offer a configurable per-minute limit.

**Acceptance Criteria:** a key containing `../../x` stays inside the cache directory; the Estimated
case gets the short TTL; a repeat request does not call the fake endpoint again; the request
counter is tested.

### CE-6.9: AWS errors become real gRPC codes

**Status:** DONE, `go test -count=1 ./...`, all packages pass including TestCEContractFixtures (pricing 0.199s); break check: LimitExceededException mapped to codes.Internal, TestGetActualCost_AWSErrorCodes/LimitExceededException failed (status.Code = Internal, want ResourceExhausted), then ResourceExhausted was restored.

**ID:** CE-6.9  
**Description:** Plain `fmt.Errorf` becomes `Unknown`. Map `LimitExceededException` to
`ResourceExhausted`, access denied to `PermissionDenied`, missing or expired credentials to
`Unauthenticated` with the spec's `ErrorCode`, bad parameters to `InvalidArgument`, and "no data" to
the spec's NO_COST_DATA style error. This extends CE-1.5.

**Acceptance Criteria:** a table-driven test per AWS error class through a real gRPC server.

### CE-6.10: Capabilities, dead code and test debt

**Status:** DONE, `go test -count=1 ./... && golangci-lint run ./... && markdownlint-cli2 README.md ROADMAP.md TASKS.md`, exit 0; break check: plain projected error, disabled cache read and disabled uppercase gate each failed regression tests, restored; enabled fake E2E passed.

**ID:** CE-6.10  
**Description:** `GetProjectedCost` returns a plain error after a `Supports` check: return
`codes.Unimplemented` and declare the actual-cost capability in `GetPluginInfo`. Remove the unused
types in `data.go`. The E2E gate variable is the lowercase `finfocus_E2E`: rename it with a
fallback and document it. `calculator_test.go` covers only ARN handling: add `GetActualCost` tests
through the fake endpoint for the cache, error paths and pagination. Declare the real-time
dependency: Cost Explorer data lags 24 hours or more, which the docs must say.

**Acceptance Criteria:** the listed items are done and each has a test; no TODO is added.

### CE-6.11: Document the AWS per-request credential keys

**Status:** DONE, `go test -count=1 ./... && golangci-lint run ./... && markdownlint-cli2 README.md docs/CREDENTIALS.md TASKS.md`, exit 0; break check: raw client error logging and disabled IAM-role validation each failed redaction/shape tests, restored.

**ID:** CE-6.11  
**Description:** CE-1.6 already uses the keys `access_key_id`, `secret_access_key`, `session_token` and
`role_arn`. Document them (README and `docs/`), validate them, and test that none is ever logged. Credential
names are free-form in the spec by design and every other plugin uses its own convention, so this is plugin
documentation, not a spec change: do not file a spec issue. Write a short docs-only draft in
`.superpowers/issue-drafts/spec-docs-credentials.md` (an optional row in the SDK README listing the names
plugins use) and list its path in the report. The owner files it.

**Acceptance Criteria:** the keys are documented and tested; the draft path is in the report.

---

## Phase 7: Second run, Tier B (CE-7.x)

*Not part of v0.1.0. Each task needs a live AWS account to verify against real Cost Explorer, so the
live part is BLOCKED-ON-CREDENTIALS; everything else is tested with the fake endpoint and fixtures.
All carriers below exist in finfocus-spec v0.7.0 unless stated.*

### CE-7.1: AWS Budgets

**Issues:** [#8](https://github.com/rshade/finfocus-plugin-aws-ce/issues/8), duplicate
[#24](https://github.com/rshade/finfocus-plugin-aws-ce/issues/24). **Carrier:** `GetBudgets`,
`PLUGIN_CAPABILITY_BUDGETS`. Use the Budgets API (`budgets:ViewBudget`, a different endpoint and
IAM action from Cost Explorer). **Acceptance:** fake-endpoint tests for pagination and empty lists.

### CE-7.2: Recommendations umbrella, with the three slices

**Issues:** [#13](https://github.com/rshade/finfocus-plugin-aws-ce/issues/13), duplicate
[#22](https://github.com/rshade/finfocus-plugin-aws-ce/issues/22). **Carrier:** `GetRecommendations`.
One shared recommendation mapper, then CE-7.3, CE-7.4, CE-7.5. A scorer plugin may consume the
result: never include account ids or resource names beyond what the core's identifier mode allows.

### CE-7.3: Rightsizing recommendations

**Issue:** [#27](https://github.com/rshade/finfocus-plugin-aws-ce/issues/27).
`GetRightsizingRecommendation` into `RECOMMENDATION_ACTION_TYPE_RIGHTSIZE`.

### CE-7.4: Savings Plans purchase recommendations

**Issue:** [#32](https://github.com/rshade/finfocus-plugin-aws-ce/issues/32).
`GetSavingsPlansPurchaseRecommendation` into `PURCHASE_COMMITMENT`.

### CE-7.5: Reserved Instance purchase recommendations

**Issue:** [#33](https://github.com/rshade/finfocus-plugin-aws-ce/issues/33).
`GetReservationPurchaseRecommendation` into `PURCHASE_COMMITMENT`.

### CE-7.6: Blended, unblended and effective cost

**Issue:** [#21](https://github.com/rshade/finfocus-plugin-aws-ce/issues/21). The actual side maps
to the `FocusCostRecord` `billed_cost`, `list_cost`, `effective_cost` and `contracted_cost`. The
projected-side `effective_*` fields the issue proposes do not exist in the spec and do not belong
in an actual-cost plugin: record that decision, and file a spec issue only if the owner wants it.

### CE-7.7: Greenops research

**Issue:** [#29](https://github.com/rshade/finfocus-plugin-aws-ce/issues/29). A findings document.
The issue itself says there is no public AWS API: confirm from AWS documentation, with sources,
and recommend a data source or close the question.

### CE-7.8: EstimateCost what-if

**Issue:** [#30](https://github.com/rshade/finfocus-plugin-aws-ce/issues/30). Overlaps
`finfocus-plugin-aws-public`, which already prices projected cost and which this plugin's own
`GetProjectedCost` message points users to. Decide and document: implement through the Pricing API,
or close as out of role.

### CE-7.9: Spot market advisor research

**Issue:** [#34](https://github.com/rshade/finfocus-plugin-aws-ce/issues/34). Outside Cost
Explorer. A findings document on where spot price and interruption data can come from, and which
plugin should own it.

### CE-7.10: Forecasting with prediction intervals

**Issues:** [#36](https://github.com/rshade/finfocus-plugin-aws-ce/issues/36), duplicate
[#25](https://github.com/rshade/finfocus-plugin-aws-ce/issues/25). `GetCostForecast` into
`GetProjectedCostResponse.prediction_interval_lower`, `_upper` and `confidence_level`. The client
method exists (`client.go:117`). The blocking spec issue (#314) is closed.

### CE-7.11: Anomaly detection through GetRecommendations

**Issues:** [#38](https://github.com/rshade/finfocus-plugin-aws-ce/issues/38), duplicate
[#26](https://github.com/rshade/finfocus-plugin-aws-ce/issues/26) (which proposes a new `GetAnomalies`
RPC: do not add one). `RECOMMENDATION_CATEGORY_ANOMALY` with `ACTION_TYPE_INVESTIGATE`.

### CE-7.12: Web and Connect protocol

**Issue:** [#49](https://github.com/rshade/finfocus-plugin-aws-ce/issues/49). Enable
`ServeConfig.Web` through environment configuration. The SDK does the work.

### CE-7.13: CORS

**Issue:** [#54](https://github.com/rshade/finfocus-plugin-aws-ce/issues/54). Set
`WebConfig.AllowedOrigins`, `AllowCredentials` and `AllowedHeaders` from configuration. The SDK
validates them. Never default to a wildcard with credentials.

---

## Issue Index (All 40 Open Issues)

| # | Title | Labels | Disposition | Task ID |
|---|-------|--------|-------------|---------|
| [#3](https://github.com/rshade/finfocus-plugin-aws-ce/issues/3) | refactor: Adopt pluginsdk/mapping for property extraction | roadmap/future | stale: close or rewrite | — |
| [#8](https://github.com/rshade/finfocus-plugin-aws-ce/issues/8) | Feature: AWS Budgets Support | roadmap/next | Tier B | CE-7.1 |
| [#11](https://github.com/rshade/finfocus-plugin-aws-ce/issues/11) | Implement Core Cost Plugin (Spec 001) & E2E Testing | roadmap/current | **CE-5.1** | ✅ Umbrella |
| [#12](https://github.com/rshade/finfocus-plugin-aws-ce/issues/12) | Polish: Installation & Documentation | roadmap/current | **CE-4.3** | ✅ Umbrella |
| [#13](https://github.com/rshade/finfocus-plugin-aws-ce/issues/13) | Feature: Optimization Recommendations | roadmap/future | Tier B | CE-7.2 |
| [#19](https://github.com/rshade/finfocus-plugin-aws-ce/issues/19) | Feature: Reserved Instance Coverage Detection & Pricing | enhancement, roadmap/future | duplicate of #37 | CE-6.4 |
| [#20](https://github.com/rshade/finfocus-plugin-aws-ce/issues/20) | Feature: Savings Plans Coverage Detection & Pricing | enhancement, roadmap/future | duplicate of #37 | CE-6.4 |
| [#21](https://github.com/rshade/finfocus-plugin-aws-ce/issues/21) | Feature: Blended vs Unblended Cost Comparison | enhancement, roadmap/future | Tier B | CE-7.6 |
| [#22](https://github.com/rshade/finfocus-plugin-aws-ce/issues/22) | Feature: RI/Savings Plan Purchase Recommendations | enhancement, roadmap/future | duplicate of #13 | CE-7.2 |
| [#23](https://github.com/rshade/finfocus-plugin-aws-ce/issues/23) | chore(deps): Update finfocus-spec to enable gRPC reflection | roadmap/current | stale: satisfied by spec v0.7.0 | CE-1.1 |
| [#24](https://github.com/rshade/finfocus-plugin-aws-ce/issues/24) | Feature: AWS Budgets Support | roadmap/next | duplicate of #8 | CE-7.1 |
| [#25](https://github.com/rshade/finfocus-plugin-aws-ce/issues/25) | Feature: Cost Forecasting | roadmap/next | duplicate of #36 | CE-7.10 |
| [#26](https://github.com/rshade/finfocus-plugin-aws-ce/issues/26) | Feature: Anomaly Detection | roadmap/next | duplicate of #38 | CE-7.11 |
| [#27](https://github.com/rshade/finfocus-plugin-aws-ce/issues/27) | Feature: Rightsizing Recommendations | roadmap/future | Tier B | CE-7.3 |
| [#28](https://github.com/rshade/finfocus-plugin-aws-ce/issues/28) | Chore: FOCUS 1.3 Transition | roadmap/future | duplicate of #37, stale (FOCUS 1.3) | CE-6.4 |
| [#29](https://github.com/rshade/finfocus-plugin-aws-ce/issues/29) | Research: Greenops Discovery | roadmap/future | Tier B | CE-7.7 |
| [#30](https://github.com/rshade/finfocus-plugin-aws-ce/issues/30) | Feature: EstimateCost (What-If) | roadmap/future | Tier B | CE-7.8 |
| [#31](https://github.com/rshade/finfocus-plugin-aws-ce/issues/31) | Chore: Plugin Conformance Testing | roadmap/current | **CE-2.3** | ✅ In scope |
| [#32](https://github.com/rshade/finfocus-plugin-aws-ce/issues/32) | Feature: Savings Plans Recommendations | roadmap/future | Tier B | CE-7.4 |
| [#33](https://github.com/rshade/finfocus-plugin-aws-ce/issues/33) | Feature: Reserved Instance Recommendations | roadmap/future | Tier B | CE-7.5 |
| [#34](https://github.com/rshade/finfocus-plugin-aws-ce/issues/34) | Research: Spot Market Advisor | roadmap/future | Tier B | CE-7.9 |
| [#36](https://github.com/rshade/finfocus-plugin-aws-ce/issues/36) | feat: Implement GetProjectedCost with prediction intervals | enhancement, roadmap/next | Tier B | CE-7.10 |
| [#37](https://github.com/rshade/finfocus-plugin-aws-ce/issues/37) | feat: Enrich GetActualCost with RI/SP commitment discount data | enhancement, roadmap/next | **v0.1.0, Tier A** | **CE-6.4** |
| [#38](https://github.com/rshade/finfocus-plugin-aws-ce/issues/38) | feat: Implement cost anomaly detection via GetRecommendations | enhancement, roadmap/next | Tier B | CE-7.11 |
| [#40](https://github.com/rshade/finfocus-plugin-aws-ce/issues/40) | feat: Add GetPluginInfo() RPC implementation | enhancement, roadmap/current | **CE-1.3** | ✅ In scope |
| [#41](https://github.com/rshade/finfocus-plugin-aws-ce/issues/41) | feat: Add Supports() RPC implementation | enhancement, roadmap/current | **CE-1.2** | ✅ In scope |
| [#42](https://github.com/rshade/finfocus-plugin-aws-ce/issues/42) | feat: Add Docker support with multi-stage build | enhancement, roadmap/current | **CE-3.2** | Deferred in run 2; PM |
| [#43](https://github.com/rshade/finfocus-plugin-aws-ce/issues/43) | feat: Add HTTP health endpoint for container orchestration | enhancement, roadmap/current | **CE-3.3** | Deferred in run 2; PM |
| [#44](https://github.com/rshade/finfocus-plugin-aws-ce/issues/44) | docs: Create documentation directory with API and deployment guides | documentation, roadmap/current | **CE-4.1** | Deferred in run 2; PM |
| [#45](https://github.com/rshade/finfocus-plugin-aws-ce/issues/45) | test: Add integration tests for gRPC server | enhancement, roadmap/current | **CE-2.1** | ✅ In scope |
| [#46](https://github.com/rshade/finfocus-plugin-aws-ce/issues/46) | feat: Implement trace ID propagation for distributed tracing | enhancement, roadmap/current | **CE-1.4** | ✅ In scope |
| [#47](https://github.com/rshade/finfocus-plugin-aws-ce/issues/47) | feat: Use proto ErrorCode enum for standardized error handling | enhancement, roadmap/current | **CE-1.5** | ✅ In scope |
| [#48](https://github.com/rshade/finfocus-plugin-aws-ce/issues/48) | ci: Standardize workflow names and add missing `CI/CD` workflows | enhancement, roadmap/current | **CE-3.4** | Deferred in run 2; PM |
| [#49](https://github.com/rshade/finfocus-plugin-aws-ce/issues/49) | feat: Add Web/Connect protocol support for browser clients | enhancement, roadmap/next | Tier B | CE-7.12 |
| [#50](https://github.com/rshade/finfocus-plugin-aws-ce/issues/50) | chore: Add missing Makefile targets | enhancement, roadmap/current | **CE-3.1** | ✅ In scope |
| [#51](https://github.com/rshade/finfocus-plugin-aws-ce/issues/51) | test: Add config parsing tests for main.go | enhancement, roadmap/next | **CE-2.2** | ✅ In scope |
| [#52](https://github.com/rshade/finfocus-plugin-aws-ce/issues/52) | feat: Add metadata enrichment to cost responses | enhancement, roadmap/next | **v0.1.0, Tier A** | **CE-6.5** |
| [#53](https://github.com/rshade/finfocus-plugin-aws-ce/issues/53) | docs: Create CONTRIBUTING.md with development guidelines | documentation, roadmap/current | **CE-4.2** | Deferred in run 2; PM |
| [#54](https://github.com/rshade/finfocus-plugin-aws-ce/issues/54) | feat: Add CORS support for browser-based clients | enhancement, roadmap/future | Tier B | CE-7.13 |
| [#55](https://github.com/rshade/finfocus-plugin-aws-ce/issues/55) | feat: Add batch configuration options | enhancement, roadmap/future | **v0.1.0, Tier A** | **CE-6.6** |

**Summary:** 40 open issues (8 closed, 48 total): 18 in Tier A for v0.1.0 (CE-1 to CE-6), 13 in Tier B for a second run (CE-7), 7 duplicates and 2 stale, marked above. This plan makes no GitHub change: the owner closes or merges the duplicates and stale issues.

---

## Toolchain Completion Summary (2026-09-30)

### What Was Completed

**Ecosystem Baseline Rollout** — All infrastructure and tooling for plugin development:

1. **Language & Dependencies**
   - ✅ Go upgraded from 1.25.5 → 1.27.1 (go.mod, mise.toml, CI workflows)
   - ✅ finfocus-spec upgraded from v0.5.2 → v0.7.0 (go.mod, go get)
   - ✅ go.mod tidy, go.sum verified, vendor directory removed

2. **Local Development Environment (mise.toml)**
   - ✅ All required tools pinned to versions matching ecosystem baseline
   - ✅ golangci-lint 2.14.0, govulncheck 1.8.0, goreleaser 2.18.2
   - ✅ Node 24 + npm dependencies (markdownlint, commitlint, reviewdog)
   - ✅ Vale 3.20.0, actionlint 1.7.12

3. **Build & Release Configuration**
   - ✅ .goreleaser.yaml: project_name set, asset naming fixed for installer compatibility
   - ✅ Internal test added to validate GoReleaser asset names
   - ✅ Makefile targets available: lint, test, build, release-check

4. **GitHub Actions Workflows**
   - ✅ test.yml: go@1.27.1, golangci-lint, govulncheck (non-blocking), go test
   - ✅ release.yml: updated to use mise-action with explicit tool pins
   - ✅ commitlint.yml: created for conventional commit validation
   - ✅ prose-lint.yml: created for Vale + markdownlint via reviewdog

5. **Code Quality Configuration**
   - ✅ .golangci-lint.yml: existing configuration preserved (no edits)
   - ✅ .markdownlint.json: created with Google style, 120-char line length
   - ✅ .vale.ini: created for prose linting
   - ✅ renovate.json: created for dependency automation

6. **Naming Migration (legacy → FinFocus)**
   - ✅ go.mod module name: finfocus-plugin-aws-ce
   - ✅ All tracked files: 0 remaining legacy references
   - ✅ README.md, manifest files, internal package comments, docs, specs, tests
   - ⚠️ Untracked specs/004-core-cost-e2e/: 4 files retain legacy references (not edited per safety rules)

### Test Results Post-Toolchain

- ✅ `go build ./...` — Pass
- ✅ `go vet ./...` — Pass
- ✅ `go test -count=1 ./...` — All 4 packages pass (5ms total)
- ✅ `actionlint .github/workflows/*.yml` — All 4 workflows valid
- ✅ Repo state: clean working tree, no uncommitted changes

### What Happens Next

The toolchain baseline is now in place. Remaining work:

1. **Feature Implementation (CE-1.x through CE-5.x)**
   - Spec conformance: Supports(), GetPluginInfo(), trace ID, error codes
   - Per-request credentials (new in v0.7.0)
   - FOCUS 1.4 billing columns (post-v0.1.0)

2. **Testing & Documentation (CE-2.x, CE-4.x)**
   - Integration tests, conformance tests, E2E tests
   - API reference, deployment guides, CONTRIBUTING.md

3. **Build & Release (CE-3.x)**
   - Makefile targets, Docker support, CI/CD optimization
   - v0.1.0 release with GitHub Actions automation

### Files Modified in This Session

**Created:**

- `.specify/memory/constitution.md` (updated)
- `mise.toml` (created, then refined to baseline)
- `.markdownlint.json` (created)
- `.vale.ini` (created)
- `renovate.json` (created)
- `.github/workflows/commitlint.yml` (created)
- `.github/workflows/prose-lint.yml` (created)

**Updated:**

- `go.mod`: Go 1.27.1, finfocus-spec v0.7.0
- `Makefile`: lint-md, vuln, release-check targets added
- `.goreleaser.yaml`: project_name, name_template, asset naming fixed
- `.github/workflows/test.yml`: mise-action, tool versions, govulncheck
- `.github/workflows/release.yml`: mise-action, goreleaser pins
- `README.md`: Go 1.27.1, FinFocus references
- `manifest.yaml`, `manifest.json`: FinFocus descriptions
- All docs/specs in `specs/001-*`, `specs/002-*`, `specs/003-*` — naming migration
- `test/e2e/e2e_test.go` — naming migration
- `internal/pricing/calculator.go` — naming migration, logger component
- `TASKS.md`: toolchain status documented

---

## Open Questions

1. **AWS Credentials for Testing:** E2E tests require live AWS credentials (e.g., `AWS_ACCESS_KEY_ID`, `AWS_SECRET_ACCESS_KEY`). Should e2e_test.go be marked with `BLOCKED-ON-CREDENTIALS`?

2. **Docker Registry:** Where should Docker images be published? Docker Hub (`rshade/finfocus-plugin-aws-ce`), GitHub Container Registry (`ghcr.io/rshade/finfocus-plugin-aws-ce`), or ECR?

3. **Release Strategy:** Should v0.1.0 be released as a single binary (one region) or multi-region binaries like aws-public? aws-ce uses dynamic region from config, so single binary should suffice.

4. **Cost Explorer API Costs:** AWS charges ~$0.01 per GetCostAndUsage API call. Should client-side rate limiting or request deduplication be implemented in v0.1.0, or defer to v0.2+?

5. **Multi-Account Support:** Does v0.1.0 need to support cross-account Cost Explorer queries, or focus on single-account setup first?

---

## Verification Commands (Run Before Release)

```bash
# Build
make build

# Test
make test
make test-integration
make test-e2e  # Requires AWS credentials and PULUMI_CONFIG_PASSPHRASE set

# Lint
make lint

# Docker
docker build -t finfocus-plugin-aws-ce:v0.1.0 .

# Manual smoke test
./bin/finfocus-plugin-aws-ce --port 50051 &
sleep 1
grpcurl -plaintext localhost:50051 list
pkill finfocus-plugin-aws-ce
```

---

## Notes

- **Spec Migration:** Upgrading finfocus-spec must happen early (CE-1.1) to unblock later features
- **Testing First:** Integration tests (CE-2.1) should validate all RPC methods before CI/CD setup
- **Docker Build:** Deferred CE-3.2 owner decision; the release publishes archives only.
- **No PR Blocking:** Tasks should be implemented phase-by-phase without PR approval gates
- **E2E with Core:** Live/core verification is outside this run and blocked on credentials.

## Phase 8: Release readiness (REL-x)

### REL-1: Release Please configuration and manifest

**Status:** DONE, `go test -count=1 ./internal/test`, exit 0; break check: 15 config and manifest mutations each failed TestReleasePleaseConfiguration, restored.

**ID:** REL-1  
**Description:** Match `aws-public` settings, plain tags, initial version 0.1.0 and patch pre-major bumps. Manifest starts at 0.0.0; guard accepts bootstrap and future stable versions at or above 0.1.0. Ignore generated `CHANGELOG.md`.

**Acceptance Criteria:** See superpowers run requirements; regression guard, deliberate break check and verification pass.

### REL-2: Family release workflows

**Status:** DONE, `go test -count=1 ./internal/test && actionlint .github/workflows/*.yml && markdownlint-cli2 CLAUDE.md TASKS.md`, exit 0; break check: docker action in `yml` and `yaml`, tag push, removed flag and wrong organization each failed guard, restored.

**ID:** REL-2  
**Description:** Copy `aws-public` Release Please and `opencost` single-binary release workflow, verify action tags, guard banned publishing paths, document the release token.

**Acceptance Criteria:** See superpowers run requirements; regression guard, deliberate break check and verification pass.

### REL-3: `goreleaser` archive configuration

**Status:** DONE, `go test -count=1 ./internal/test && goreleaser check`, exit 0; break check: 10 banned-section, deprecated-format, naming, template and Windows format mutations failed guards, restored; snapshot built six archives and checksums.

**ID:** REL-3  
**Description:** Use formats, installer-compatible names, archive and checksum assets only; validate and build snapshots for Linux, Darwin and Windows.

**Acceptance Criteria:** See superpowers run requirements; regression guard, deliberate break check and verification pass.

### REL-4: Spec v0.7.5 and resource descriptor compatibility

**Status:** DONE, `go test -count=1 ./... && go build ./... && go vet ./... && markdownlint-cli2 README.md ROADMAP.md CONTEXT.md TASKS.md`, exit 0; break check: removed legacy fallback; TestActualCostResourceDescriptor legacy and empty-identity cases failed, restored.

**ID:** REL-4  
**Description:** Upgrade spec and tidy; prefer resource descriptor identity with legacy `resource_id` fallback; preserve valid YAML and JSON manifests as Release Please extra-files.

**Acceptance Criteria:** See superpowers run requirements; regression guard, deliberate break check and verification pass.

### CE-R.2: Correct Supports capability inference (CE-6.10)

**Status:** DONE, `go test -count=1 ./internal/pricing && golangci-lint run ./... && markdownlint-cli2 TASKS.md`, exit 0; break check: removing explicit Supports enum reproduces four inferred wire capabilities and fails actual-only guard.

**ID:** CE-R.2
**Description:** Explicitly advertise actual costs in typed and legacy Supports
capabilities, consistent with GetPluginInfo. Guard against SDK inference of
unimplemented projected, pricing, and estimate RPCs.

**Acceptance Criteria:**

- Both discovery RPCs advertise only actual costs over real gRPC.
- Supported and unsupported resource responses preserve their reason/decision.
