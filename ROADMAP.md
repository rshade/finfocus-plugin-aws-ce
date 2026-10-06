# Roadmap

This roadmap outlines the development path for `finfocus-plugin-aws-ce`, prioritizing direct API integration, FOCUS standard compliance, and adherence to the `finfocus-spec` v0.7.5.

> **Constitutional Reference:** All features must comply with [CONTEXT.md](./CONTEXT.md) boundaries. Features violating "Hard No's" are rejected.

## Overview

| Milestone | Focus | Status |
|-----------|-------|--------|
| v0.1.0 | Foundation & CI/CD | 🔄 Offline readiness verified; release pending |
| v0.2.0 | Core Features | 📋 Planned |
| v0.3.0 | Advanced Features | 🔬 Research |

## Past Milestones (Done)

### SDK Compliance & ARN Support

| Status | Issue | Description |
|--------|-------|-------------|
| ✅ Done | [#6](https://github.com/rshade/finfocus-plugin-aws-ce/issues/6) | SDK Compliance - Adopt `pluginsdk` for logging, validation, and config |
| ✅ Done | [#14](https://github.com/rshade/finfocus-plugin-aws-ce/issues/14) | ARN Support - Primary identifier in `GetActualCostRequest` |
| ✅ Done | [#2](https://github.com/rshade/finfocus-plugin-aws-ce/issues/2) | Initial plugin creation for real AWS billing data |
| ✅ Done | [#7](https://github.com/rshade/finfocus-plugin-aws-ce/issues/7) | Establish CI/CD Infrastructure |

Release plumbing, actual-cost behavior, integration and conformance tests are
verified offline. Live CE-6.1 and CE-5.1 remain blocked on credentials.
The whole-repository Markdown check retains frozen-history errors; vulnerability
scanning retains the known gRPC finding. Docker support is removed by owner decision.
Local delivery labels do not close GitHub issues.

## Current Focus (v0.1.0 - Foundation & CI/CD)

| Status | Issue | Technical Thesis | Boundary Guardrail |
|--------|-------|------------------|-------------------|
| 🔄 In Progress | [#11](https://github.com/rshade/finfocus-plugin-aws-ce/issues/11) | Core Cost Plugin - `GetActualCost` with FOCUS 1.2 records | Use values directly from `GetCostAndUsage` |
| ✅ Implemented | [#12](https://github.com/rshade/finfocus-plugin-aws-ce/issues/12) | Installation & Documentation polish | Out-of-the-box experience |
| ✅ Implemented | [#31](https://github.com/rshade/finfocus-plugin-aws-ce/issues/31) | Plugin Conformance Test Suite integration | Do not modify test suite to pass |
| 📋 Planned | [#23](https://github.com/rshade/finfocus-plugin-aws-ce/issues/23) | Use finfocus-spec v0.7.5 | Dependency update only |

## Near-Term Vision (v0.2.0 - Core Features)

### Plugin Infrastructure (HIGH Priority)

| Status | Issue | Technical Thesis | Boundary Guardrail |
|--------|-------|------------------|-------------------|
| ✅ Implemented | [#40](https://github.com/rshade/finfocus-plugin-aws-ce/issues/40) | Add GetPluginInfo() RPC | Return plugin metadata for discovery |
| ✅ Implemented | [#41](https://github.com/rshade/finfocus-plugin-aws-ce/issues/41) | Add Supports() RPC | Validate resource/provider support |
| ⏭️ Removed | [#42](https://github.com/rshade/finfocus-plugin-aws-ce/issues/42) | Docker support | Owner decision: binary archives only |
| 📋 Planned | [#43](https://github.com/rshade/finfocus-plugin-aws-ce/issues/43) | HTTP health endpoint | Container orchestration support |
| 📋 Planned | [#44](https://github.com/rshade/finfocus-plugin-aws-ce/issues/44) | Documentation directory | API and deployment guides |
| ✅ Implemented | [#45](https://github.com/rshade/finfocus-plugin-aws-ce/issues/45) | Integration tests for gRPC server | Server behavior verification |
| ✅ Implemented | [#46](https://github.com/rshade/finfocus-plugin-aws-ce/issues/46) | Trace ID propagation | Distributed tracing support |
| ✅ Implemented | [#47](https://github.com/rshade/finfocus-plugin-aws-ce/issues/47) | Proto ErrorCode enum | Standardized error handling |
| 📋 Planned | [#48](https://github.com/rshade/finfocus-plugin-aws-ce/issues/48) | Standardize workflow names | CI/CD consistency |
| ✅ Implemented | [#50](https://github.com/rshade/finfocus-plugin-aws-ce/issues/50) | Makefile targets | Developer workflows; binary archives only |
| ✅ Implemented | [#51](https://github.com/rshade/finfocus-plugin-aws-ce/issues/51) | Config parsing tests | Configuration reliability |
| 📋 Planned | [#53](https://github.com/rshade/finfocus-plugin-aws-ce/issues/53) | CONTRIBUTING.md | Contributor onboarding |

### AWS Cost Features

| Status | Issue | Technical Thesis | Boundary Guardrail |
|--------|-------|------------------|-------------------|
| 📋 Planned | [#8](https://github.com/rshade/finfocus-plugin-aws-ce/issues/8) / [#24](https://github.com/rshade/finfocus-plugin-aws-ce/issues/24) | AWS Budgets - Proxy `budgets:DescribeBudgets` | Read-only; no alerting logic |
| 📋 Planned | [#25](https://github.com/rshade/finfocus-plugin-aws-ce/issues/25) | Cost Forecasting - Proxy `ce:GetCostForecast` | **HARD NO:** No custom forecasting math |
| 📋 Planned | [#36](https://github.com/rshade/finfocus-plugin-aws-ce/issues/36) | GetProjectedCost with prediction intervals | Use AWS forecast intervals |
| 📋 Planned | [#37](https://github.com/rshade/finfocus-plugin-aws-ce/issues/37) | Enrich GetActualCost with RI/SP data | Map existing CE data |
| 🔬 Research | [#26](https://github.com/rshade/finfocus-plugin-aws-ce/issues/26) / [#38](https://github.com/rshade/finfocus-plugin-aws-ce/issues/38) | Anomaly Detection - Map `ce:GetAnomalies` | **HARD NO:** No local ML models |

## Future Vision (v0.3.0+ - Advanced Features)

### Plugin Enhancements

| Status | Issue | Technical Thesis | Boundary Guardrail |
|--------|-------|------------------|-------------------|
| 🔬 Research | [#49](https://github.com/rshade/finfocus-plugin-aws-ce/issues/49) | Web/Connect protocol support | Browser client access |
| ✅ Done | [#52](https://github.com/rshade/finfocus-plugin-aws-ce/issues/52) | Metadata enrichment | FOCUS source, granularity, metric, estimate and `lookback` columns |
| 🔬 Research | [#54](https://github.com/rshade/finfocus-plugin-aws-ce/issues/54) | CORS support | Browser-based access |
| ✅ Done | [#55](https://github.com/rshade/finfocus-plugin-aws-ce/issues/55) | Batch configuration | Validated SDK batch limits and safe shared client initialization |

### Optimization Recommendations

| Status | Issue | Technical Thesis | Boundary Guardrail |
|--------|-------|------------------|-------------------|
| 🔬 Research | [#13](https://github.com/rshade/finfocus-plugin-aws-ce/issues/13) | Optimization Recommendations (umbrella) | Proxy AWS recommendations only |
| 🔬 Research | [#27](https://github.com/rshade/finfocus-plugin-aws-ce/issues/27) | Rightsizing - `ce:GetRightsizingRecommendation` | Do not calculate utilization locally |
| 🔬 Research | [#32](https://github.com/rshade/finfocus-plugin-aws-ce/issues/32) | Savings Plans Recommendations | Rely on AWS for ROI/break-even math |
| 🔬 Research | [#33](https://github.com/rshade/finfocus-plugin-aws-ce/issues/33) | Reserved Instance Recommendations | Focus on non-Compute (RDS, Redshift) |
| 🔬 Research | [#22](https://github.com/rshade/finfocus-plugin-aws-ce/issues/22) | RI/SP Purchase Recommendations (combined) | Proxy AWS API only |

### Coverage Detection

| Status | Issue | Technical Thesis | Boundary Guardrail |
|--------|-------|------------------|-------------------|
| 🔬 Research | [#19](https://github.com/rshade/finfocus-plugin-aws-ce/issues/19) | RI Coverage Detection & Pricing | Read coverage data only |
| 🔬 Research | [#20](https://github.com/rshade/finfocus-plugin-aws-ce/issues/20) | Savings Plans Coverage Detection | Read coverage data only |
| 🔬 Research | [#21](https://github.com/rshade/finfocus-plugin-aws-ce/issues/21) | Blended vs Unblended Cost Comparison | Map existing CE data fields |

### Pricing & Estimation

| Status | Issue | Technical Thesis | Boundary Guardrail |
|--------|-------|------------------|-------------------|
| 🔬 Research | [#30](https://github.com/rshade/finfocus-plugin-aws-ce/issues/30) | EstimateCost (What-If) via Pricing API | Disclaimer: list prices only |
| 🔬 Research | [#34](https://github.com/rshade/finfocus-plugin-aws-ce/issues/34) | Spot Market Advisor | Use external data feeds for risk |

### Standards & Compliance

| Status | Issue | Technical Thesis | Boundary Guardrail |
|--------|-------|------------------|-------------------|
| 🔬 Research | [#28](https://github.com/rshade/finfocus-plugin-aws-ce/issues/28) | FOCUS 1.3 Transition (Commitment columns) | Map only what AWS provides explicitly |
| 🔬 Research | [#29](https://github.com/rshade/finfocus-plugin-aws-ce/issues/29) | Greenops Discovery (Carbon API) | **Blocked:** No public AWS Carbon API |

## Icebox / Backlog

| Status | Issue | Description |
|--------|-------|-------------|
| 📋 Backlog | [#3](https://github.com/rshade/finfocus-plugin-aws-ce/issues/3) | Adopt pluginsdk/mapping for property extraction |

## Rejected / Out of Scope

| Status | Item | Reasoning |
|--------|------|-----------|
| ❌ Rejected | Smart Sizing (Dev Mode) | Violates "No Logic" boundary. Policy-as-Code belongs in core engine. |
| ❌ Blocked | Greenops/Carbon Metrics | No public AWS API available for synchronous queries |
| ❌ Closed | [#1](https://github.com/rshade/finfocus-plugin-aws-ce/issues/1) | Wrong repo - belongs to aws-public plugin |
| ❌ Closed | [#4](https://github.com/rshade/finfocus-plugin-aws-ce/issues/4) | Duplicate of #24 |
| ❌ Closed | [#9](https://github.com/rshade/finfocus-plugin-aws-ce/issues/9) | Duplicate of #25 |
| ❌ Closed | [#10](https://github.com/rshade/finfocus-plugin-aws-ce/issues/10) | Duplicate of #26 |

## Legend

| Icon | Status | Description |
|------|--------|-------------|
| ✅ | Done | Implemented in the checkout; merging and releasing are separate steps |
| 🔄 | In Progress | Active development |
| 📋 | Planned | Spec drafted, ready for implementation |
| 🔬 | Research | Investigating API capabilities |
| ❌ | Rejected/Blocked | Not implementing |
