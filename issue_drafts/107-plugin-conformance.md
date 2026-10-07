# Plugin conformance testing, issue 107

**Status:** planned
**Type:** technical debt and quality
**Priority:** high

## User story

A plugin maintainer needs to verify compliance with the `finfocus-spec` contract to ensure interoperability with the core engine.

## Technical approach

Integrate the official plugin conformance test suite from `github.com/rshade/finfocus-spec/sdk/go/conformance`.

### Implementation plan

1. Create `cmd/conformance/main.go` or `conformance_test.go`.
2. Import the conformance suite.
3. Run the suite against the `aws-ce` plugin binary.
4. Add a `make conformance` target.

## Constraints

1. Fix plugin code when a conformance test fails. Don't change the specification tests.
2. Tests must pass with a mock AWS client. Basic conformance doesn't require live AWS calls.

## Acceptance criteria

- [ ] `make conformance` runs the official test suite.
- [ ] All mandatory compliance tests pass.
- [ ] Continuous integration runs conformance tests on every pull request.
