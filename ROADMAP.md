# Roadmap

This roadmap outlines the development path for `pulumicost-plugin-aws-ce`, prioritizing direct API integration, FOCUS standard compliance, and adherence to the `pulumicost-spec`.

## 1. Core Platform & Compliance

| Status | Item & Link | Technical Thesis / Scope | Boundary Guardrail (NO-GO) |
| :--- | :--- | :--- | :--- |
| **[In Progress]** | **Core Cost Plugin** (#11) | Implement `GetActualCost` with full FOCUS 1.2 record construction using SDK helpers. | Do not calculate costs locally; use values directly from `GetCostAndUsage`. |
| **[In Progress]** | **CI/CD Infrastructure** (#7) | Establish GitHub Actions for testing, linting, and releasing single binaries. | No complex multi-arch builds unless requested by users (currently focused on typical Linux/Mac envs). |
| **[Done]** | **SDK Compliance** (#6) | Adopt `pluginsdk` for logging, validation, and config. | Do not use `fmt.Println` or custom loggers. |
| **[Done]** | **Contextual Identity** (#14) | Support `arn` in `GetActualCostRequest` as the primary identifier. | Do not attempt to guess ARNs if not provided or resolvable via API. |
| **[Planned]** | **Plugin Conformance** (#31) | Integrate the official `Plugin Conformance Test Suite` from the SDK into `make test`. | Do not modify the test suite to pass; fix the plugin code. |

## 2. Cost Intelligence Features

| Status | Item & Link | Technical Thesis / Scope | Boundary Guardrail (NO-GO) |
| :--- | :--- | :--- | :--- |
| **[Planned]** | **Cost Forecasting** (#25) | Implement `GetProjectedCost` using `ce:GetCostForecast`. | **HARD NO:** Do not implement linear regression or custom forecasting math. If API fails, return error. |
| **[Planned]** | **AWS Budgets** (#24) | Implement `getbudgets` RPC by proxying AWS Budgets API. | Do not implement alerting logic or "budget remaining" math if the API provides it. |
| **[Researching]** | **Anomaly Detection** (#26) | Map `ce:GetAnomalies` to the new `AnomalyRecord` structure. | **HARD NO:** Do not train local ML models. Only report anomalies detected by AWS. |
| **[Researching]** | **Optimization: Rightsizing** (#27) | Implement EC2/RDS rightsizing using `ce:GetRightsizingRecommendation`. | Do not calculate rightsizing locally. Proxy AWS "Modify"/"Terminate" actions. |
| **[Researching]** | **Optimization: Savings Plans** (#32) | Implement SP recommendations using `ce:GetSavingsPlansPurchaseRecommendation`. | Rely on AWS for ROI/Break-even math. |
| **[Researching]** | **Optimization: Reservations** (#33) | Implement RI recommendations using `ce:GetReservationPurchaseRecommendation`. | Focus on non-Compute services (RDS, Redshift) where SPs don't apply. |
| **[Researching]** | **EstimateCost (What-If)** (#30) | Investigate `EstimateCost` RPC using AWS Pricing API or CE "What-If" scenarios. | Do not maintain a local price list database. |
| **[Researching]** | **Spot Market Advisor** (#34) | Implement `DescribeSpotPriceHistory` and investigate Risk data sources. | Do not calculate risk scores; rely on external data feeds (Spot Advisor). |

## 3. Advanced Standards & Discovery

| Status | Item & Link | Technical Thesis / Scope | Boundary Guardrail (NO-GO) |
| :--- | :--- | :--- | :--- |
| **[Researching]** | **FOCUS 1.3 Transition** (#28) | Audit new FOCUS 1.3 columns (Contract Commitment) against AWS data. | Do not invent values for new columns. Map only what AWS provides explicitly. |
| **[Researching]** | **Greenops Discovery** (#29) | Investigate AWS Carbon Footprint API availability for Greenops metrics. | Do not calculate carbon emissions based on instance types/usage hours. |

## 4. Rejected / Out of Scope

| Status | Item & Link | Reasoning |
| :--- | :--- | :--- |
| **[Rejected]** | **Smart Sizing (Dev Mode)** | Proposal to recommend `t3` instances for `UsageProfile=DEV`. **Reason:** Violates the "No Logic" boundary. This is Policy-as-Code logic that belongs in `pulumicost-core`, not the data adapter. |

## Legend
*   **[Done]**: Feature delivered and merged.
*   **[In Progress]**: Active development with defined spec.
*   **[Planned]**: Spec drafted or next in queue; ready for implementation.
*   **[Researching]**: Investigating API capabilities; scope and thesis being defined.
