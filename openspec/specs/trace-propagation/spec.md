# Trace propagation

## Purpose

Describe tested trace identifier fields in structured operation logs.

## Requirements

### Requirement: Include available trace IDs in operation logs

The plugin SHALL include a trace ID from context in `Supports`, `GetPluginInfo`,
and `GetActualCost` logs, and SHALL omit that field when no trace ID exists.
Covered by `TestSupports_LogTraceID`, `TestGetPluginInfo_LogTraceID`,
`TestGetPluginInfo_LogOmitsTraceID`, `TestGetActualCost_LogTraceID`, and
`TestGetActualCost_LogOmitsTraceID` in `internal/pricing/trace_test.go`.

#### Scenario: Traced operation

- **WHEN** an operation receives a context containing a trace ID
- **THEN** its structured log contains that ID

#### Scenario: Operation without a trace ID

- **WHEN** an operation receives a context without a trace ID
- **THEN** its structured log omits the trace ID field
