# Configuration reference

This guide covers the current plugin executable, the AWS credential chain, and
the repository's installation and verification settings. See the
[API reference](api.md) for request fields and [deployment guide](deployment.md)
for installation steps.

## Plugin settings

Set environment variables before starting the plugin or the FinFocus process
that launches it. `GetPluginInfo.settings` advertises the settings below.

| Variable | Default | Meaning |
| --- | --- | --- |
| `FINFOCUS_PLUGIN_PORT` | `0`, automatic port | Local gRPC port, unless `--port` overrides it |
| `FINFOCUS_LOG_LEVEL` | `info` | Entry-point log level: `trace`, `debug`, `info`, `warn`, `error`, `fatal`, or `panic` |
| `FINFOCUS_LOG_FILE` | Unset, standard error | Absolute filename for SDK log output |
| `FINFOCUS_AWS_CE_MAX_BATCH_SIZE` | `100` | Maximum resources per batch, integer from 1 to 1000 |
| `FINFOCUS_AWS_CE_BATCH_WORKERS` | `10` | Concurrent batch workers, integer from 1 to 50 |
| `FINFOCUS_AWS_CE_MAX_REQUESTS_PER_MINUTE` | `0`, unlimited | Cost Explorer attempts and Budgets pages allowed in a rolling minute |
| `FINFOCUS_AWS_CE_SPOT_COMPARISON` | `false` | `true` answers `GetProjectedCost` for an EC2 instance that has a type and a zone with a Spot quote. Off returns `InvalidArgument` so another plugin can price it |
| `FINFOCUS_HEALTH_ENDPOINT` | `false` | Serve `/healthz` on the plugin listener and enable Connect |
| `FINFOCUS_WEB_ENABLED` | `false` | Serve Connect and gRPC-Web alongside gRPC |
| `FINFOCUS_CORS_ALLOWED_ORIGINS` | Empty | Comma-separated browser origins. Empty allows none |
| `FINFOCUS_CORS_ALLOW_CREDENTIALS` | `false` | Allow credentialed cross-origin requests with explicit origins |
| `FINFOCUS_CORS_ALLOWED_HEADERS` | SDK defaults | Comma-separated allowed request headers |
| `FINFOCUS_AWS_ACCOUNT_ID` | Empty | Budgets account id. Empty uses STS `GetCallerIdentity` |

### Ports and command-line flags

| Flag | Meaning |
| --- | --- |
| `--port N` | Select a port from 0 to 65535. Zero requests an automatic port |
| `--help` | Print the Go flag help and exit |

An explicit `--port` always wins over the environment, including `--port 0`.
An unset environment port, a value that isn't an integer, or a value less than
one resolves to an automatic port. A numeric environment port greater than
65535 stops startup. Invalid command-line values also stop startup with a
nonzero exit code.

The server binds to `127.0.0.1`. It prints `PORT=<assigned-port>` to standard
output so the host can discover its address. Keep standard output separate from
logs when launching the binary through another process.

### Logging

Logs use structured JSON. Log-level parsing ignores case and accepts `warning`
as an alias for `warn`. Unknown levels use `info`.

For a file in the current directory:

```bash
export FINFOCUS_LOG_FILE="$(pwd)/aws-ce.log"
export FINFOCUS_LOG_FILE_PERMISSIONS=0600
export FINFOCUS_LOG_LEVEL=debug
```

`FINFOCUS_LOG_FILE_PERMISSIONS` is an additional SDK setting. It controls the
creation mode for new log files, subject to the process file-creation mask.
The default is `0644`. Invalid octal values fall back to that default with a
warning. It doesn't change permissions on an existing file.

The SDK appends to the configured file. A relative path, missing parent
directory, or failure to open the file causes a diagnostic and fallback to
standard error. Create the parent directory before startup.

The entry point applies `FINFOCUS_LOG_LEVEL` to its own logger. SDK server and
calculator loggers have separate defaults, so this variable isn't a global
filter for every message. Calculator logs can still appear on standard error
when the SDK writes to a file.

### Batch and request limits

Empty batch settings select the defaults. Invalid values, including whitespace
or numbers outside the documented range, stop startup. The error identifies
the setting.

The request budget accepts a positive integer and trims surrounding whitespace.
Unset, zero, negative, or invalid integer values turn off the limit. Each Cost
Explorer attempt counts, including commitment discovery, pagination, and retries. Batch
workers share the same process budget. Reaching the limit returns
`ResourceExhausted` without waiting for capacity. Requests age out after one
minute. Forecast attempts and Budgets pages also consume the shared budget.
For Budgets, exceeding the local limit returns `Internal`. Cache hits don't
consume the budget, and STS requests aren't part of it.

For example:

```bash
export FINFOCUS_AWS_CE_MAX_BATCH_SIZE=50
export FINFOCUS_AWS_CE_BATCH_WORKERS=5
export FINFOCUS_AWS_CE_MAX_REQUESTS_PER_MINUTE=60
```

### Web transport, health, and browser access

`FINFOCUS_WEB_ENABLED=true` enables Connect and gRPC-Web on the same listener
as gRPC. `FINFOCUS_HEALTH_ENDPOINT=true` also enables Connect, regardless of
`FINFOCUS_WEB_ENABLED`, and adds `/healthz` on that port. There is no separate
health port or `/ready` endpoint. Connect mode doesn't run the gRPC trace-header
interceptor, so trace-header propagation differs from the default transport.

The health, web, and credentialed-origin switches accept `true` or `false`,
ignoring case, and trim surrounding whitespace. Empty values select `false`. Other
values stop startup.

Cross-origin resource sharing applies in web mode. Origin and header
lists trim whitespace around entries. Empty settings use their documented
defaults, but an empty entry between commas stops startup. A wildcard origin
can't accompany other origins or credentialed requests. The SDK rejects those
combinations at startup.

For a local browser client:

```bash
export FINFOCUS_WEB_ENABLED=true
export FINFOCUS_HEALTH_ENDPOINT=true
export FINFOCUS_CORS_ALLOWED_ORIGINS=http://localhost:3000
export FINFOCUS_CORS_ALLOW_CREDENTIALS=false
```

## AWS configuration

The plugin initializes its default Cost Explorer client on the first cost request.
Discovery calls don't validate credentials. Configure an AWS region even though
Cost Explorer accesses a global billing dataset.

| Variable | Purpose |
| --- | --- |
| `AWS_REGION` | AWS signing and client region, for example `us-east-1` |
| `AWS_DEFAULT_REGION` | AWS SDK fallback when `AWS_REGION` is empty |
| `AWS_PROFILE` | Named profile from the shared AWS files |
| `AWS_ACCESS_KEY_ID` | Access key, paired with `AWS_SECRET_ACCESS_KEY` |
| `AWS_SECRET_ACCESS_KEY` | Secret key matching the access key |
| `AWS_SESSION_TOKEN` | Session token for temporary credentials |
| `AWS_SHARED_CREDENTIALS_FILE` | Override the shared credentials file location |
| `AWS_CONFIG_FILE` | Override the shared configuration file location |
| `AWS_ROLE_ARN`, `AWS_WEB_IDENTITY_TOKEN_FILE` | Role and token file for web identity credentials |
| `AWS_ROLE_SESSION_NAME` | Optional session name for web identity |
| `AWS_CONTAINER_CREDENTIALS_RELATIVE_URI`, `AWS_CONTAINER_CREDENTIALS_FULL_URI` | Workload-provided container credential endpoints |
| `AWS_CONTAINER_AUTHORIZATION_TOKEN`, `AWS_CONTAINER_AUTHORIZATION_TOKEN_FILE` | Authorization for a container credential endpoint |
| `AWS_EC2_METADATA_DISABLED` | Set to `true` to turn off EC2 instance metadata credentials |

The AWS SDK owns these settings. It also supports profile-based role assumption,
single sign-on, container credentials, and EC2 instance roles. This table lists
the relevant credential and region inputs, not every AWS SDK transport option.
Workload platforms normally supply their own credential endpoint variables.

For a local profile:

```bash
export AWS_PROFILE=default
export AWS_REGION=us-east-1
```

For temporary environment credentials, obtain the key, secret, and token through
your credential manager before starting FinFocus. See
[AWS credentials](CREDENTIALS.md) for host-supplied credentials and failure
handling. Use [the deployment policy](deployment.md#grant-aws-permissions) for
the required Cost Explorer permissions.

### Host-supplied credentials

The host can supply `access_key_id` and `secret_access_key` together, an optional
`session_token`, and an optional `role_arn`. A role alone uses the default AWS
chain as the source for STS. Names ignore case. The host uses the SDK's
credential metadata transport, not `GetActualCostRequest.tags`.

Unsupported names, incomplete pairs, whitespace-only values, and malformed role
ARNs return `InvalidArgument` with `ERROR_CODE_INVALID_CREDENTIALS`. Invalid
credentials never fall back to process credentials. Each valid credential-bearing
actual-cost or forecast call creates its own client and bypasses the shared
cache. For actual costs and forecasts, region configuration still comes from
the process or AWS profile. `GetBudgets` uses the process credential chain instead
of host-supplied credentials, with region `us-east-1`. Its account comes from
`FINFOCUS_AWS_ACCOUNT_ID` or STS.

## Cache behavior

The executable uses an in-memory cache scoped to one calculator instance. It
doesn't read or write the historical disk-cache directory. Restarting the
process clears cached costs.

| Result | Expiry |
| --- | --- |
| AWS marks any contributing row as an estimate | 15 minutes |
| Request ends within the last 48 hours | 15 minutes |
| Older history without estimates | 24 hours |

Cache keys include query identity, linked-account filter, and normalized
Coordinated Universal Time API dates. Request tags don't change the key.
Each result includes `expires_at`.
There is no executable setting for cache location, expiry, or manual invalidation.
Host-supplied credentials bypass cache reads and writes. Forecasts and budgets
don't use this cache.

## Legacy aliases and unavailable settings

The SDK accepts these fallback names when the corresponding preferred name is
empty. Use the `FINFOCUS_` names for new installations.

| Preferred name | Fallback order |
| --- | --- |
| `FINFOCUS_PLUGIN_PORT` | `PULUMICOST_PLUGIN_PORT` |
| `FINFOCUS_LOG_LEVEL` | `PULUMICOST_LOG_LEVEL`, then `LOG_LEVEL` |
| `FINFOCUS_LOG_FILE` | `PULUMICOST_LOG_FILE` |
| `FINFOCUS_LOG_FILE_PERMISSIONS` | `PULUMICOST_LOG_FILE_PERMISSIONS` |

Use `FINFOCUS_HEALTH_ENDPOINT` for health checks. The executable doesn't read
`FINFOCUS_PLUGIN_HEALTH_ENDPOINT` or `FINFOCUS_PLUGIN_HEALTH_PORT`, and it
doesn't expose a separate HTTP readiness probe. It doesn't use
`FINFOCUS_LOG_FORMAT` to select a different log format. `FINFOCUS_TEST_MODE`
doesn't replace the Cost Explorer client with fake data. Use the offline test
targets to verify behavior without contacting AWS.

## Installation and verification settings

These settings belong to the Makefile or test harness, not plugin request
configuration.

| Variable | Purpose |
| --- | --- |
| `FINFOCUS_HOME` | Registry root for installation, default `~/.finfocus` |
| `PLUGIN_VERSION` | Installed version directory, default from `manifest.json` |
| `FINFOCUS_CORE_VERSION` | Immutable core revision for `make core-install`, default in the Makefile |
| `FINFOCUS_CORE_BINARY` | Core executable for E2E tests, default `bin/core/finfocus` in Make targets |
| `FINFOCUS_E2E` | Set to `true` to enable the real-core offline test |
| `finfocus_E2E` | Legacy fallback when `FINFOCUS_E2E` is unset |
| `FINFOCUS_AWS_LIVE` | Explicit live-test opt-in, used with the `awslive` build tag |

See [offline verification](deployment.md#verify-the-installation) and
[live AWS verification](AWS-VERIFICATION.md). The offline harness configures
synthetic credentials and local AWS endpoints itself.
