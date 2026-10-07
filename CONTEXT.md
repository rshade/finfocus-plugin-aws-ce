# Architectural context

## Core architectural identity

**Lightweight gRPC plugin and adapter**
This project is a stateless provider plugin for the FinFocus engine. It acts as a translation layer between the `finfocus-core` gRPC client and the external AWS Cost Explorer API.

The plugin operates as a worker node in a plugin architecture. It doesn't provide a standalone app, command-line tool, or dashboard.

## Technical boundaries

To prevent scope creep and architectural drift, this project adheres to the following boundaries:

1. **No financial modeling:** the plugin doesn't invent forecasting algorithms, amortizations, or cost models. If AWS provides the number, for example through `GetCostForecast`, the plugin uses it. The plugin performs basic arithmetic, such as currency conversion or summing, only when necessary to fit the FOCUS 1.2 schema.
2. **No resource management:** the plugin only reads billing data. It never creates, modifies, or deletes AWS resources such as EC2 instances or S3 buckets.
3. **No durable state:** the plugin doesn't own a database. It uses a transient file-based cache to reduce API costs and latency. Restarting the plugin at any time must cause no data loss.
4. **No user interface:** the project doesn't render HTML, charts, or command-line tables. It returns raw structured data as Protocol Buffers or Go structs to the core engine.

## Data source of truth

* **Actual costs:** AWS Cost Explorer API supplies `GetCostAndUsage` for service totals and `GetCostAndUsageWithResources` for EC2 instances.
* **Forecasts:** planned support uses AWS Cost Explorer API `GetCostForecast`. v0.1.0 serves actual costs only.
* **Budgets:** planned support uses AWS Budgets API.
* **Metadata:** Cost Explorer and the caller supply values. AWS Tags and Organizations integration remains unimplemented.

**Responsibility:** AWS owns billing data accuracy. The plugin owns the accuracy of the **translation** to the FOCUS 1.2 standard.

## Interaction model

* **Inbound:** a gRPC server listens on localhost at a port the core assigns.
* **Outbound:** AWS SDK for Go v2 authenticates through standard AWS credential chains.
* **Data format:** the plugin returns Protocol Buffers that comply with `finfocus-spec`, derived from FOCUS 1.2.

## Implementation status

### Implemented features

| Remote procedure call | Status | AWS API |
|-----|--------|---------|
| `GetActualCost` | ✅ Offline protocol/contract verified | `GetCostAndUsage`, `GetCostAndUsageWithResources` |
| `GetProjectedCost` | ❌ Outside v0.1.0 scope | Returns gRPC `Unimplemented` with task reference CE-6.10 |

### SDK compliance for v0.7.5

The plugin uses standardized SDK helpers:

* `pluginsdk.NewLogWriter()` / `NewPluginLogger()` - Structured logging
* `pluginsdk.LogOperation()` - Operation timing
* `pluginsdk.ValidateActualCostRequest()` - Input validation
* `pluginsdk.ParsePortFlag()` / `GetPort()` - Configuration
* `pluginsdk.Serve()` - gRPC server startup

### Available extension points

1. **New AWS cost operations** - The `CostExplorerAPI` interface supports additional methods
2. **Additional cost dimensions** - Grouping by `SERVICE`, `USAGE_TYPE`, or tags
3. **Filter expressions** - Cost Explorer query filters
4. **Cache configuration** - Time to live and storage location customization

### Future work and current limits

The v0.7.5 SDK already provides budgets and recommendations interfaces and
FOCUS commitment fields. Those features need plugin implementations and
verification. Their presence isn't evidence of a protocol gap. Anomaly
mapping requires separate design work outside this release.

The caller must supply `billing_account_id` for FOCUS output. Without it,
the plugin returns actual costs with no FOCUS record. Credential keys are plugin
conventions documented in [AWS credentials](docs/CREDENTIALS.md). Resource
queries need account opt-in, cover 14 days, and can lag 24 to 48 hours.
Pulumi Environments, Secrets, and Configuration supplied an AWS identity for
live checks. After the owner granted Cost Explorer read permission, the plugin
matched nine live service totals for 2026-09-27 through 2026-10-04.
The comparison included exact decimals and period metadata.
The initial denied query also verified `PermissionDenied` mapping. Resource-level
and FinFocus core E2E verification remain separate. See [AWS verification](docs/AWS-VERIFICATION.md).

## Verification checklist

When adding new features, verify:

1. ✅ Does it only read data from AWS, without writes?
2. ✅ Does it avoid local calculations and pass through AWS values?
3. ✅ Does it use SDK helpers for logging and validation?
4. ✅ Does it map to an existing AWS API without inventing data?
5. ✅ Does it comply with FOCUS schema requirements?
