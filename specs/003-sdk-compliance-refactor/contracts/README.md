# API Contracts: SDK Compliance Refactor

**Date**: 2025-12-16
**Feature**: 003-sdk-compliance-refactor

## Overview

This feature is a **refactoring task** that does not introduce new API endpoints or modify existing gRPC contracts.

## Existing Contracts (Unchanged)

The gRPC service contract is defined in `pulumicost-spec` and remains unchanged:

- `CostSourceService.Name()` - Returns plugin name
- `CostSourceService.Supports()` - Checks resource support
- `CostSourceService.GetProjectedCost()` - Returns error (not supported)
- `CostSourceService.GetActualCost()` - Retrieves historical costs

## Why No New Contracts

The SDK compliance refactoring affects:

1. **Internal implementation** - How the plugin initializes, logs, and validates
2. **Configuration surface** - Environment variables and CLI flags
3. **Error message formatting** - Standardized SDK error messages

None of these changes affect the gRPC wire protocol or message schemas.

## Contract Testing

Existing contract tests in `internal/pricing/calculator_test.go` will be updated to verify:

- SDK validation errors match expected format
- Response structures remain unchanged
- Error codes use proto-defined enum values
