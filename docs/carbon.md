# Historical carbon reporting

Use historical reports to retrieve AWS-calculated emissions estimates for
published usage months. Reports retain their account, region, service,
accounting method, emissions scope and model version. They are independent of
monetary resource costs and the offline FinFocus carbon estimation plugin.

## Enable reporting

The feature is off by default. Set the plugin configuration value:

```yaml
plugins:
  aws-ce:
    carbon_enabled: true
```

Alternatively, export the environment variable before launching FinFocus or the
plugin:

```sh
export FINFOCUS_AWS_CARBON_ENABLED=true
```

Restart the plugin after changing the setting. `true` and `false` are accepted
without regard to case or surrounding whitespace. An unset or blank variable
selects false; other values fail startup.

`GetPluginInfo.settings` always advertises `carbon_enabled`, its BOOL type,
environment mapping and false default. Enabled discovery adds
`HISTORICAL_CARBON` and `GetCarbonEmissions`. Neither discovery nor enabling
the feature accesses AWS. Monetary requests do not acquire a Sustainability
permission dependency.

## Request a report

The companion FinFocus implementation adds a dedicated command:

```sh
finfocus carbon actual --start 2026-08-01 --end 2026-09-01 --output json
```

Both dates must be the first day of a month. The start is inclusive and the end
exclusive. Default reporting uses the market-based total. Select the alternative
view explicitly, for example with `--method location-based` or `--scope 2`.
Queries accept account, region and service filters using exact AWS dimension
values. Consult command help for the filter flags.

The shared SDK and core changes must be available alongside the plugin to use
this command. Updating the plugin alone does not add a command to an older
FinFocus executable.

For direct clients, the [RPC reference](api.md#getcarbonemissions) describes the
typed request and record fields. A minimal JSON request is:

```json
{
  "start": "2026-08-01T00:00:00Z",
  "end": "2026-09-01T00:00:00Z",
  "regions": ["eu-west-1"]
}
```

Grant `sustainability:GetEstimatedCarbonEmissions` to the selected AWS identity.
See [the credentials guide](CREDENTIALS.md#historical-carbon-permissions) for the
policy and account-visibility rules. The plugin uses the official synchronous
AWS API; console scraping and S3 exports are unnecessary.

## Interpret the report

Each record is one account/region/service/month aggregate. The original AWS
monthly interval, model version and selected method/scope remain attached to
the value. Values are estimates in grams of CO2 equivalent (`gCO2e`), converted
from AWS metric tons (`MTCO2e`). A metric ton equals 1000000 grams.

Market-based and location-based figures are alternative accounting views. Scope
totals and their individual scope components overlap. Compare these views
separately; summing them would double-count emissions. Totals cover only the
returned disjoint records for the selected method and scope.

No row means no available data; it does not mean zero emissions. Only a value
explicitly returned by AWS is reported as zero. AWS rounding can produce zero
without establishing physical zero. Errors, missing quantities and malformed
records are not converted to zero-valued results.

AWS publishes carbon data for the previous usage month by the following month's
21st day. Reports do not provide real-time or resource-ARN emissions. Historical
values can change when AWS revises its methodology, so retain model versions
and retrieval timestamps when comparing exported reports. The plugin does not
cache report data.

See [AWS's publication schedule](https://docs.aws.amazon.com/sustainability/latest/userguide/getting-started.html)
and [API reference](https://docs.aws.amazon.com/sustainability/latest/APIReference/API_GetEstimatedCarbonEmissions.html).

## Relationship to projected carbon

The existing [FinFocus carbon plugin](https://github.com/rshade/finfocus/tree/main/plugins/carbon)
provides offline modeled EC2 projections. Use it for infrastructure planning.
Historical AWS reports use customer usage aggregates and a different
methodology. Do not add these reports to projections for the same footprint,
or attach a complete service total to each individual resource.

## Development and release coordination

This feature spans the SDK, aws-ce and FinFocus core. Development checks can use
an external Go workspace containing all three feature checkouts; keep local
filesystem replacements out of releasable module files. The shared SDK changes
must be released and pinned in both consumers before standalone builds can use
the new RPC outside that workspace.

For three local feature checkouts, create a workspace outside those repositories
(replace the example checkout paths):

```sh
mkdir -p /tmp/finfocus-carbon-workspace
cd /tmp/finfocus-carbon-workspace
go work init /path/to/finfocus-spec /path/to/finfocus-plugin-aws-ce /path/to/finfocus
export GOWORK="$PWD/go.work"
go -C /path/to/finfocus-plugin-aws-ce build -o /tmp/finfocus-carbon-workspace/aws-ce ./cmd/plugin
go -C /path/to/finfocus build -o /tmp/finfocus-carbon-workspace/finfocus ./cmd/finfocus
```

Use the resulting core executable with an isolated plugin home when testing the
plugin binary. The normal installation instructions apply after the coordinated
SDK dependency pins are available.

Ordinary tests use offline fixtures. A request to build this feature does not
authorize real AWS verification.
