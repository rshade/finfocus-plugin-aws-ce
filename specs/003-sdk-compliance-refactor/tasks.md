# Tasks: SDK Compliance Refactor

**Input**: Design documents from `/specs/003-sdk-compliance-refactor/`
**Prerequisites**: plan.md, spec.md, research.md

**Tests**: No new test tasks - existing tests will be updated as part of implementation to match new SDK validation error messages.

**Organization**: Tasks are grouped by user story. Note that US1 (Startup Config) and US2 (Logging) both modify `main.go`, so they are combined into a single implementation phase to avoid file conflicts.

## Format: `[ID] [P?] [Story] Description`

- **[P]**: Can run in parallel (different files, no dependencies)
- **[Story]**: Which user story this task belongs to (e.g., US1, US2)
- Include exact file paths in descriptions

## Path Conventions

Per plan.md, this is a single-binary Go plugin:

- `cmd/plugin/main.go` - Plugin entry point
- `internal/pricing/calculator.go` - Cost calculation and RPC handlers
- `internal/pricing/calculator_test.go` - Calculator tests
- `internal/client/client.go` - AWS Cost Explorer client

---

## Phase 1: Setup

**Purpose**: Verify current state and prepare for refactoring

- [x] T001 Run `make test` to establish baseline test results
- [x] T002 Run `make lint` to verify current lint status
- [x] T003 Search codebase for `log.Fatal` and `os.Exit` calls to identify all removal targets

---

## Phase 2: Foundational (Blocking Prerequisites)

**Purpose**: No new foundation needed - this is a refactoring feature. Existing structure is reused.

**Note**: Skip to User Story phases. The SDK helpers are already available in `finfocus-spec v0.5.2`.

---

## Phase 3: User Story 1 & 2 - Plugin Startup & Logging (Priority: P1) 🎯 MVP

**Goal**: Plugin reads configuration from SDK environment variables and CLI flags, uses SDK logger

**Why Combined**: US1 (Startup Configuration) and US2 (Standardized Logging) both modify `cmd/plugin/main.go` - combining prevents file conflicts

**Independent Test**: Start plugin with `PULUMICOST_PLUGIN_PORT=50051 PULUMICOST_LOG_LEVEL=debug ./bin/pulumicost-plugin-aws-ce --port 50052` and verify:

1. Plugin binds to port 50052 (CLI flag takes precedence)
2. Debug-level logs appear with structured JSON format

### Implementation for User Stories 1 & 2

- [x] T004 [US1] Add `flag` package import and `flag.Parse()` call in cmd/plugin/main.go
- [x] T005 [US1] Replace hardcoded port with `pluginsdk.ParsePortFlag()` and `pluginsdk.GetPort()` in cmd/plugin/main.go
- [x] T006 [US2] Add `parseLogLevel()` helper function to convert string to zerolog.Level in cmd/plugin/main.go
- [x] T007 [US2] Initialize logger using `pluginsdk.NewLogWriter()` and `pluginsdk.NewPluginLogger()` in cmd/plugin/main.go
- [x] T008 [US1] Update `pluginsdk.ServeConfig` to use dynamically configured port in cmd/plugin/main.go
- [x] T009 [US2] Replace `log.Printf` startup message with SDK logger in cmd/plugin/main.go

**Checkpoint**: Plugin starts with SDK environment variables and CLI flags working

---

## Phase 4: User Story 4 - Graceful Shutdown (Priority: P2)

**Goal**: Plugin shuts down gracefully without `os.Exit` or `log.Fatal` calls

**Why Before US3**: Graceful shutdown modifies `main.go` (same file as US1/2), validation is in `calculator.go`

**Independent Test**: Start plugin, send SIGTERM, verify clean exit with no panic or abrupt termination

### Implementation for User Story 4

- [x] T010 [US4] Replace `log.Fatalf` with `logger.Error().Err(err).Msg()` + return in cmd/plugin/main.go
- [x] T011 [US4] Verify signal handling uses context cancellation (already implemented) in cmd/plugin/main.go
- [x] T012 [US4] Audit internal/client/client.go for any `log.Fatal` or `os.Exit` calls and remove if found

**Checkpoint**: No `log.Fatal` or `os.Exit` calls remain in codebase (verify with `grep -r`)

---

## Phase 5: User Story 3 - Request Validation (Priority: P2)

**Goal**: Plugin uses SDK validation helpers for consistent error messages

**Independent Test**: Send `GetActualCost` request with missing `resource_id` and verify error message matches SDK format: "resource_id is required"

### Implementation for User Story 3

- [x] T013 [US3] Add `pluginsdk.ValidateActualCostRequest()` call at start of `GetActualCost()` in internal/pricing/calculator.go
- [x] T014 [US3] Add gRPC status error wrapping for validation errors in internal/pricing/calculator.go
- [x] T015 [US3] Remove redundant manual `resourceID == ""` check (now handled by SDK) in internal/pricing/calculator.go
- [x] T016 [US3] Retain 14-month lookback validation (AWS-specific, not in SDK) in internal/pricing/calculator.go
- [x] T016a [US3] Audit `parseCostResults()` for nil pointer handling per FR-015 in internal/client/client.go
- [x] T016b [US3] Verify zero-value handling in cost amount parsing per FR-014 in internal/client/client.go
- [x] T017 [US2] [US3] Add `pluginsdk.LogOperation()` timing to `GetActualCost()` in internal/pricing/calculator.go
- [x] T018 [US2] [US3] Add `pluginsdk.LogOperation()` timing to `GetServiceActualCost()` in internal/pricing/calculator.go
- [x] T019 [US2] [US3] Add `pluginsdk.LogOperation()` timing to `GetAccountActualCost()` in internal/pricing/calculator.go

**Checkpoint**: Validation errors match SDK format, all RPC operations have timing logs

---

## Phase 6: Test Updates

**Purpose**: Update existing tests to expect new SDK validation error messages

- [x] T020 Update tests expecting "invalid request: ResourceId is required" to expect "resource_id is required" in internal/pricing/calculator_test.go (N/A - no such tests exist)
- [x] T021 Update tests expecting custom time range error to expect `ErrActualCostTimeRangeInvalid` message in internal/pricing/calculator_test.go (N/A - no such tests exist)
- [x] T022 Run `make test` to verify all tests pass with new validation messages

**Checkpoint**: All existing tests pass with refactored code ✓

---

## Phase 7: Polish & Documentation

**Purpose**: Complete documentation and final verification

- [x] T023 [P] Update CLAUDE.md with SDK helper usage patterns per FR-016
- [x] T024 [P] Update README.md with environment variables and CLI flags per FR-017
- [x] T025 Run `make lint` to verify no new lint warnings (0 issues)
- [x] T026 Run `make build` to verify successful compilation
- [x] T027 Run `grep -r "log.Fatal\|os.Exit" cmd/ internal/` to verify SC-003 (no forbidden calls)
- [x] T028 Run quickstart.md validation scenarios manually (manual testing deferred to user)

---

## Dependencies & Execution Order

### Phase Dependencies

- **Setup (Phase 1)**: No dependencies - establishes baseline
- **Foundational (Phase 2)**: Skipped - not needed for refactoring
- **US1 & US2 (Phase 3)**: Depends on Setup - MVP phase
- **US4 (Phase 4)**: Depends on US1 & US2 (same file)
- **US3 (Phase 5)**: Can start after Setup (different file), but sequenced after US4 for clarity
- **Test Updates (Phase 6)**: Depends on US3 completion
- **Polish (Phase 7)**: Depends on all user stories complete

### User Story Dependencies

- **User Story 1 (Startup Config)**: No dependencies - core configuration
- **User Story 2 (Logging)**: Coupled with US1 (same file `main.go`)
- **User Story 4 (Graceful Shutdown)**: Depends on US1/US2 (modifies `main.go` logging)
- **User Story 3 (Validation)**: Independent (`calculator.go`) but sequenced after `main.go` work

### Parallel Opportunities

Limited due to file overlap:

- T001, T002, T003 can run in parallel (Setup phase)
- T023, T024 can run in parallel (different files)

---

## Parallel Example: Setup Phase

```bash
# Launch all setup verification tasks together:
Task T001: "Run make test to establish baseline"
Task T002: "Run make lint to verify current lint status"
Task T003: "Search codebase for log.Fatal and os.Exit calls"
```

---

## Parallel Example: Polish Phase

```bash
# Launch documentation updates together:
Task T023: "Update CLAUDE.md with SDK helper usage patterns"
Task T024: "Update README.md with environment variables and CLI flags"
```

---

## Implementation Strategy

### MVP First (User Stories 1 & 2 Only)

1. Complete Phase 1: Setup (verify baseline)
2. Complete Phase 3: US1 & US2 (startup + logging)
3. **STOP and VALIDATE**: Test with environment variables and CLI flags
4. Plugin is now SDK-compliant for configuration and logging

### Incremental Delivery

1. Complete Setup → Baseline established
2. Add US1 & US2 (startup/logging) → Test configuration → Deploy (MVP!)
3. Add US4 (graceful shutdown) → Test signal handling → Deploy
4. Add US3 (validation) → Test error messages → Deploy
5. Update tests and docs → Final validation → Complete

### Sequential Execution (Recommended)

Due to file overlap, sequential execution is recommended:

1. T001-T003: Setup (parallel)
2. T004-T009: US1 & US2 in order (main.go)
3. T010-T012: US4 in order (main.go + audit)
4. T013-T016b: US3 in order (calculator.go + client.go robustness audit)
5. T017-T019: US3 LogOperation (calculator.go)
6. T020-T022: Test updates (sequential - same file)
7. T023-T028: Polish (T023, T024 parallel; rest sequential)

---

## Notes

- This is a refactoring feature - no new data models or API contracts
- Most tasks modify the same files, limiting parallelization
- 14-month lookback validation is AWS-specific and retained (not in SDK)
- Existing test framework (`pluginsdk.NewTestPlugin`) is preserved
- Commit after each phase for clean rollback points
