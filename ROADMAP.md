# Roadmap

This roadmap outlines the development path for `finfocus-plugin-aws-ce`, prioritizing direct API integration, FOCUS standard compliance, and adherence to the `finfocus-spec` v0.7.5.

> **Constitutional reference:** all features must comply with [CONTEXT.md](./CONTEXT.md) boundaries. The project rejects features that violate these boundaries.

## Overview

| Milestone | Area | Status |
|-----------|-------|--------|
| v0.1.0 | Foundation and continuous delivery | 🔄 Offline readiness verified. Release pending |
| v0.2.0 | Core Features | 📋 Planned |
| v0.3.0 | Advanced Features | 🔬 Research |

## Completed milestones

### SDK compliance and ARN support

| Status | Issue | Description |
|--------|-------|-------------|
| ✅ Done | [#6](https://github.com/rshade/finfocus-plugin-aws-ce/issues/6) | SDK Compliance - Adopt `pluginsdk` for logging, validation, and config |
| ✅ Done | [#14](https://github.com/rshade/finfocus-plugin-aws-ce/issues/14) | ARN Support - Primary identifier in `GetActualCostRequest` |
| ✅ Done | [#2](https://github.com/rshade/finfocus-plugin-aws-ce/issues/2) | Initial plugin creation for real AWS billing data |
| ✅ Done | [#7](https://github.com/rshade/finfocus-plugin-aws-ce/issues/7) | Establish continuous delivery infrastructure |

Offline verification covers release plumbing, actual-cost behavior, integration, and conformance tests.
Pulumi Environments, Secrets, and Configuration also supplied AWS credentials. Live service
cost comparison passed after the owner granted Cost Explorer read permission.
Resource-level verification is separate, and core E2E remains outside this run.
The whole-repository Markdown check retains frozen-history errors. Vulnerability
scanning retains the known gRPC finding. The owner removed Docker support.
Local delivery labels don't close GitHub issues.

## Foundation and continuous delivery for v0.1.0

| Status | Issue | Technical Thesis | Boundary Guardrail |
|--------|-------|------------------|-------------------|
| 🔄 In Progress | [#11](https://github.com/rshade/finfocus-plugin-aws-ce/issues/11) | Core Cost Plugin - `GetActualCost` with FOCUS 1.2 records | Use values directly from `GetCostAndUsage` |
| ✅ Implemented | [#12](https://github.com/rshade/finfocus-plugin-aws-ce/issues/12) | Installation & Documentation polish | Ready to use after installation |
| ✅ Implemented | [#31](https://github.com/rshade/finfocus-plugin-aws-ce/issues/31) | Plugin Conformance Test Suite integration | Don't modify test suite to pass |
| 📋 Planned | [#23](https://github.com/rshade/finfocus-plugin-aws-ce/issues/23) | Use `finfocus-spec` v0.7.5 | Dependency update only |

## Core features for v0.2.0

### Plugin infrastructure with high priority

| Status | Issue | Technical Thesis | Boundary Guardrail |
|--------|-------|------------------|-------------------|
| ✅ Implemented | [#40](https://github.com/rshade/finfocus-plugin-aws-ce/issues/40) | Add `GetPluginInfo()` remote procedure call | Return plugin metadata for discovery |
| ✅ Implemented | [#41](https://github.com/rshade/finfocus-plugin-aws-ce/issues/41) | Add `Supports()` remote procedure call | Validate resource/provider support |
| ⏭️ Removed | [#42](https://github.com/rshade/finfocus-plugin-aws-ce/issues/42) | Docker support | Owner decision: binary archives only |
| 📋 Planned | [#43](https://github.com/rshade/finfocus-plugin-aws-ce/issues/43) | HTTP health endpoint | Container orchestration support |
| 📋 Planned | [#44](https://github.com/rshade/finfocus-plugin-aws-ce/issues/44) | Documentation directory | API and deployment guides |
| ✅ Implemented | [#45](https://github.com/rshade/finfocus-plugin-aws-ce/issues/45) | Integration tests for gRPC server | Server behavior verification |
| ✅ Implemented | [#46](https://github.com/rshade/finfocus-plugin-aws-ce/issues/46) | Trace ID propagation | Distributed tracing support |
| ✅ Implemented | [#47](https://github.com/rshade/finfocus-plugin-aws-ce/issues/47) | Proto ErrorCode enum | Standardized error handling |
| 📋 Planned | [#48](https://github.com/rshade/finfocus-plugin-aws-ce/issues/48) | Standardize workflow names | Continuous delivery consistency |
| ✅ Implemented | [#50](https://github.com/rshade/finfocus-plugin-aws-ce/issues/50) | Makefile targets | Developer workflows. Binary archives only |
| ✅ Implemented | [#51](https://github.com/rshade/finfocus-plugin-aws-ce/issues/51) | Configuration parsing tests | Configuration reliability |
| 📋 Planned | [#53](https://github.com/rshade/finfocus-plugin-aws-ce/issues/53) | CONTRIBUTING.md | Contributor setup |

### AWS cost features

| Status | Issue | Technical Thesis | Boundary Guardrail |
|--------|-------|------------------|-------------------|
| 📋 Planned | [#8](https://github.com/rshade/finfocus-plugin-aws-ce/issues/8) / [#24](https://github.com/rshade/finfocus-plugin-aws-ce/issues/24) | AWS Budgets - Proxy `budgets:DescribeBudgets` | Read-only. No alerting logic |
| 📋 Planned | [#25](https://github.com/rshade/finfocus-plugin-aws-ce/issues/25) | Cost Forecasting - Proxy `ce:GetCostForecast` | **Boundary:** no custom forecasting math |
| 📋 Planned | [#36](https://github.com/rshade/finfocus-plugin-aws-ce/issues/36) | GetProjectedCost with prediction intervals | Use AWS forecast intervals |
| 📋 Planned | [#37](https://github.com/rshade/finfocus-plugin-aws-ce/issues/37) | Enrich GetActualCost with RI/SP data | Map existing CE data |
| 🔬 Research | [#26](https://github.com/rshade/finfocus-plugin-aws-ce/issues/26) / [#38](https://github.com/rshade/finfocus-plugin-aws-ce/issues/38) | Anomaly Detection - Map `ce:GetAnomalies` | **Boundary:** no local machine learning models |

## Advanced features for v0.3.0+

### Plugin enhancements

| Status | Issue | Technical Thesis | Boundary Guardrail |
|--------|-------|------------------|-------------------|
| 🔬 Research | [#49](https://github.com/rshade/finfocus-plugin-aws-ce/issues/49) | Web/Connect protocol support | Browser client access |
| ✅ Done | [#52](https://github.com/rshade/finfocus-plugin-aws-ce/issues/52) | Metadata enrichment | FOCUS source, granularity, metric, estimate and `lookback` columns |
| 🔬 Research | [#54](https://github.com/rshade/finfocus-plugin-aws-ce/issues/54) | Cross-origin resource sharing support | Browser-based access |
| ✅ Done | [#55](https://github.com/rshade/finfocus-plugin-aws-ce/issues/55) | Batch configuration | Validated SDK batch limits and safe shared client initialization |

### Optimization recommendations

| Status | Issue | Technical Thesis | Boundary Guardrail |
|--------|-------|------------------|-------------------|
| 🔬 Research | [#13](https://github.com/rshade/finfocus-plugin-aws-ce/issues/13) | Optimization recommendations across services | Proxy AWS recommendations only |
| 🔬 Research | [#27](https://github.com/rshade/finfocus-plugin-aws-ce/issues/27) | Rightsizing - `ce:GetRightsizingRecommendation` | Don't calculate utilization locally |
| 🔬 Research | [#32](https://github.com/rshade/finfocus-plugin-aws-ce/issues/32) | Savings Plans Recommendations | Rely on AWS for return on investment and break-even math |
| 🔬 Research | [#33](https://github.com/rshade/finfocus-plugin-aws-ce/issues/33) | Reserved Instance Recommendations | Target non-compute services such as RDS and Redshift |
| 🔬 Research | [#22](https://github.com/rshade/finfocus-plugin-aws-ce/issues/22) | Combined RI/SP purchase recommendations | Proxy AWS API only |

### Coverage detection

| Status | Issue | Technical Thesis | Boundary Guardrail |
|--------|-------|------------------|-------------------|
| 🔬 Research | [#19](https://github.com/rshade/finfocus-plugin-aws-ce/issues/19) | RI Coverage Detection & Pricing | Read coverage data only |
| 🔬 Research | [#20](https://github.com/rshade/finfocus-plugin-aws-ce/issues/20) | Savings Plans Coverage Detection | Read coverage data only |
| 🔬 Research | [#21](https://github.com/rshade/finfocus-plugin-aws-ce/issues/21) | Blended vs Unblended Cost Comparison | Map existing CE data fields |

### Pricing and estimation

| Status | Issue | Technical Thesis | Boundary Guardrail |
|--------|-------|------------------|-------------------|
| 🔬 Research | [#30](https://github.com/rshade/finfocus-plugin-aws-ce/issues/30) | What-if `EstimateCost` via Pricing API | Disclaimer: list prices only |
| 🔬 Research | [#34](https://github.com/rshade/finfocus-plugin-aws-ce/issues/34) | Spot Market Advisor | Use external data feeds for risk |

### Standards and compliance

| Status | Issue | Technical Thesis | Boundary Guardrail |
|--------|-------|------------------|-------------------|
| 🔬 Research | [#28](https://github.com/rshade/finfocus-plugin-aws-ce/issues/28) | FOCUS 1.3 transition with commitment columns | Map only what AWS provides explicitly |
| 🔬 Research | [#29](https://github.com/rshade/finfocus-plugin-aws-ce/issues/29) | Greenops discovery via Carbon API | **Blocked:** no public AWS Carbon API |

## Icebox and backlog

| Status | Issue | Description |
|--------|-------|-------------|
| 📋 Backlog | [#3](https://github.com/rshade/finfocus-plugin-aws-ce/issues/3) | Adopt pluginsdk/mapping for property extraction |

## Rejected and out of scope

| Status | Item | Reasoning |
|--------|------|-----------|
| ❌ Rejected | Smart sizing in development mode | Violates "No Logic" boundary. Policy-as-Code belongs in core engine. |
| ❌ Blocked | Greenops/Carbon Metrics | No public AWS API available for synchronous queries |
| ❌ Closed | [#1](https://github.com/rshade/finfocus-plugin-aws-ce/issues/1) | Wrong repo - belongs to `aws-public` plugin |
| ❌ Closed | [#4](https://github.com/rshade/finfocus-plugin-aws-ce/issues/4) | Duplicate of #24 |
| ❌ Closed | [#9](https://github.com/rshade/finfocus-plugin-aws-ce/issues/9) | Duplicate of #25 |
| ❌ Closed | [#10](https://github.com/rshade/finfocus-plugin-aws-ce/issues/10) | Duplicate of #26 |

## Legend

| Icon | Status | Description |
|------|--------|-------------|
| ✅ | Done | Implemented in the checkout. Merging and releasing are separate steps |
| 🔄 | In Progress | Active development |
| 📋 | Planned | Spec drafted, ready for implementation |
| 🔬 | Research | Investigating API capabilities |
| ❌ | Rejected/Blocked | Not implementing |
