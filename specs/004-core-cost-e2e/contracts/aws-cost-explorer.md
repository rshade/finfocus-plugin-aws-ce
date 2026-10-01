# Contract: AWS Cost Explorer API Integration

**Feature**: 004-core-cost-e2e
**Date**: 2026-01-21
**Status**: Design

This document defines the integration contract between the PulumiCost AWS CE plugin and the AWS Cost Explorer API.

## API Endpoint

**Service**: AWS Cost Explorer (ce)
**Region**: Global service (can be called from any region)
**SDK**: `github.com/aws/aws-sdk-go-v2/service/costexplorer`

## Authentication

**Method**: AWS SDK credential chain

**Resolution order:**
1. Environment variables (`AWS_ACCESS_KEY_ID`, `AWS_SECRET_ACCESS_KEY`, `AWS_SESSION_TOKEN`)
2. Shared credentials file (`~/.aws/credentials`)
3. IAM role (for EC2 instances, ECS tasks, Lambda functions)
4. GitHub OIDC federation (for CI/CD)

**Required permissions:**
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

## API Operations

### GetCostAndUsage

**Purpose**: Retrieve historical cost data for resources.

**Request:**
```go
input := &costexplorer.GetCostAndUsageInput{
    TimePeriod: &types.DateInterval{
        Start: aws.String("2024-01-01"),  // YYYY-MM-DD format
        End:   aws.String("2024-01-31"),
    },
    Granularity: types.GranularityDaily,  // DAILY, MONTHLY, HOURLY
    Metrics: []string{
        "UnblendedCost",  // Actual costs without discounts applied
    },
    Filter: &types.Expression{
        Dimensions: &types.DimensionValues{
            Key:    types.DimensionResourceId,
            Values: []string{"i-12345"},
        },
    },
}
```

**Response:**
```go
type GetCostAndUsageOutput struct {
    ResultsByTime []types.ResultByTime
    // Additional fields...
}

type ResultByTime struct {
    TimePeriod struct {
        Start *string  // "2024-01-01"
        End   *string  // "2024-01-02"
    }
    Total map[string]struct {
        Amount *string  // "2.45"
        Unit   *string  // "USD"
    }
    Groups []types.Group  // Optional: when grouped by dimensions
}
```

**Rate limits:**
- 5 requests per second per account
- 100 requests per account per day for Cost Anomaly Detection APIs (not used)
- GetCostAndUsage has no documented daily limit

**Latency:**
- Typical: 2-5 seconds
- With grouping: 5-10 seconds
- Large date ranges (1+ year): 10-30 seconds

**Data freshness:**
- Cost data updated 3 times per day (approximately 8-hour lag)
- Data for current day is preliminary and subject to change
- Finalized data available after ~48 hours

## Error Handling

### Retryable Errors

| Error Code | HTTP Status | Description | Retry Strategy |
|------------|-------------|-------------|----------------|
| `ThrottlingException` | 429 | Rate limit exceeded | Exponential backoff |
| `RequestTimeout` | 504 | Request took too long | Retry with same params |
| (No code) | 500 | Internal server error | Retry with backoff |
| (No code) | 503 | Service unavailable | Retry with backoff |

### Non-Retryable Errors

| Error Code | HTTP Status | Description | Action |
|------------|-------------|-------------|--------|
| `ValidationException` | 400 | Invalid request parameters | Fix request, do not retry |
| `AccessDeniedException` | 403 | Permission denied | Check IAM policy |
| `InvalidParameterException` | 400 | Parameter value invalid | Fix parameter value |
| `UnauthorizedOperation` | 401 | Not authorized | Check credentials |

### Network Errors

| Error Type | Retry | Notes |
|------------|-------|-------|
| DNS timeout | Yes | Temporary DNS resolution failure |
| Connection refused | Yes | AWS endpoint temporarily unavailable |
| Connection reset | Yes | Network interruption |
| TLS handshake failure | No | Certificate or configuration issue |

## Filter Expressions

### By ResourceId

```go
filter := &types.Expression{
    Dimensions: &types.DimensionValues{
        Key:    types.DimensionResourceId,
        Values: []string{"i-12345"},
    },
}
```

**Example ResourceId values:**
- EC2 instance: `i-12345`
- RDS instance: `db-ABC123DEF456`
- S3 bucket: `my-bucket-name`
- Lambda function: `my-function-name`

### By Service

```go
filter := &types.Expression{
    Dimensions: &types.DimensionValues{
        Key: types.DimensionService,
        Values: []string{
            "Amazon Elastic Compute Cloud - Compute",
            "Amazon Relational Database Service",
        },
    },
}
```

**Common service names:**
- EC2: `"Amazon Elastic Compute Cloud - Compute"`
- RDS: `"Amazon Relational Database Service"`
- S3: `"Amazon Simple Storage Service"`
- Lambda: `"AWS Lambda"`

### By Region

```go
filter := &types.Expression{
    Dimensions: &types.DimensionValues{
        Key:    types.DimensionRegion,
        Values: []string{"us-east-1"},
    },
}
```

### Combined Filters (AND)

```go
filter := &types.Expression{
    And: []*types.Expression{
        {
            Dimensions: &types.DimensionValues{
                Key:    types.DimensionService,
                Values: []string{"Amazon Elastic Compute Cloud - Compute"},
            },
        },
        {
            Dimensions: &types.DimensionValues{
                Key:    types.DimensionRegion,
                Values: []string{"us-east-1"},
            },
        },
    },
}
```

## Grouping

**Purpose**: Aggregate costs by dimensions (Service, Region, LinkedAccount).

```go
input := &costexplorer.GetCostAndUsageInput{
    // ... (TimePeriod, Granularity, Metrics)
    GroupBy: []types.GroupDefinition{
        {
            Type: types.GroupDefinitionTypeDimension,
            Key:  aws.String("SERVICE"),
        },
        {
            Type: types.GroupDefinitionTypeDimension,
            Key:  aws.String("REGION"),
        },
    },
}
```

**Response structure:**
```go
type ResultByTime struct {
    Groups []types.Group
}

type Group struct {
    Keys []string  // ["Amazon Elastic Compute Cloud - Compute", "us-east-1"]
    Metrics map[string]struct {
        Amount *string
        Unit   *string
    }
}
```

## Pagination

**Note**: Cost Explorer API does not use traditional pagination (no NextToken).

**Date range handling:**
- Maximum: 12 months of data per request
- If requesting > 12 months, split into multiple requests
- Plugin limitation: E2E tests restricted to 7-day queries

## Cost Data Completeness

### Partial Data Scenarios

| Scenario | AWS Behavior | Plugin Response |
|----------|--------------|-----------------|
| Resource created mid-period | Returns data from creation date | Include available data, log warning |
| Resource deleted mid-period | Returns data until deletion | Include available data |
| No cost data (free tier) | Returns empty results | Return empty with `FALLBACK_HINT_RECOMMENDED` |
| Date range in future | Returns empty results | Return empty with warning |
| Data not yet finalized | Returns preliminary values | Include data, note freshness in logs |

### Data Lag

**Current day:**
- Data available but preliminary
- Subject to change as usage data arrives
- Typically 8-12 hour lag

**Previous days:**
- Data mostly finalized after 24 hours
- Fully finalized after 48 hours
- May have minor adjustments for up to 3 days

## Performance Characteristics

### Query Cost

- Free for up to 1,000 queries per month
- $0.01 per query after first 1,000
- Recommendation: Cache aggressively (24-hour TTL appropriate)

### Latency Targets

| Query Type | Target | Notes |
|------------|--------|-------|
| Single resource, 7 days | < 3s | Typical use case |
| Single resource, 30 days | < 5s | Monthly cost report |
| Account-level, 7 days | < 8s | Multiple resources aggregated |
| With grouping (2+ dimensions) | < 10s | Additional processing overhead |

**Timeout recommendations:**
- Per-request: 30 seconds
- E2E test overall: 2 minutes (per spec)

## Example Integration

```go
package client

import (
    "context"
    "github.com/aws/aws-sdk-go-v2/aws"
    "github.com/aws/aws-sdk-go-v2/config"
    "github.com/aws/aws-sdk-go-v2/service/costexplorer"
    "github.com/aws/aws-sdk-go-v2/service/costexplorer/types"
)

type Client struct {
    ceClient *costexplorer.Client
}

func NewClient(ctx context.Context, region string) (*Client, error) {
    cfg, err := config.LoadDefaultConfig(ctx, config.WithRegion(region))
    if err != nil {
        return nil, err
    }

    return &Client{
        ceClient: costexplorer.NewFromConfig(cfg),
    }, nil
}

func (c *Client) GetCostForResource(ctx context.Context, resourceID string, start, end string) (float64, error) {
    input := &costexplorer.GetCostAndUsageInput{
        TimePeriod: &types.DateInterval{
            Start: aws.String(start),
            End:   aws.String(end),
        },
        Granularity: types.GranularityDaily,
        Metrics: []string{"UnblendedCost"},
        Filter: &types.Expression{
            Dimensions: &types.DimensionValues{
                Key:    types.DimensionResourceId,
                Values: []string{resourceID},
            },
        },
    }

    output, err := c.ceClient.GetCostAndUsage(ctx, input)
    if err != nil {
        return 0, err
    }

    // Aggregate costs across all time periods
    var totalCost float64
    for _, result := range output.ResultsByTime {
        if cost, ok := result.Total["UnblendedCost"]; ok {
            amount, _ := strconv.ParseFloat(*cost.Amount, 64)
            totalCost += amount
        }
    }

    return totalCost, nil
}
```

## References

- [AWS Cost Explorer API Reference](https://docs.aws.amazon.com/aws-cost-management/latest/APIReference/API_GetCostAndUsage.html)
- [AWS Cost Management User Guide](https://docs.aws.amazon.com/cost-management/latest/userguide/ce-api.html)
- [AWS SDK for Go v2 - Cost Explorer](https://pkg.go.dev/github.com/aws/aws-sdk-go-v2/service/costexplorer)
- [Best Practices for AWS Cost Explorer API](https://docs.aws.amazon.com/cost-management/latest/userguide/ce-api-best-practices.html)
