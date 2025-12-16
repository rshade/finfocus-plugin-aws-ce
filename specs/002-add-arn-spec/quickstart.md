# Quickstart: Add ARN to GetActualCostRequest

**Feature**: 002-add-arn-spec
**Estimated Effort**: 2-3 hours (excluding upstream PR review time)

## Prerequisites

- Go 1.25.5+
- Access to `rshade/pulumicost-spec` repository (for upstream PR)
- AWS credentials configured (for integration testing)

## Implementation Sequence

### Step 1: Upstream Spec Change (pulumicost-spec repo)

```bash
# Clone spec repo
git clone https://github.com/rshade/pulumicost-spec.git
cd pulumicost-spec
git checkout -b feat/add-arn-field

# Edit proto file
# Add to proto/pulumicost/v1/costsource.proto:
#   string arn = 5;  // in GetActualCostRequest

# Regenerate Go code
make generate

# Commit and push
git add .
git commit -m "feat(proto): add arn field to GetActualCostRequest"
git push origin feat/add-arn-field

# Create PR, wait for merge and release
```

### Step 2: Update Plugin Dependency

```bash
# In pulumicost-plugin-aws-ce repo
go get github.com/rshade/pulumicost-spec@v0.4.8  # or new version
go mod tidy
```

### Step 3: Implement ARN Parsing

Create `internal/pricing/arn.go`:

```go
package pricing

import (
    "fmt"
    "strings"
)

// ParsedARN contains components extracted from an AWS ARN.
type ParsedARN struct {
    Partition string
    Service   string
    Region    string
    AccountID string
    Resource  string
}

// ParseARN parses an AWS ARN string into components.
func ParseARN(arn string) (*ParsedARN, error) {
    if arn == "" {
        return nil, nil  // Empty ARN is valid (optional field)
    }

    parts := strings.SplitN(arn, ":", 6)
    if len(parts) < 6 || parts[0] != "arn" {
        return nil, fmt.Errorf("invalid ARN format: %s", arn)
    }

    return &ParsedARN{
        Partition: parts[1],
        Service:   parts[2],
        Region:    parts[3],
        AccountID: parts[4],
        Resource:  parts[5],
    }, nil
}
```

### Step 4: Update GetActualCost Handler

Modify `internal/pricing/calculator.go`:

```go
func (c *Calculator) GetActualCost(ctx context.Context, req *pbc.GetActualCostRequest) (*pbc.GetActualCostResponse, error) {
    resourceID := req.GetResourceId()
    arn := req.GetArn()  // NEW: Get ARN field

    // NEW: Validate and resolve identifiers
    lookupID := c.resolveIdentifier(resourceID, arn)

    // ... rest of implementation uses lookupID
}

func (c *Calculator) resolveIdentifier(resourceID, arn string) string {
    if arn == "" {
        return resourceID
    }

    parsed, err := ParseARN(arn)
    if err != nil {
        c.logger.Warn().Err(err).Str("arn", arn).Msg("Invalid ARN, using resource_id")
        return resourceID
    }

    if !strings.Contains(parsed.Resource, resourceID) && resourceID != "" {
        c.logger.Warn().
            Str("resource_id", resourceID).
            Str("arn_resource", parsed.Resource).
            Msg("Identifier mismatch: using ARN as source of truth")
    }

    return parsed.Resource
}
```

### Step 5: Add Tests

Create `internal/pricing/arn_test.go`:

```go
func TestParseARN(t *testing.T) {
    tests := []struct {
        name    string
        arn     string
        want    *ParsedARN
        wantErr bool
    }{
        {
            name: "valid EC2 instance ARN",
            arn:  "arn:aws:ec2:us-east-1:123456789012:instance/i-1234567890abcdef0",
            want: &ParsedARN{
                Partition: "aws",
                Service:   "ec2",
                Region:    "us-east-1",
                AccountID: "123456789012",
                Resource:  "instance/i-1234567890abcdef0",
            },
        },
        {
            name: "empty ARN returns nil",
            arn:  "",
            want: nil,
        },
        {
            name:    "invalid format",
            arn:     "not-an-arn",
            wantErr: true,
        },
    }
    // ... table-driven test implementation
}
```

### Step 6: Run Validation

```bash
make lint
make test
make build
```

## Verification Checklist

- [x] Upstream spec PR merged and released
- [x] Plugin dependency updated to new spec version
- [x] `ParseARN` function implemented with tests
- [x] `GetActualCost` uses ARN when available
- [x] Mismatch detection logs warning
- [x] All existing tests pass (backward compatibility)
- [x] `make lint` passes
- [x] `make test` passes
