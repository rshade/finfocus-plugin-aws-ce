# Quick Start: AWS Cost Explorer Plugin for PulumiCost

**Plugin Name**: aws-ce
**Purpose**: Retrieve actual historical billing data from AWS Cost Explorer

## Prerequisites

1. **AWS Account** with Cost Explorer enabled
2. **AWS Credentials** configured (environment variables, shared credentials file, or IAM roles)
3. **IAM Permissions**:
   ```json
   {
     "Version": "2012-10-17",
     "Statement": [
       {
         "Effect": "Allow",
         "Action": [
           "ce:GetCostAndUsage",
           "ce:GetCostForecast",
           "ce:GetReservationCoverage",
           "ce:GetReservationPurchaseRecommendation",
           "ce:GetReservationUtilization"
         ],
         "Resource": "*"
       }
     ]
   }
   ```

## Installation

### Option 1: Build from Source
```bash
git clone <repository-url>
cd pulumicost-plugin-aws-ce
make build
make install
```

### Option 2: Download Binary
```bash
# Download from releases page
# Place binary in ~/.pulumicost/plugins/aws-ce/1.0.0/
```

## Configuration

### AWS Credentials (choose one method)

**Environment Variables**:
```bash
export AWS_ACCESS_KEY_ID=your-access-key
export AWS_SECRET_ACCESS_KEY=your-secret-key
export AWS_DEFAULT_REGION=us-east-1
```

**Shared Credentials File** (`~/.aws/credentials`):
```ini
[default]
aws_access_key_id = your-access-key
aws_secret_access_key = your-secret-key
region = us-east-1
```

**IAM Roles** (for EC2/ECS/EKS):
- Attach the IAM policy above to your instance/role
- No additional configuration needed

## Usage Examples

### Basic Cost Query
```go
// Query costs for the last 30 days for an EC2 instance
resource := &pluginsdk.ResourceDescriptor{
    Provider: "aws",
    Type:     "aws:ec2/instance",
    Id:       "i-1234567890abcdef0",
}

response, err := plugin.GetActualCost(ctx, &pluginsdk.GetActualCostRequest{
    Resource:  resource,
    StartTime: time.Now().AddDate(0, 0, -30),
    EndTime:   time.Now(),
})
```

### Cost Query with Dimensions
```go
// Query costs grouped by service and region
response, err := plugin.GetActualCost(ctx, &pluginsdk.GetActualCostRequest{
    Resource:   resource,
    StartTime:  time.Now().AddDate(0, 0, -30),
    EndTime:    time.Now(),
    Dimensions: []string{"SERVICE", "REGION"},
})
```

### Service-Level Cost Query
```go
// Get costs for EC2 service across all accounts
response, err := plugin.GetServiceActualCost(ctx, &pluginsdk.GetServiceActualCostRequest{
    ServiceCode: "Amazon Elastic Compute Cloud - Compute",
    StartTime:   time.Now().AddDate(0, -1, 0), // Last month
    EndTime:     time.Now(),
})
```

### Account-Level Cost Query
```go
// Get costs for specific AWS account
response, err := plugin.GetAccountActualCost(ctx, &pluginsdk.GetAccountActualCostRequest{
    AccountId: "123456789012",
    StartTime: time.Now().AddDate(0, -3, 0), // Last 3 months
    EndTime:   time.Now(),
})
```

## Response Format

### Successful Response
```json
{
  "entries": [
    {
      "timestamp": "2024-12-01T00:00:00Z",
      "amount": "45.67",
      "currency": "USD",
      "service": "Amazon Elastic Compute Cloud - Compute",
      "account_id": "123456789012",
      "region": "us-east-1",
      "tags": {
        "Environment": "production",
        "Team": "backend"
      }
    }
  ],
  "error_code": "ERROR_CODE_UNSPECIFIED",
  "fallback_hint": "FALLBACK_HINT_NONE"
}
```

### No Data Response
```json
{
  "entries": [],
  "error_code": "ERROR_CODE_NO_DATA",
  "error_message": "No cost data available for the specified period",
  "fallback_hint": "FALLBACK_HINT_RECOMMENDED"
}
```

## Common Issues

### Authentication Failed
**Error**: `AWS authentication failed: NoCredentialProviders`
**Solution**: Configure AWS credentials using one of the methods above

### Cost Explorer Not Enabled
**Error**: `AWS Cost Explorer must be enabled for this account`
**Solution**: Enable Cost Explorer in AWS Console → Billing → Cost Explorer

### Rate Limit Exceeded
**Behavior**: Plugin automatically retries with exponential backoff
**Solution**: Wait for retry, or reduce query frequency

### Date Range Too Large
**Error**: `Date range exceeds AWS Cost Explorer limits`
**Solution**: Limit queries to maximum 14 months historical data

## Performance Tips

1. **Use Caching**: The plugin caches results to improve performance for repeated queries
2. **Limit Date Ranges**: Smaller date ranges (1-3 months) return faster
3. **Batch Queries**: Group similar resources in single requests when possible
4. **Off-Peak Hours**: AWS Cost Explorer may be faster during off-peak AWS usage hours

## Monitoring

The plugin logs structured JSON to stderr. Monitor for:

- `level=warn` messages indicating slow API calls (>5 seconds)
- `level=error` messages for failed requests
- Cache hit/miss ratios in debug logs

## Troubleshooting

### Enable Debug Logging
```bash
export PULUMICOST_LOG_LEVEL=debug
```

### Check Plugin Status
```bash
# Verify plugin is installed
ls -la ~/.pulumicost/plugins/aws-ce/1.0.0/

# Check plugin binary
./pulumicost-plugin-aws-ce --help
```

### Test AWS Credentials
```bash
aws sts get-caller-identity
aws ce get-cost-and-usage --time-period Start=2024-12-01,End=2024-12-02 --granularity=DAILY --metrics=BlendedCost
```

## Support

- **Issues**: GitHub repository issues
- **Documentation**: Full API reference in `/specs/001-aws-ce-plugin/contracts/`
- **Logs**: Check plugin stderr output for detailed error information</content>
<parameter name="filePath">/mnt/c/GitHub/go/src/github.com/rshade/pulumicost-plugin-aws-ce/specs/001-aws-ce-plugin/quickstart.md