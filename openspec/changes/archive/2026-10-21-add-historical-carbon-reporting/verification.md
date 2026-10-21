# Verification and release coordination

## Implemented behavior

Historical carbon reporting is off by default. GetPluginInfo.settings always
advertises the BOOL carbon_enabled descriptor, environment mapping
FINFOCUS_AWS_CARBON_ENABLED, and false default. Enabled discovery adds the
distinct historical capability and report RPC without AWS calls.

The reader uses the official Sustainability SDK. It retains monthly
account/region/service identity, method, scope, AWS model, retrieval time and
explicit quantity presence. Protected cursors bind pagination to the normalized
query and credential context with expiration, repeated-token detection and a
100-page cap. Monetary requests remain independent.

Core provides the dedicated report command, read-only MCP tool and authenticated
web query hook. They share atomic query/aggregation logic. No available data
has a null total; an explicitly returned zero retains numeric presence.

## Initial implementation verification

All AWS calls in these checks used loopback fixtures and synthetic credentials.
No live AWS verification, resource mutation or publication was performed.

| Repository/check | Result |
| --- | --- |
| SDK make validate | Pass: full Go suite, lint, buf, Markdown/YAML, schema/examples |
| SDK TypeScript workspaces | Pass: tests, build, generated output reproducibility |
| Plugin complete client suite and Carbon race tests | Pass |
| Plugin handler/integration Carbon race tests | Pass |
| Plugin final Carbon tests across client/pricing/entry point/integration | Pass |
| Plugin make lint and make build | Pass |
| Plugin make test and make conformance with feature SDK | Four pre-existing GetPricingSpec conformance failures; all other packages pass |
| Core complete go test ./... in extended feature workspace | Pass, with companion process integration explicitly enabled |
| Core focused report/pluginhost/web/CLI race checks | Pass, including actual core and plugin binaries |
| Core make build validate docs-lint | Pass |
| Core complete make lint with explicit golangci-lint 2.15.0 override | Pass, including nested plugins and boundaries |
| Core pinned golangci-lint 2.13.2 | Fails on the existing exhaustive mapliteral configuration |

Real-process coverage independently sums two pages across two accounts, two
services and two months to 4500000 gCO2e. The pages include two AWS model
versions and an explicit zero. Tests verify both environment opt-in and
configuration-only opt-in. In configuration-only mode the plugin starts with
the variable unset, advertises its descriptor, then restarts with true. Later
page failure leaves CLI stdout empty. Disabled reports and ordinary monetary
requests do not invoke Sustainability. MCP schema discovery does not launch the
plugin or call AWS.

Independent task reviews approved SDK, setting/discovery, reader, protected
handler and core integration. Review found and corrected optional quantity
presence and preservation of sanitized AWS authentication codes. Final review
also identified inherited JSON mode, web error classification and ambient
credential cursor binding gaps. The core fixes have independent approval:
actual MCP invocation now returns JSON without a local output flag, and web
responses retain safe, distinct authentication, permission, throttling,
disabled-support and provider-data error categories. Focused race/process
tests, compatible complete lint, build, validation and documentation checks
passed after those fixes.

The credential fix snapshots resolved default-chain base credentials once per
page and binds continuations to those keys and the target role. The same
credentials are supplied to the report client, preventing a resolution/signing
race. Rotation requires restarting the report. Invalid signatures, queries and
expired cursors fail before credential acquisition; caller and expiry are
checked again afterward. Offline shared-file and role-only rotation fixtures,
loopback signing, credential-process no-access and sanitization checks pass.
The complete client/pricing packages, focused carbon/monetary race integration,
plugin lint and build also pass after this fix.

Scoped review additionally caught classification loss during credential
acquisition. The existing sanitized AWS error mapping now preserves wrapped
permission, authentication and throttling codes there too. Focused client and
pricing carbon tests, lint and build pass after that correction. The final
cross-repository process/MCP fixture also passes with the credential-binding
patch. No checks required live AWS access.

Final independent implementation review approved SDK 778f5f7, core fa7b5945
and plugin e09d591. All original findings and the acquisition-classification
follow-up are resolved. Release prerequisites below remain separate from that
implementation approval.

## Exact development commands

The external workspace is intentionally outside version control. The extended
workspace additionally includes core's nested kubernetes, scorer, prometheus
and carbon modules for its complete lint/test targets.

```sh
# SDK checkout, standalone validation.
GOWORK=off make validate

# Plugin checkout, external three-repository workspace.
GOWORK=/tmp/finfocus-historical-carbon-build/go.work \
  go test -count=1 ./internal/client ./internal/pricing ./cmd/plugin ./test/integration -run TestCarbon
GOWORK=/tmp/finfocus-historical-carbon-build/go.work make lint build

# Core checkout, companion source explicitly enables the offline process test.
FINFOCUS_CARBON_PLUGIN_SOURCE=/home/rshade/.codex/worktrees/ad6f/finfocus-plugin-aws-ce \
GOWORK=/tmp/finfocus-historical-carbon-build/lint.go.work go test ./...

FINFOCUS_CARBON_PLUGIN_SOURCE=/home/rshade/.codex/worktrees/ad6f/finfocus-plugin-aws-ce \
GOWORK=/tmp/finfocus-historical-carbon-build/lint.go.work \
  go test -race ./internal/carbonreport ./internal/pluginhost ./internal/webui ./internal/cli \
  -run 'TestQuery|TestSelectors|TestReportJSON|TestCarbon|TestHistoricalCarbon|TestWebCarbon'

GOWORK=/tmp/finfocus-historical-carbon-build/lint.go.work \
  make build validate docs-lint
```

The compatible complete core lint invocation overrides both GOLANGCI_LINT and
GOLANGCI_LINT_VERSION=2.15.0. The tracked repository pin remains unchanged.

## Baselines before rebase

The initial local SDK baseline lacked the capability-aware conformance revision
in the plugin's pinned dependency. Unchanged plugin main with that feature SDK
reproduced four failures: three Spec Validation/GetPricingSpec and one RPC
Correctness/GetPricingSpec. Existing GetPricingSpec behavior was preserved.

Core's independent published-dependency check initially failed
TestHopsReachCoreSpecVersion: SDK v0.7.9 exceeds its newest v0.7.7 upgrade guide.
The initial local feature SDK's SpecVersion constant was v0.7.7, so its
feature-workspace suite passed despite the published-dependency mismatch.

These conformance and upgrade-guide gaps were fixed upstream and incorporated
by the subsequent rebase. The post-rebase results below supersede those initial
baseline failures.

## Post-rebase validation

On 2026-10-21 all three feature branches were rebased onto fetched origin/main:
plugin b3a48ec, SDK 812890e, and core 71029d7c. Backup branches retain the
pre-rebase heads. Range comparisons confirm the implementation patches are
unchanged. Plugin documentation conflicts were resolved using the new upstream
layout, including its canonical permissions reference.

| Check after rebase | Result |
| --- | --- |
| SDK standalone make validate | Pass with GOMAXPROCS=4 and GOFLAGS=-p=2 |
| SDK TypeScript workspace tests/builds | Pass: 260 tests and all three builds |
| Plugin make test lint build with feature workspace | Pass, including conformance |
| Core complete go test ./... with extended feature workspace | Pass, with real CLI/plugin/MCP process integration enabled |
| Core make build validate docs-lint | Pass |
| Core complete compatible golangci-lint 2.15.0 make lint | Pass |
| Plugin final documentation/OpenSpec lint | Pass |

The first SDK validation attempt under concurrent load failed two timeout-based
tests: allocator Connect shutdown and native credential round-trip. Full
validation passed on retry with reduced parallelism and no source changes.

The rebased SDK now reports v0.7.9 and includes capability-aware conformance;
the rebased core includes upgrade hops through v0.7.9. Both previous baseline
failures are resolved. Core lint still uses the explicit compatible 2.15.0
override; the tracked 2.13.2 pin and its existing mapliteral configuration have
not changed.

## Release prerequisites

Before standalone CI/release outside the workspace, publish an appropriate SDK
revision, pin it in both consumers, coordinate the new protocol/version and
upgrade guide metadata, and reconcile the pinned core linter configuration.
No local-path go.mod replacements or invented unpublished version pins were
committed. The paired binaries built through the workspace contain the new
SDK code; the workspace is a build-time requirement, not a runtime service.

## Decisions made during implementation

- Use three isolated feature worktrees to protect existing core Terraform work;
  integration requires coordinating three repository branches.
- Run sequential implementation tasks with independent reviews, as selected by
  the user; this trades additional review turns for intermediate checks.
- Require explicit historical capability advertisement. Generated server stubs
  otherwise make old plugins structurally appear to implement the new RPC.
  New providers must explicitly advertise the capability.
- Preserve SDK validation precedence for malformed wire requests. Valid disabled
  requests fail before credentials; malformed requests may first return
  InvalidArgument.
- Preserve intentional monetary behavior and report independently reproduced
  baseline failures. Release readiness still depends on reconciling them.
- Expose the report through the existing read-only MCP command mapping. This
  adds a read-only tool surface; enumeration is offline and invocation retains
  the plugin opt-in requirement.
- Isolate generated temporary-module test builds with GOWORK=off, so they check
  their own dependency manifests rather than inheriting development workspace
  overrides. Production scaffold dependencies remain unchanged.
- Record the browser-screen UX follow-up locally. The authenticated API hook is
  implemented; a dedicated report screen is future UI work.
