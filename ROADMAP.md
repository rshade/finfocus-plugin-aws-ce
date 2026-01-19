# Roadmap

This roadmap outlines the development path for `pulumicost-plugin-aws-ce`, prioritizing direct API integration, FOCUS standard compliance, and adherence to the `finfocus-spec` v0.5.2+.

> **Constitutional Reference:** All features must comply with [CONTEXT.md](./CONTEXT.md) boundaries. Features violating "Hard No's" are rejected.

## Overview

| Milestone | Focus | Status |
|-----------|-------|--------|
| v0.1.0 | Foundation & CI/CD | 🔄 In Progress |
| v0.2.0 | Core Features | 📋 Planned |
| v0.3.0 | Advanced Features | 🔬 Research |

## Past Milestones (Done)

### SDK Compliance & ARN Support

| Status | Issue | Description |
|--------|-------|-------------|
| ✅ Done | [#6](https://github.com/rshade/pulumicost-plugin-aws-ce/issues/6) | SDK Compliance - Adopt `pluginsdk` for logging, validation, and config |
| ✅ Done | [#14](https://github.com/rshade/pulumicost-plugin-aws-ce/issues/14) | ARN Support - Primary identifier in `GetActualCostRequest` |
| ✅ Done | [#2](https://github.com/rshade/pulumicost-plugin-aws-ce/issues/2) | Initial plugin creation for real AWS billing data |

## Current Focus (v0.1.0 - Foundation & CI/CD)

| Status | Issue | Technical Thesis | Boundary Guardrail |
|--------|-------|------------------|-------------------|
| 🔄 In Progress | [#7](https://github.com/rshade/pulumicost-plugin-aws-ce/issues/7) | Establish GitHub Actions for testing, linting, releasing | No complex multi-arch builds unless requested |
| 🔄 In Progress | [#11](https://github.com/rshade/pulumicost-plugin-aws-ce/issues/11) | Core Cost Plugin - `GetActualCost` with FOCUS 1.2 records | Use values directly from `GetCostAndUsage` |
| 📋 Planned | [#12](https://github.com/rshade/pulumicost-plugin-aws-ce/issues/12) | Installation & Documentation polish | Out-of-the-box experience |
| 📋 Planned | [#31](https://github.com/rshade/pulumicost-plugin-aws-ce/issues/31) | Plugin Conformance Test Suite integration | Do not modify test suite to pass |
| 📋 Planned | [#23](https://github.com/rshade/pulumicost-plugin-aws-ce/issues/23) | Update finfocus-spec to enable gRPC reflection | Dependency update only |

## Near-Term Vision (v0.2.0 - Core Features)

| Status | Issue | Technical Thesis | Boundary Guardrail |
|--------|-------|------------------|-------------------|
| 📋 Planned | [#8](https://github.com/rshade/pulumicost-plugin-aws-ce/issues/8) / [#24](https://github.com/rshade/pulumicost-plugin-aws-ce/issues/24) | AWS Budgets - Proxy `budgets:DescribeBudgets` | Read-only; no alerting logic |
| 📋 Planned | [#25](https://github.com/rshade/pulumicost-plugin-aws-ce/issues/25) | Cost Forecasting - Proxy `ce:GetCostForecast` | **HARD NO:** No custom forecasting math |
| 🔬 Research | [#26](https://github.com/rshade/pulumicost-plugin-aws-ce/issues/26) | Anomaly Detection - Map `ce:GetAnomalies` | **HARD NO:** No local ML models |

## Future Vision (v0.3.0+ - Advanced Features)

### Optimization Recommendations

| Status | Issue | Technical Thesis | Boundary Guardrail |
|--------|-------|------------------|-------------------|
| 🔬 Research | [#13](https://github.com/rshade/pulumicost-plugin-aws-ce/issues/13) | Optimization Recommendations (umbrella) | Proxy AWS recommendations only |
| 🔬 Research | [#27](https://github.com/rshade/pulumicost-plugin-aws-ce/issues/27) | Rightsizing - `ce:GetRightsizingRecommendation` | Do not calculate utilization locally |
| 🔬 Research | [#32](https://github.com/rshade/pulumicost-plugin-aws-ce/issues/32) | Savings Plans Recommendations | Rely on AWS for ROI/break-even math |
| 🔬 Research | [#33](https://github.com/rshade/pulumicost-plugin-aws-ce/issues/33) | Reserved Instance Recommendations | Focus on non-Compute (RDS, Redshift) |
| 🔬 Research | [#22](https://github.com/rshade/pulumicost-plugin-aws-ce/issues/22) | RI/SP Purchase Recommendations (combined) | Proxy AWS API only |

### Coverage Detection

| Status | Issue | Technical Thesis | Boundary Guardrail |
|--------|-------|------------------|-------------------|
| 🔬 Research | [#19](https://github.com/rshade/pulumicost-plugin-aws-ce/issues/19) | RI Coverage Detection & Pricing | Read coverage data only |
| 🔬 Research | [#20](https://github.com/rshade/pulumicost-plugin-aws-ce/issues/20) | Savings Plans Coverage Detection | Read coverage data only |
| 🔬 Research | [#21](https://github.com/rshade/pulumicost-plugin-aws-ce/issues/21) | Blended vs Unblended Cost Comparison | Map existing CE data fields |

### Pricing & Estimation

| Status | Issue | Technical Thesis | Boundary Guardrail |
|--------|-------|------------------|-------------------|
| 🔬 Research | [#30](https://github.com/rshade/pulumicost-plugin-aws-ce/issues/30) | EstimateCost (What-If) via Pricing API | Disclaimer: list prices only |
| 🔬 Research | [#34](https://github.com/rshade/pulumicost-plugin-aws-ce/issues/34) | Spot Market Advisor | Use external data feeds for risk |

### Standards & Compliance

| Status | Issue | Technical Thesis | Boundary Guardrail |
|--------|-------|------------------|-------------------|
| 🔬 Research | [#28](https://github.com/rshade/pulumicost-plugin-aws-ce/issues/28) | FOCUS 1.3 Transition (Commitment columns) | Map only what AWS provides explicitly |
| 🔬 Research | [#29](https://github.com/rshade/pulumicost-plugin-aws-ce/issues/29) | Greenops Discovery (Carbon API) | **Blocked:** No public AWS Carbon API |

## Icebox / Backlog

| Status | Issue | Description |
|--------|-------|-------------|
| 📋 Backlog | [#3](https://github.com/rshade/pulumicost-plugin-aws-ce/issues/3) | Adopt pluginsdk/mapping for property extraction |

## Rejected / Out of Scope

| Status | Item | Reasoning |
|--------|------|-----------|
| ❌ Rejected | Smart Sizing (Dev Mode) | Violates "No Logic" boundary. Policy-as-Code belongs in core engine. |
| ❌ Blocked | Greenops/Carbon Metrics | No public AWS API available for synchronous queries |
| ❌ Closed | [#1](https://github.com/rshade/pulumicost-plugin-aws-ce/issues/1) | Wrong repo - belongs to aws-public plugin |
| ❌ Closed | [#4](https://github.com/rshade/pulumicost-plugin-aws-ce/issues/4) | Duplicate of #24 |
| ❌ Closed | [#9](https://github.com/rshade/pulumicost-plugin-aws-ce/issues/9) | Duplicate of #25 |
| ❌ Closed | [#10](https://github.com/rshade/pulumicost-plugin-aws-ce/issues/10) | Duplicate of #26 |

## Legend

| Icon | Status | Description |
|------|--------|-------------|
| ✅ | Done | Feature delivered and merged |
| 🔄 | In Progress | Active development |
| 📋 | Planned | Spec drafted, ready for implementation |
| 🔬 | Research | Investigating API capabilities |
| ❌ | Rejected/Blocked | Not implementing |
