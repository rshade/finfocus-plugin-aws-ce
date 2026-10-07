# Product analysis

This analysis compares `finfocus-plugin-aws-ce` with `finfocus-spec`, AWS Cost Explorer capabilities, and the `spec-kit` methodology. It records the planning baseline from December 2025.

## 1. Project alignment with Spec-Kit

The `.gemini/`, `.claude/`, and `specs/` directories establish the project's use of Spec-Kit.

- **Specification:** `specs/001-aws-ce-plugin` retained draft status as of 2025-12-05. The owner considers the specification complete.
- **Dependency:** the planning baseline uses `github.com/rshade/finfocus-spec v0.5.2`, including validation helpers, FOCUS 1.2 support, and the ARN field.

## 2. Upgrades and improvements

- **Resolve the `FR-015` dependency on `FallbackHint`:**
  - **Context:** `specs/001-aws-ce-plugin/spec.md` describes `FR-015` as blocked by issue 124 in `finfocus-spec`.
  - **Action:** check whether the SDK provides `FallbackHint`. Version 0.5.2 introduced this capability. If available, resolve `FR-015` in the implementation.
- **Use the validation helpers from version 0.5.2:**
  - **Context:** version 0.5.2 added request validation helpers to `pluginsdk`.
  - **Action:** use these helpers for `SR-001` input validation when implementing features.
- **Align logging and configuration with the SDK:**
  - **Context:** upstream pull requests 145 and 143 added `FINFOCUS_LOG_FILE` and `--port` parsing.
  - **Action:** configure `main.go` and logger initialization through these helpers to support core orchestration.
- **Apply least privilege to forecasting permissions:**
  - **Issue:** `SR-005` requests `ce:GetCostForecast`, but `FR-007` requires `GetProjectedCost` to return an error in the baseline specification.
  - **Action:** resolve this conflict in a new forecasting specification. Remove the permission if forecasting remains unsupported, or define and implement forecasting explicitly.

## 3. FOCUS 1.2 compliance

- **Standard:** align the plugin with FOCUS 1.2, which upstream pull request 99 added to `finfocus-spec`.
- **Data types:** use Protocol Buffers `double` for financial fields such as billed cost, per the owner's preference. This maps FOCUS decimal values to the SDK's numeric representation.
- **Implementation:** construct cost records through the SDK's `FocusRecordBuilder` to validate schema compliance.

## 4. Missing features and gaps

Create separate specifications for these potential capabilities:

- **AWS Budgets, high priority:**
  - **Basis:** `finfocus-spec` added the `getbudgets` remote procedure call by version 0.5.2.
  - **Proposal:** create `003-aws-budgets/spec.md` with a user story for viewing budget status and map it to the SDK contract.
- **Anomaly detection, high value:**
  - **Basis:** AWS Cost Explorer provides `GetAnomalies`.
  - **Proposal:** create `004-aws-anomalies/spec.md` with a `P2` or `P3` user story for cost anomalies.
- **Forecasting, strategic priority:**
  - **Basis:** `SR-005` in `001-aws-ce-plugin/spec.md` already requests `ce:GetCostForecast`.
  - **Proposal:** create `005-aws-forecasting/spec.md` to implement `GetProjectedCost` through the AWS forecast API.
- **Optimization recommendations from upstream pull request 125:**
  - **Basis:** `finfocus-spec` added `getrecommendations`. AWS provides rightsizing and Savings Plans recommendations.
  - **Proposal:** create `006-aws-recommendations/spec.md` for these capabilities.

## 5. Completed specification updates

The contextual ARN identity work in `specs/002-add-arn-spec` is complete.

- Version 0.5.2 of `finfocus-spec` added `arn` to `GetActualCostRequest`.
- Commit `16cb974` adds ARN support to `GetActualCost` for precise resource identification.
- Use the ARN when available and fall back to `resource_id` for backward compatibility.

## 6. Continuous integration and delivery plan

At the planning baseline, the project lacked the infrastructure present in `finfocus-plugin-aws-public`. The public plugin needs builds for individual regions. This plugin uses a single binary.

Required configuration:

1. **Workflows in `.github/workflows/`:**
   - `test.yml` for standard Go tests.
   - `release.yml` for automated releases through `goreleaser/goreleaser-action`.
   - `release-please.yml` for changelog generation and version updates.
2. **Release configuration:**
   - `.goreleaser.yaml` for a single binary.
   - `release-please-config.json` and `.release-please-manifest.json`.
3. **Local development:**
   - Add convenience targets to `Makefile`.

## 7. Lessons from the sibling project

- Use `pluginsdk/env.go` and `pluginsdk/mapping` helpers.
- Handle zero-value pricing data without errors.
- Use `zerolog` structured logging.

## 8. Execution roadmap and active issues

### Foundation and delivery, version 0.1.0

- **Issue 6, complete:** update dependencies and use SDK helpers for validation, logging, `FINFOCUS_LOG_FILE`, and `--port`. The baseline specification version is 0.5.2.
- **Issue 7:** establish workflows, goreleaser, and release-please configuration.
- **Issue 11:** implement the core cost plugin from specification 001, with AWS integration, continuous integration secrets, FOCUS 1.2 compliance, and tests of the complete workflow.
- **Issue 12:** improve installation and documentation. Fix the Makefile version, rewrite the README, and consolidate manifests.
- **Issue 14, complete:** add ARN to `GetActualCostRequest` in `specs/002-add-arn-spec`.

### Core features, version 0.2.0

- **Issue 8:** define AWS Budgets support in `003-aws-budgets`, with `getbudgets`.
- **Issue 9:** define forecasting in `005-aws-forecasting`, with `GetProjectedCost`.
- **Issue 10:** define anomaly detection in `004-aws-anomalies`.

### Advanced features, version 0.3.0

- **Issue 13:** define optimization recommendations in `006-aws-recommendations` for rightsizing and Savings Plans.
