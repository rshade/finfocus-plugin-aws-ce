# Deployment guide

Install `aws-ce` as a local plugin for FinFocus. The core discovers the executable
in its registry, starts the process, and reads its local gRPC port. The plugin
reads billing data through AWS credentials available to that process or supplied
by the host for a request.

Use the [configuration reference](configuration.md) for environment settings and
the [API reference](api.md) when integrating another host.

## Build and install from source

Install Git, Make, and the Go version declared in [Go module file](../go.mod):
Go 1.27.2. From a shell:

```bash
git clone https://github.com/rshade/finfocus-plugin-aws-ce.git
cd finfocus-plugin-aws-ce
make build
make install-local
```

The build creates `bin/finfocus-plugin-aws-ce`. Installation uses the version
from [plugin manifest](../manifest.json) and copies the binary into
`~/.finfocus/plugins/aws-ce/<version>/`. `make install` is an alias for
`make install-local`.

To use a separate registry, export the same root before installing and running
core:

```bash
export FINFOCUS_HOME="$HOME/.finfocus-testing"
make install-local
finfocus plugin list
```

`PLUGIN_VERSION` can override the installation directory's version. It doesn't
change the version embedded in the executable. Use the source build for changes
that aren't part of a published release.

## Install a release archive

Choose a version and download its archive plus `checksums.txt` from the
[repository releases](https://github.com/rshade/finfocus-plugin-aws-ce/releases).
Select the operating system and processor architecture of the machine that runs
FinFocus.

| Platform | Archive label | Format | Executable |
| --- | --- | --- | --- |
| Linux | `Linux_x86_64` or `Linux_arm64` | `.tar.gz` | `finfocus-plugin-aws-ce` |
| macOS | `Darwin_x86_64` or `Darwin_arm64` | `.tar.gz` | `finfocus-plugin-aws-ce` |
| Windows | `Windows_x86_64` or `Windows_arm64` | `.zip` | `finfocus-plugin-aws-ce.exe` |

Archive names follow
`finfocus-plugin-aws-ce_<version>_<platform-label>.<format>`.
The version in an archive name omits the release tag's leading `v`.

After downloading both files into the same directory, verify the archive's
digest against its line in `checksums.txt`. On Linux, calculate it with:

```bash
sha256sum finfocus-plugin-aws-ce_*.tar.gz
```

On macOS, use `shasum -a 256 <archive>`. On Windows, use
`Get-FileHash <archive> -Algorithm SHA256` in PowerShell. Stop if the digest
doesn't match.

Extract the archive and copy its executable into
`<registry-root>/plugins/aws-ce/<version>/`, with execute permission on Linux or
macOS. The default registry root is `~/.finfocus`. For example, after extracting
an archive on Linux or macOS, replace `X.Y.Z` with its version:

```bash
PLUGIN_RELEASE_VERSION=X.Y.Z
PLUGIN_REGISTRY_ROOT="${FINFOCUS_HOME:-$HOME/.finfocus}"
mkdir -p "$PLUGIN_REGISTRY_ROOT/plugins/aws-ce/$PLUGIN_RELEASE_VERSION"
install -m 0755 ./finfocus-plugin-aws-ce \
  "$PLUGIN_REGISTRY_ROOT/plugins/aws-ce/$PLUGIN_RELEASE_VERSION/finfocus-plugin-aws-ce"
```

On Windows, copy `finfocus-plugin-aws-ce.exe` into the corresponding directory
under your configured FinFocus registry.

## Grant AWS permissions

Enable AWS Cost Explorer for the billing account. Querying individual
resources also requires Cost Explorer resource-level data opt-in.

Grant these actions to the identity that performs the queries:

```json
{
  "Version": "2012-10-17",
  "Statement": [
    {
      "Effect": "Allow",
      "Action": [
        "ce:GetDimensionValues",
        "ce:GetCostAndUsage",
        "ce:GetCostAndUsageWithResources",
        "ce:GetCostForecast",
        "budgets:ViewBudget"
      ],
      "Resource": "*"
    }
  ]
}
```

`ce:GetDimensionValues` supports reservation and Savings Plan discovery for
every actual-cost query. `ce:GetCostAndUsage` supplies account and service totals.
`ce:GetCostAndUsageWithResources` supplies resource costs.
`ce:GetCostForecast` supplies projections, and `budgets:ViewBudget` permits
reading budgets. Grant only the operations the installation needs. Budget
queries use the process credentials and the account in `FINFOCUS_AWS_ACCOUNT_ID`,
or call STS `GetCallerIdentity` when that setting is empty.

For host-supplied `role_arn` credentials on actual-cost or forecast requests,
the source identity also needs
`sts:AssumeRole`, and the target role needs an appropriate trust policy and the
Cost Explorer permissions. See [AWS credentials](CREDENTIALS.md).

## Configure authentication and startup

For an existing local AWS profile:

```bash
export AWS_PROFILE=default
export AWS_REGION=us-east-1
finfocus plugin list
finfocus plugin validate --plugin aws-ce
```

The AWS SDK also accepts environment credentials and workload identities.
Temporary credentials require the access key, secret key, and session token.
Configure a region even though Cost Explorer uses a global billing dataset. Select the profile or workload identity that can see the costs
you intend to query.

Normally, core starts the installed plugin as needed. To inspect startup from a
source checkout:

```bash
./bin/finfocus-plugin-aws-ce --port 0
```

Wait for `PORT=<assigned-port>` on standard output. The server binds to
`127.0.0.1` and accepts gRPC requests. Logs use standard error unless a configured
logger writes to a file. `--port 50051` selects a fixed port. An explicit flag
overrides `FINFOCUS_PLUGIN_PORT`.

Stop the process with Ctrl+C or SIGTERM. Shutdown stops the server gracefully.
The process cache clears on exit. Successful startup and plugin discovery
don't prove AWS access because the client initializes on the first cost request.

## Containers and Kubernetes

This repository ships executable archives. It has no Dockerfile,
`make docker-build` target, published container image, or maintained Kubernetes
deployment manifest.

The executable listens on `127.0.0.1`. Docker port publishing doesn't make
that local listener reachable from outside its container. Enable
`FINFOCUS_HEALTH_ENDPOINT=true` for `/healthz` on the same port. This also
enables Connect mode. There is no separate health port, `/health`, or `/ready`
endpoint. The settings `FINFOCUS_PLUGIN_HEALTH_ENDPOINT` and
`FINFOCUS_PLUGIN_HEALTH_PORT` don't configure health checks.

A probe must reach the process through its local listener. A Kubernetes HTTP
probe to the pod address can't reach a listener bound only to `127.0.0.1`.
See [web and health settings](configuration.md#web-transport-health-and-browser-access)
for the supported transport options and trace-header limitation.

If you package FinFocus in your own container, install the plugin in the registry
visible to core and let core launch it in the same container. Supply credentials
through your workload's AWS credential chain. A separate network service needs
additional deployment and transport support beyond the executable interface.

## Verify the installation

Use `finfocus plugin list` and `finfocus plugin validate --plugin aws-ce` to
check discovery. From a source checkout, verify protocol behavior without AWS
credentials or live billing requests:

```bash
make test-integration
make core-install
make e2e
make conformance
```

`make core-install` downloads and builds the immutable core revision in the
Makefile. To use an existing compatible executable instead, set
`FINFOCUS_CORE_BINARY` to its absolute path and omit that step.

The E2E suite uses real core, an isolated plugin registry, synthetic credentials,
and a local fake Cost Explorer endpoint. It checks account and service totals,
empty results, and denied requests. Conformance checks validate the plugin's
advertised capabilities. These checks don't establish live-account permissions
or billing-data availability.

For an actual query, follow the [quick start](QUICKSTART.md) with an existing
resource and recent dates. Live requests can incur AWS charges. Use
[live AWS verification](AWS-VERIFICATION.md) for the separately gated test
procedure. Setting `FINFOCUS_TEST_MODE` alone doesn't make a live query offline.

## Troubleshooting

| Symptom | Action |
| --- | --- |
| Core doesn't find `aws-ce` | Check the versioned registry path, executable permission, and matching `FINFOCUS_HOME` |
| Address already in use | Choose another port or use `--port 0` |
| No startup port announcement | Read standard error for port, batch-setting, or listener failures |
| Missing AWS region | Set `AWS_REGION=us-east-1` or configure the selected profile's region |
| `PermissionDenied` | Check all actions needed by the query, including `ce:GetDimensionValues` |
| `Unauthenticated` | Refresh the AWS session and include its session token |
| `ERROR_CODE_INVALID_CREDENTIALS` | Check the supported host credential keys, complete key pair, and role ARN |
| `NotFound` for an EC2 instance | Check resource opt-in, instance identity, lookback, and data availability |
| `ERROR_CODE_INVALID_TIME_RANGE` | Use a start within 14 days for resources or 14 months for totals, and an end after the start |
| Non-EC2 resource has no costs | Check the ARN. These resource identity forms lack live verification |
| `ResourceExhausted` | Reduce batch concurrency or wait for the local or AWS rate limit to reset |
| Unexpectedly old costs | Check cache expiry and AWS billing-data delay |
| `NotFound` for projected costs | AWS returned no forecast rows for the next 30 days |
| Budget account unavailable | Set `FINFOCUS_AWS_ACCOUNT_ID` or check process credentials for STS account lookup |

Cost Explorer data can lag by 24 hours or longer, with resource-level data taking
up to 48 hours. Newly created resources may have no billed usage. Missing data
returns an error rather than a successful zero-cost result.

For errors that persist, include the plugin version, query scope, dates, and
sanitized logs in an issue. Remove credentials and sensitive billing details
before sharing logs.
