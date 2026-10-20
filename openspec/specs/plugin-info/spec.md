# Plugin info

## Purpose

Describe the identity, provider, capability, and RPC metadata exposed to plugin clients.

## Requirements

### Requirement: Advertise plugin identity and capabilities

The plugin SHALL report name `aws-ce`, its build version (defaulting to
`0.1.0` without release linker flags), spec version `v0.7.8`, AWS as its
provider, and its supported capabilities and RPC metadata through
`GetPluginInfo`. The default version is covered by `TestGetPluginInfo` and
`TestGetPluginInfo_GRPC` in `internal/pricing/plugin_info_test.go`; the release
version is covered by `TestReleaseVersionInMetadataAndLogs` in
`cmd/plugin/version_test.go`.

#### Scenario: Plugin discovery

- **WHEN** a caller invokes `GetPluginInfo` through gRPC
- **THEN** the response identifies the plugin and advertises its supported capabilities and RPCs
