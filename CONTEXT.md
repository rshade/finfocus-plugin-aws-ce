# CONTEXT.md

## Core Architectural Identity

**Lightweight gRPC Plugin / Adapter**
This project is a stateless "Provider Plugin" for the FinFocus engine. It acts as a translation layer between the `finfocus-core` (gRPC client) and the AWS Cost Explorer API (External Service).

It is **NOT** a standalone application, CLI tool, or dashboard. It is a worker node in a plugin architecture.

## Technical Boundaries & Hard No's

To prevent scope creep and architectural drift, this project adheres to the following boundaries:

1. **No "Fin" Logic (Math):** We do not invent forecasting algorithms, amortizations, or cost models. If AWS provides the number (e.g., via `GetCostForecast`), we use it. We only perform basic arithmetic (e.g., currency conversion, summing) if absolutely necessary to fit the FOCUS 1.2 schema.
2. **No "Ops" Logic (Resource Management):** This plugin is **Read-Only**. It will NEVER create, modify, or delete AWS resources (EC2, S3, etc.). It only reads billing data.
3. **No Durable State:** This plugin does not own a database. It utilizes a transient file-based cache for performance (to reduce API costs/latency) but assumes it can be restarted at any time with zero data loss.
4. **No UI/UX:** This project does not render HTML, charts, or CLI tables. It returns raw structured data (Protobuf/Go structs) to the Core engine.

## Data Source of Truth

* **Actual Costs:** AWS Cost Explorer API (`GetCostAndUsage` for service totals, `GetCostAndUsageWithResources` for EC2 instances).
* **Forecasts:** Planned AWS Cost Explorer API (`GetCostForecast`); v0.1.0 serves actual costs only.
* **Budgets:** AWS Budgets API (Planned).
* **Metadata:** Values supplied by Cost Explorer and the caller; AWS Tags and Organizations integration is not implemented.

**Responsibility:** AWS is responsible for the accuracy of the billing data. We are responsible for the accuracy of the **translation** to the FOCUS 1.2 standard.

## Interaction Model

* **Inbound:** gRPC Server (listening on localhost, port assigned by Core).
* **Outbound:** AWS SDK for Go v2 (authenticated via standard AWS chains).
* **Data Format:** Returns data complying strictly with the `finfocus-spec` (FOCUS 1.2 derived) Protocol Buffers.

## Implementation Status

### Currently Implemented

| RPC | Status | AWS API |
|-----|--------|---------|
| `GetActualCost` | ✅ Offline protocol/contract verified | `GetCostAndUsage`, `GetCostAndUsageWithResources` |
| `GetProjectedCost` | ❌ Outside v0.1.0 scope | Returns gRPC `Unimplemented` (CE-6.10) |

### SDK Compliance (v0.7.5)

The plugin uses standardized SDK helpers:

* `pluginsdk.NewLogWriter()` / `NewPluginLogger()` - Structured logging
* `pluginsdk.LogOperation()` - Operation timing
* `pluginsdk.ValidateActualCostRequest()` - Input validation
* `pluginsdk.ParsePortFlag()` / `GetPort()` - Configuration
* `pluginsdk.Serve()` - gRPC server startup

### Extension Points Available

1. **New AWS Cost APIs** - CostExplorerAPI interface supports additional methods
2. **Additional Cost Dimensions** - SERVICE, USAGE_TYPE, TAG-based grouping
3. **Filter Expressions** - Cost Explorer query filters
4. **Cache Configuration** - TTL and storage location customization

### Future work and current limits

The v0.7.5 SDK already provides budgets and recommendations interfaces and
FOCUS commitment fields. Those features need plugin implementations and
verification; their presence is not evidence of a protocol gap. Anomaly
mapping requires separate design work outside this release.

The caller must supply `billing_account_id` for FOCUS output. Without it,
actual costs are returned with no FOCUS record. Credential keys are plugin
conventions documented in [AWS credentials](docs/CREDENTIALS.md). Resource
queries need account opt-in, cover 14 days and can lag 24 to 48 hours.
Pulumi ESC supplied an AWS identity for live checks. After the owner granted
Cost Explorer read permission, the plugin matched nine live service totals
for 2026-09-27 through 2026-10-04, including exact decimals and period metadata.
The initial denied query also verified PermissionDenied mapping. Resource-level
and FinFocus core E2E verification remain separate. See docs/AWS-VERIFICATION.md.

## Verification Checklist

When adding new features, verify:

1. ✅ Does it only read data from AWS? (No writes allowed)
2. ✅ Does it avoid local calculations? (Pass-through only)
3. ✅ Does it use SDK helpers for logging/validation?
4. ✅ Does it map to an existing AWS API? (No invented data)
5. ✅ Does it comply with FOCUS schema requirements?
