# Feature Specification: SDK Compliance Refactor

**Feature Branch**: `003-sdk-compliance-refactor`
**Created**: 2025-12-16
**Status**: Draft
**Input**: GitHub Issue #6 - Update Dependencies & Refactor for SDK Compliance

## Clarifications

### Session 2025-12-16

- Q: How should the plugin handle backward compatibility when migrating to SDK helpers? → A: Strict SDK compliance - Plugin behavior matches SDK exactly; any previous non-standard behavior is considered a bug fix.

## User Scenarios & Testing *(mandatory)*

### User Story 1 - Plugin Startup Configuration (Priority: P1)

As an operator deploying the AWS Cost Explorer plugin, I want the plugin to respect standard PulumiCost environment variables and CLI flags so that I can configure it consistently with other plugins in my infrastructure.

**Why this priority**: Plugin startup configuration is foundational - without proper environment variable and flag handling, the plugin cannot integrate correctly with the PulumiCost Core orchestration system.

**Independent Test**: Can be fully tested by starting the plugin with various environment variable combinations and verifying it initializes correctly with the expected configuration.

**Acceptance Scenarios**:

1. **Given** the `PULUMICOST_PLUGIN_PORT` environment variable is set, **When** the plugin starts, **Then** it binds to the specified port.
2. **Given** the `--port` CLI flag is provided, **When** the plugin starts, **Then** it binds to the port specified by the flag (overriding environment variable if both set).
3. **Given** neither port configuration is provided, **When** the plugin starts, **Then** it binds to an available system-assigned port.

---

### User Story 2 - Standardized Logging (Priority: P1)

As an operator troubleshooting plugin behavior, I want logs written to a configurable file location with consistent formatting so that I can correlate plugin activity with the Core system logs.

**Why this priority**: Logging is essential for operational visibility and debugging. Without standardized logging, operators cannot effectively diagnose issues in production.

**Independent Test**: Can be fully tested by setting `PULUMICOST_LOG_FILE` and `PULUMICOST_LOG_LEVEL`, then verifying log output appears in the specified file with appropriate detail levels.

**Acceptance Scenarios**:

1. **Given** `PULUMICOST_LOG_FILE` is set to a valid path, **When** the plugin logs messages, **Then** logs are written to that file.
2. **Given** `PULUMICOST_LOG_LEVEL` is set to "debug", **When** the plugin processes requests, **Then** detailed debug-level messages are logged.
3. **Given** `PULUMICOST_LOG_LEVEL` is set to "error", **When** the plugin processes requests successfully, **Then** only error-level messages appear (no info/debug noise).

---

### User Story 3 - Request Validation (Priority: P2)

As a developer integrating with the plugin, I want consistent and descriptive error messages for invalid requests so that I can quickly identify and fix integration issues.

**Why this priority**: Proper validation improves developer experience and reduces support burden. It enables faster debugging of integration problems.

**Independent Test**: Can be fully tested by sending malformed requests and verifying standardized error responses that match the SDK validation patterns.

**Acceptance Scenarios**:

1. **Given** a `GetActualCost` request with missing time range, **When** the request is processed, **Then** a standardized validation error is returned indicating the missing field.
2. **Given** a `GetActualCost` request with end time before start time, **When** the request is processed, **Then** a clear error is returned explaining the invalid time range.

---

### User Story 4 - Graceful Shutdown (Priority: P2)

As an operator managing plugin lifecycle, I want the plugin to shut down cleanly without abrupt process termination so that in-flight requests complete and resources are properly released.

**Why this priority**: Graceful shutdown prevents data corruption, ensures clean resource cleanup, and allows proper integration with container orchestration systems.

**Independent Test**: Can be fully tested by sending SIGINT/SIGTERM signals during request processing and verifying the plugin completes current work before exiting.

**Acceptance Scenarios**:

1. **Given** the plugin is processing requests, **When** SIGINT is received, **Then** current requests complete before shutdown.
2. **Given** the plugin is idle, **When** SIGTERM is received, **Then** the plugin exits without calling `os.Exit` directly (uses context cancellation instead).
3. **Given** an unrecoverable error occurs during startup, **When** the error is detected, **Then** the error is returned from main (not logged with Fatal which calls os.Exit).

---

### Edge Cases

- What happens when `PULUMICOST_LOG_FILE` points to a non-writable path?
  - The plugin logs an error to stderr and continues with console-only logging.
- What happens when port specified by `--port` is already in use?
  - The plugin returns a clear error and exits gracefully (no panic or os.Exit).
- What happens when log level is set to an invalid value?
  - The plugin defaults to "info" level and logs a warning about the unrecognized level.

## Requirements *(mandatory)*

### Functional Requirements

**Environment Variables**

- **FR-001**: Plugin MUST read port configuration from `PULUMICOST_PLUGIN_PORT` environment variable using SDK helpers.
- **FR-002**: Plugin MUST read log file path from `PULUMICOST_LOG_FILE` environment variable using SDK helpers.
- **FR-003**: Plugin MUST read log level from `PULUMICOST_LOG_LEVEL` environment variable using SDK helpers.

**CLI Flags**

- **FR-004**: Plugin MUST support `--port` CLI flag for specifying the gRPC server port.
- **FR-005**: CLI flag values MUST take precedence over environment variable values when both are specified.

**Logging**

- **FR-006**: Plugin MUST use `pluginsdk.NewPluginLogger()` for creating the main logger instance.
- **FR-007**: Plugin MUST use `pluginsdk.NewLogWriter()` to configure log output destination.
- **FR-008**: Plugin MUST use `pluginsdk.LogOperation()` for timing and logging RPC operations.

**Validation**

- **FR-009**: Plugin MUST use `pluginsdk.ValidateActualCostRequest()` for validating incoming actual cost requests.
- **FR-010**: Plugin MUST return SDK-standard validation errors (e.g., `ErrActualCostTimeRangeInvalid`) rather than custom error messages.

**Safety**

- **FR-011**: Plugin MUST NOT use `log.Fatal()` or `os.Exit()` in any code path.
- **FR-012**: Plugin MUST use context cancellation and error returns for shutdown signaling.
- **FR-013**: Plugin MUST handle startup errors by returning from main, not by calling Fatal.

**Robustness**

- **FR-014**: Plugin MUST handle zero/missing numeric values gracefully without panics.
- **FR-015**: Plugin MUST handle nil pointer access in cost parsing without crashing.

**Documentation**

- **FR-016**: `CLAUDE.md` MUST be updated with new SDK dependency version and helper usage patterns.
- **FR-017**: `README.md` MUST document environment variables and CLI flags for configuration.

### Assumptions

- The project already uses `finfocus-spec v0.5.2` which includes all required SDK helpers.
- Existing zerolog usage will be migrated to SDK-provided logger initialization for consistency.
- The SDK validation helpers cover the validation cases currently implemented with custom logic.
- **Strict SDK compliance**: Any behavioral differences between current implementation and SDK helpers are treated as bug fixes, not breaking changes. No backward-compatibility shims will be added.

## Success Criteria *(mandatory)*

### Measurable Outcomes

- **SC-001**: Plugin starts successfully with any combination of `PULUMICOST_PLUGIN_PORT`, `PULUMICOST_LOG_FILE`, and `PULUMICOST_LOG_LEVEL` environment variables.
- **SC-002**: All RPC operations are logged with timing information using `LogOperation()` pattern.
- **SC-003**: No occurrences of `log.Fatal`, `log.Fatalf`, or `os.Exit` exist in the codebase.
- **SC-004**: Invalid requests return SDK-standard error codes that match other PulumiCost plugins.
- **SC-005**: Plugin passes all existing tests after refactoring without regression.
- **SC-006**: `go build` completes without errors after dependency updates.
- **SC-007**: `make lint` passes without warnings related to the refactored code.
