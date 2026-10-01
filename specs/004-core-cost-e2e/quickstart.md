# Quickstart: Core Cost Plugin & E2E Testing

**Feature**: 004-core-cost-e2e
**Date**: 2026-01-21
**Audience**: Developers working on or testing the plugin

This guide provides step-by-step instructions for local development, E2E test execution, and troubleshooting.

## Prerequisites

- **Go**: 1.25.5 or later
- **AWS Account**: With Cost Explorer enabled and cost data available
- **AWS Credentials**: Configured locally (see [AWS Credentials Setup](#aws-credentials-setup))
- **Git**: For cloning the repository

## Quick Start

### 1. Clone and Build

```bash
# Clone repository
git clone https://github.com/rshade/finfocus-plugin-aws-ce.git
cd finfocus-plugin-aws-ce

# Install dependencies
make deps

# Build plugin binary
make build

# Verify build
./bin/finfocus-plugin-aws-ce --version
```

### 2. Run Unit Tests

```bash
# Run all unit tests (no AWS credentials required)
make test

# Run specific package tests
go test -v ./internal/pricing/

# Run with coverage
go test -cover ./...
```

### 3. Run E2E Tests

```bash
# Set up AWS credentials (see section below)
export AWS_PROFILE=your-profile

# Enable E2E tests and run
FINFOCUS_E2E=true make e2e

# Or run directly with timeout
FINFOCUS_E2E=true go test -v -timeout 2m30s ./test/e2e/
```

---

## AWS Credentials Setup

### Option 1: AWS Profile (Recommended for Local Development)

**Prerequisites**: AWS CLI installed

```bash
# Configure AWS CLI with your credentials
aws configure --profile your-profile
# Follow prompts to enter:
# - AWS Access Key ID
# - AWS Secret Access Key
# - Default region (e.g., us-east-1)
# - Output format (json)

# Verify configuration
aws sts get-caller-identity --profile your-profile

# Set profile for plugin
export AWS_PROFILE=your-profile

# Run E2E tests
FINFOCUS_E2E=true make e2e
```

### Option 2: Environment Variables

```bash
# Set credentials directly
export AWS_ACCESS_KEY_ID=AKIAIOSFODNN7EXAMPLE
export AWS_SECRET_ACCESS_KEY=wJalrXUtnFEMI/K7MDENG/bPxRfiCYEXAMPLEKEY
export AWS_DEFAULT_REGION=us-east-1

# Run E2E tests
FINFOCUS_E2E=true make e2e
```

### Option 3: IAM Role (For EC2/ECS/Lambda)

If running on AWS infrastructure with an attached IAM role:

```bash
# No explicit credentials needed - SDK will use instance metadata

# Run E2E tests
FINFOCUS_E2E=true make e2e
```

### Required IAM Permissions

Your AWS credentials must have these permissions:

```json
{
  "Version": "2012-10-17",
  "Statement": [
    {
      "Effect": "Allow",
      "Action": [
        "ce:GetCostAndUsage",
        "ce:GetCostForecast"
      ],
      "Resource": "*"
    }
  ]
}
```

---

## Running E2E Tests

### Basic Execution

```bash
# Run all E2E tests
FINFOCUS_E2E=true go test -v -timeout 2m30s ./test/e2e/

# Run specific test
FINFOCUS_E2E=true go test -v -run TestE2E_GetActualCost ./test/e2e/

# Run with detailed logging
FINFOCUS_E2E=true go test -v -timeout 2m30s ./test/e2e/ 2>&1 | tee e2e.log
```

### With Cache Bypass

```bash
# Force fresh data (bypass cache)
FINFOCUS_E2E=true FINFOCUS_CACHE_BYPASS=true go test -v ./test/e2e/
```

### Expected Output

**Successful run:**
```
=== RUN   TestE2E
=== RUN   TestE2E/GetActualCost
    e2e_test.go:123: Starting plugin server on port 50051
    e2e_test.go:145: Plugin server ready
    e2e_test.go:178: GetActualCost succeeded: received 7 cost records
=== RUN   TestE2E/GetProjectedCost
    e2e_test.go:202: GetProjectedCost correctly returned NotSupported error
--- PASS: TestE2E (8.45s)
    --- PASS: TestE2E/GetActualCost (5.23s)
    --- PASS: TestE2E/GetProjectedCost (0.12s)
PASS
ok      github.com/rshade/finfocus-plugin-aws-ce/test/e2e    8.465s
```

---

## Cache Debugging

### Inspect Cache Location

```bash
# Find cache directory (platform-specific)
## Linux/macOS
ls -lh ~/.cache/finfocus/aws-ce/

## Windows
dir %LocalAppData%\finfocus\aws-ce\

# View cache entry
cat ~/.cache/finfocus/aws-ce/cost_v1_i-12345___1704067200_1705276800.json | jq
```

### Clear Cache

```bash
# Remove all cache files
## Linux/macOS
rm -rf ~/.cache/finfocus/aws-ce/*.json

## Windows
del %LocalAppData%\finfocus\aws-ce\*.json

# Verify cleared
ls ~/.cache/finfocus/aws-ce/
```

### Test Cache Behavior

```bash
# First run (cache miss)
time FINFOCUS_E2E=true go test -v -run TestE2E_GetActualCost ./test/e2e/
# Note: ~5 seconds (AWS API call)

# Second run (cache hit)
time FINFOCUS_E2E=true go test -v -run TestE2E_GetActualCost ./test/e2e/
# Note: ~1 second (cache hit, much faster)

# Force refresh (bypass cache)
time FINFOCUS_E2E=true FINFOCUS_CACHE_BYPASS=true go test -v -run TestE2E_GetActualCost ./test/e2e/
# Note: ~5 seconds again (bypassed cache)
```

---

## Troubleshooting

### E2E Tests Skipped

**Symptom:**
```
=== RUN   TestE2E
    e2e_test.go:89: Skipping E2E tests. Set FINFOCUS_E2E=true to run.
--- SKIP: TestE2E (0.00s)
```

**Solution:**
```bash
# Ensure environment variable is set
export FINFOCUS_E2E=true

# Or prefix command
FINFOCUS_E2E=true go test ./test/e2e/
```

### AWS Credentials Not Found

**Symptom:**
```
Error: NoCredentialProviders: no valid providers in chain
```

**Solutions:**

1. **Check AWS CLI configuration:**
   ```bash
   aws configure list
   ```

2. **Verify credentials file:**
   ```bash
   cat ~/.aws/credentials
   ```

3. **Test credentials:**
   ```bash
   aws sts get-caller-identity
   ```

4. **Set environment variables:**
   ```bash
   export AWS_PROFILE=your-profile
   # OR
   export AWS_ACCESS_KEY_ID=...
   export AWS_SECRET_ACCESS_KEY=...
   ```

### Permission Denied Errors

**Symptom:**
```
AccessDeniedException: User: arn:aws:iam::123456789012:user/test is not authorized to perform: ce:GetCostAndUsage
```

**Solution:**

1. **Check IAM permissions:**
   ```bash
   aws iam get-user-policy --user-name test --policy-name CostExplorer
   ```

2. **Attach required policy:**
   ```bash
   aws iam put-user-policy --user-name test --policy-name CostExplorerAccess --policy-document file://cost-explorer-policy.json
   ```

3. **Verify permissions:**
   ```bash
   aws ce get-cost-and-usage --time-period Start=2024-01-01,End=2024-01-31 --granularity MONTHLY --metrics UnblendedCost
   ```

### No Cost Data Available

**Symptom:**
```
Test returned empty results (no cost data found)
```

**Causes:**
- AWS account has no cost data for queried period
- Free tier usage (no billable costs)
- Resource created very recently (< 8 hours ago)

**Solutions:**

1. **Check for any costs in AWS Console:**
   - Navigate to AWS Cost Explorer
   - Verify cost data exists for your account

2. **Query wider date range:**
   - Modify test to query last 30 days instead of 7 days

3. **Use test account with known costs:**
   - Spin up EC2 instance to generate costs
   - Wait 12-24 hours for data to appear

### E2E Tests Timeout

**Symptom:**
```
panic: test timed out after 2m0s
```

**Solutions:**

1. **Increase timeout:**
   ```bash
   FINFOCUS_E2E=true go test -v -timeout 5m ./test/e2e/
   ```

2. **Check network connectivity:**
   ```bash
   curl -I https://ce.us-east-1.amazonaws.com
   ```

3. **Verify AWS API status:**
   - Check [AWS Service Health Dashboard](https://health.aws.amazon.com/health/status)

### Build Failures

**Symptom:**
```
go: github.com/rshade/finfocus-spec@v0.5.2: no matching versions for query "v0.5.2"
```

**Solution:**
```bash
# Update dependencies
go get -u github.com/rshade/finfocus-spec@latest
go mod tidy

# Rebuild
make build
```

---

## Development Workflow

### 1. Make Changes

```bash
# Create feature branch
git checkout -b feature/my-changes

# Edit code
vim internal/pricing/calculator.go

# Format code
make fmt

# Run linter
make lint
```

### 2. Test Changes

```bash
# Run unit tests
make test

# Run E2E tests
FINFOCUS_E2E=true make e2e

# Run specific test
go test -v -run TestGetActualCost_WithCache ./internal/pricing/
```

### 3. Commit Changes

```bash
# Stage changes
git add .

# Commit with conventional commit message
git commit -m "feat: add cache invalidation support"

# Push to remote
git push origin feature/my-changes
```

---

## CI/CD Integration

### GitHub Actions Workflow

E2E tests run automatically in CI using GitHub OIDC federation:

```yaml
# .github/workflows/e2e.yml
name: E2E Tests

on:
  push:
    branches: [ "main" ]

permissions:
  id-token: write
  contents: read

jobs:
  e2e:
    runs-on: ubuntu-latest
    steps:
      - uses: actions/checkout@v4
      - uses: actions/setup-go@v5
        with:
          go-version: '1.25.5'

      - name: Configure AWS credentials (OIDC)
        uses: aws-actions/configure-aws-credentials@v4
        with:
          role-to-assume: arn:aws:iam::${{ secrets.AWS_ACCOUNT_ID }}:role/GitHubE2ERole
          aws-region: us-east-1

      - name: Run E2E tests
        run: FINFOCUS_E2E=true go test -v -timeout 2m30s ./test/e2e/
```

**For fork PRs:** E2E tests are skipped (no AWS credentials available by design).

---

## Performance Benchmarks

### Cache Performance

```bash
# Run cache benchmarks
go test -bench=BenchmarkCache -benchmem ./internal/pricing/

# Expected output:
# BenchmarkCacheSet-8         100000     15000 ns/op     2048 B/op     12 allocs/op
# BenchmarkCacheGet-8        1000000      1200 ns/op        0 B/op      0 allocs/op
```

### API Call Latency

Typical latencies (from E2E tests):

| Query Type | Latency | Notes |
|------------|---------|-------|
| 7-day, single resource | 2-3s | Typical use case |
| 7-day, account-level | 5-8s | Multiple resources |
| 30-day, single resource | 4-6s | Larger date range |
| With cache hit | < 100ms | In-memory cache |

---

## Useful Commands

```bash
# Build and install plugin locally
make install

# Run plugin standalone (for manual testing)
./bin/finfocus-plugin-aws-ce --port 50051

# View plugin logs (structured JSON)
./bin/finfocus-plugin-aws-ce --port 50051 2>&1 | jq

# Check cache statistics
ls -lh ~/.cache/finfocus/aws-ce/ | wc -l  # Count cache entries

# Monitor cache size
du -sh ~/.cache/finfocus/aws-ce/

# Tail logs during E2E tests
FINFOCUS_E2E=true go test -v ./test/e2e/ 2>&1 | grep "level=warn"
```

---

## Next Steps

After completing local development:

1. **Create Pull Request** following [Development Workflow](#development-workflow)
2. **Verify CI passes** (unit tests, linting, E2E tests)
3. **Update CLAUDE.md** with any new conventions or patterns discovered
4. **Update ROADMAP.md** if feature impacts future plans

## References

- [data-model.md](./data-model.md) - Data structures
- [contracts/aws-cost-explorer.md](./contracts/aws-cost-explorer.md) - AWS API contract
- [contracts/cache-invalidation.md](./contracts/cache-invalidation.md) - Cache behavior
- [research.md](./research.md) - Design decisions
