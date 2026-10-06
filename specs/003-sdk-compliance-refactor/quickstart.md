# Quickstart: SDK Compliance Refactor

**Date**: 2025-12-16
**Feature**: 003-sdk-compliance-refactor

## Overview

After this refactoring, the AWS Cost Explorer plugin supports standard FinFocus configuration via environment variables and CLI flags. This guide documents the new configuration options and usage patterns.

## Configuration

### Environment Variables

Configure the plugin using standard FinFocus environment variables:

```bash
# Required: AWS credentials (standard AWS SDK chain)
export AWS_REGION=us-east-1
export AWS_ACCESS_KEY_ID=your-key
export AWS_SECRET_ACCESS_KEY=your-secret

# Optional: Plugin configuration
export FINFOCUS_PLUGIN_PORT=50051        # Specific port (default: auto-assign)
export FINFOCUS_LOG_FILE=/var/log/finfocus-aws-ce.log  # Log to file (default: stderr)
export FINFOCUS_LOG_LEVEL=debug          # Verbosity: debug|info|warn|error (default: info)
```

### CLI Flags

The `--port` flag overrides the environment variable:

```bash
# Use environment variable port
./finfocus-plugin-aws-ce

# Override with CLI flag (takes precedence)
./finfocus-plugin-aws-ce --port 50052
```

## Running the Plugin

### Basic Usage

```bash
# Start with default configuration
./bin/finfocus-plugin-aws-ce

# Start with specific port and debug logging
FINFOCUS_LOG_LEVEL=debug ./bin/finfocus-plugin-aws-ce --port 50051
```

### With Log File

```bash
# Log to file for production use
export FINFOCUS_LOG_FILE=/var/log/finfocus/aws-ce.log
export FINFOCUS_LOG_LEVEL=info
./bin/finfocus-plugin-aws-ce
```

### Graceful Shutdown

The plugin now responds cleanly to shutdown signals:

```bash
# Start plugin in background
./bin/finfocus-plugin-aws-ce &
PID=$!

# Send SIGTERM for graceful shutdown
kill -TERM $PID
# Plugin completes in-flight requests before exiting
```

## Error Handling

### Validation Errors

Invalid requests now return standardized SDK error messages:

| Error | Message |
| ----- | ------- |
| Missing resource_id | "resource_id is required" |
| Missing start_time | "start_time is required" |
| Invalid time range | "end_time must be strictly after start_time" |

### Startup Errors

Startup failures no longer call `os.Exit`. Instead, errors are logged and the process exits gracefully:

```text
{"level":"error","component":"finfocus-plugin-aws-ce","error":"bind: address already in use","message":"Failed to serve plugin"}
```

## Log Output Format

Logs use structured JSON format with standard fields:

```json
{
  "level": "info",
  "component": "finfocus-plugin-aws-ce",
  "plugin_name": "aws-ce",
  "plugin_version": "1.0.0",
  "operation": "GetActualCost",
  "duration_ms": 1234,
  "message": "Operation completed"
}
```

### Operation Timing

All RPC operations are automatically logged with timing:

```json
{"level":"debug","operation":"GetActualCost","message":"Starting operation"}
{"level":"debug","operation":"GetActualCost","duration_ms":1523,"message":"Operation completed"}
```

## Integration with FinFocus Core

The plugin now integrates seamlessly with FinFocus Core orchestration:

1. Core sets `FINFOCUS_PLUGIN_PORT` for consistent port assignment
2. Core sets `FINFOCUS_LOG_FILE` to aggregate plugin logs
3. Core sets `FINFOCUS_TRACE_ID` for distributed tracing (if enabled)

## Testing

### Verify SDK Compliance

```bash
# Build and run tests
make build
make test

# Verify no os.Exit or log.Fatal calls
grep -r "os.Exit\|log.Fatal" cmd/ internal/
# Should return no matches
```

### Manual Verification

```bash
# Start plugin with debug logging
FINFOCUS_LOG_LEVEL=debug ./bin/finfocus-plugin-aws-ce --port 50051

# In another terminal, send test request via grpcurl
grpcurl -plaintext localhost:50051 finfocus.v1.CostSourceService/Name
```

## Migration Notes

If you were using the plugin before this refactoring:

1. **No breaking changes** to the gRPC API
2. **New configuration options** are all optional with sensible defaults
3. **Log format** now uses JSON by default (previously plain text in some cases)
4. **Validation errors** now use standardized messages

The plugin remains backward-compatible; existing deployments will continue to work without configuration changes.
