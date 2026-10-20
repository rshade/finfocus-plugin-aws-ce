# Supports

## Purpose

Describe how callers learn whether an AWS resource is supported by this plugin.

## Requirements

### Requirement: Report support through the RPC

The plugin SHALL return its resource support decision through `Supports`,
including a reason for supported EC2 instance queries limited to 14 days.
Covered by `TestSupports` and `TestSupports_GRPC` in
`internal/pricing/supports_test.go` and `TestSupports_LogTraceID` in
`internal/pricing/trace_test.go`.

#### Scenario: EC2 instance support

- **WHEN** a caller asks whether an AWS EC2 instance is supported
- **THEN** the plugin returns a support decision and its applicable time-range reason
