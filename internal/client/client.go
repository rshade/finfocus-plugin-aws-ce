// Package client provides AWS Cost Explorer API client implementation.
package client

import (
	"context"
	"fmt"
	"time"

	"github.com/aws/aws-sdk-go-v2/aws"
	"github.com/aws/aws-sdk-go-v2/config"
	"github.com/aws/aws-sdk-go-v2/service/costexplorer"
	"github.com/aws/aws-sdk-go-v2/service/costexplorer/types"
)

// CostExplorerAPI defines the interface for AWS Cost Explorer operations.
// This interface allows for mocking in tests.
type CostExplorerAPI interface {
	GetCostAndUsage(ctx context.Context, params *costexplorer.GetCostAndUsageInput, optFns ...func(*costexplorer.Options)) (*costexplorer.GetCostAndUsageOutput, error)
	GetReservationUtilization(ctx context.Context, params *costexplorer.GetReservationUtilizationInput, optFns ...func(*costexplorer.Options)) (*costexplorer.GetReservationUtilizationOutput, error)
	GetSavingsPlansCoverage(ctx context.Context, params *costexplorer.GetSavingsPlansCoverageInput, optFns ...func(*costexplorer.Options)) (*costexplorer.GetSavingsPlansCoverageOutput, error)
}

// Client represents a client for AWS Cost Explorer API.
type Client struct {
	ceClient CostExplorerAPI
	region   string
}

// Config holds configuration for the AWS Cost Explorer client.
type Config struct {
	// Region is the AWS region to use. If empty, uses default region from environment.
	Region string
	// Profile is the AWS profile to use. If empty, uses default profile.
	Profile string
}

// NewClient creates a new AWS Cost Explorer client.
func NewClient(ctx context.Context, cfg Config) (*Client, error) {
	var opts []func(*config.LoadOptions) error

	if cfg.Region != "" {
		opts = append(opts, config.WithRegion(cfg.Region))
	}

	if cfg.Profile != "" {
		opts = append(opts, config.WithSharedConfigProfile(cfg.Profile))
	}

	awsCfg, err := config.LoadDefaultConfig(ctx, opts...)
	if err != nil {
		return nil, fmt.Errorf("loading AWS config: %w", err)
	}

	ceClient := costexplorer.NewFromConfig(awsCfg)

	return &Client{
		ceClient: ceClient,
		region:   awsCfg.Region,
	}, nil
}

// NewClientWithAPI creates a new client with a custom Cost Explorer API implementation.
// This is primarily used for testing.
func NewClientWithAPI(api CostExplorerAPI, region string) *Client {
	return &Client{
		ceClient: api,
		region:   region,
	}
}

// CostResult represents the cost data for a resource.
type CostResult struct {
	// Amount is the cost amount in the specified currency.
	Amount float64
	// Currency is the currency code (e.g., "USD").
	Currency string
	// StartDate is the start of the time period.
	StartDate time.Time
	// EndDate is the end of the time period.
	EndDate time.Time
	// ServiceName is the AWS service name.
	ServiceName string
	// UsageType is the usage type for the cost.
	UsageType string
	// AccountID is the AWS account ID (for multi-account scenarios).
	AccountID string
	// Region is the AWS region.
	Region string
	// AvailabilityZone is the availability zone.
	AvailabilityZone string
	// Tags are the resource tags.
	Tags map[string]string
	// ReservationARN is the ARN of the reservation if applicable.
	ReservationARN string
	// SavingsPlanARN is the ARN of the savings plan if applicable.
	SavingsPlanARN string
}

// GetCost retrieves cost data with flexible filtering and grouping.
func (c *Client) GetCost(ctx context.Context, filter *types.Expression, dimensions []string, startTime, endTime time.Time, granularity string) ([]CostResult, error) {
	if granularity == "" {
		granularity = string(types.GranularityDaily)
	}

	var groupDefinitions []types.GroupDefinition
	for _, dim := range dimensions {
		if len(dim) > 4 && dim[:4] == "TAG:" {
			tagKey := dim[4:]
			groupDefinitions = append(groupDefinitions, types.GroupDefinition{
				Type: types.GroupDefinitionTypeTag,
				Key:  aws.String(tagKey),
			})
		} else {
			groupDefinitions = append(groupDefinitions, types.GroupDefinition{
				Type: types.GroupDefinitionTypeDimension,
				Key:  aws.String(dim),
			})
		}
	}

	// Default grouping if none provided
	if len(groupDefinitions) == 0 {
		groupDefinitions = append(groupDefinitions, types.GroupDefinition{
			Type: types.GroupDefinitionTypeDimension,
			Key:  aws.String("SERVICE"),
		})
	}

	var allResults []CostResult
	var nextPageToken *string

	for {
		input := &costexplorer.GetCostAndUsageInput{
			TimePeriod: &types.DateInterval{
				Start: aws.String(startTime.Format("2006-01-02")),
				End:   aws.String(endTime.Format("2006-01-02")),
			},
			Granularity:   types.Granularity(granularity),
			Metrics:       []string{"UnblendedCost", "UsageQuantity"},
			NextPageToken: nextPageToken,
			GroupBy:       groupDefinitions,
			Filter:        filter,
		}

		var output *costexplorer.GetCostAndUsageOutput
		output, err := WithRetry(ctx, DefaultRetryConfig(), func(ctx context.Context) (*costexplorer.GetCostAndUsageOutput, error, bool) {
			out, err := c.ceClient.GetCostAndUsage(ctx, input)
			if err != nil {
				return nil, err, isRetryableError(err)
			}
			return out, nil, false
		})

		if err != nil {
			return nil, fmt.Errorf("getting cost and usage: %w", err)
		}

		results, err := c.parseCostResults(output)
		if err != nil {
			return nil, fmt.Errorf("parsing cost results: %w", err)
		}
		allResults = append(allResults, results...)

		if output.NextPageToken == nil {
			break
		}
		nextPageToken = output.NextPageToken
	}

	return allResults, nil
}

// isRetryableError checks if an error should trigger a retry.
func isRetryableError(err error) bool {
	if err == nil {
		return false
	}
	// Basic check for throttling/rate limiting strings
	// In production, checking specific error types like types.LimitExceededException is better
	errMsg := err.Error()
	return contains(errMsg, "Throttling") || 
	       contains(errMsg, "RateExceeded") || 
		   contains(errMsg, "RequestLimitExceeded") ||
		   contains(errMsg, "LimitExceededException")
}

func contains(s, substr string) bool {
	return len(s) >= len(substr) && func() bool {
		for i := 0; i <= len(s)-len(substr); i++ {
			if s[i:i+len(substr)] == substr {
				return true
			}
		}
		return false
	}()
}

// GetResourceCost retrieves actual cost data for a specific resource or service.
func (c *Client) GetResourceCost(ctx context.Context, resourceID string, startTime, endTime time.Time, granularity string) ([]CostResult, error) {
	var filter *types.Expression
	if resourceID != "" {
		filter = &types.Expression{
			Dimensions: &types.DimensionValues{
				Key:    types.DimensionResourceId,
				Values: []string{resourceID},
			},
		}
	}
	// Default to grouping by SERVICE for resource cost
	return c.GetCost(ctx, filter, []string{"SERVICE"}, startTime, endTime, granularity)
}

// GetServiceCost retrieves cost data for a specific AWS service.
func (c *Client) GetServiceCost(ctx context.Context, serviceName string, startTime, endTime time.Time) ([]CostResult, error) {
	filter := &types.Expression{
		Dimensions: &types.DimensionValues{
			Key:    types.DimensionService,
			Values: []string{serviceName},
		},
	}
	return c.GetCost(ctx, filter, []string{"USAGE_TYPE"}, startTime, endTime, "DAILY")
}

// GetAccountCost retrieves total cost data for the AWS account.
func (c *Client) GetAccountCost(ctx context.Context, startTime, endTime time.Time) ([]CostResult, error) {
	return c.GetCost(ctx, nil, []string{"SERVICE"}, startTime, endTime, "DAILY")
}

// GetCostByTag retrieves cost data grouped by a specific tag.
func (c *Client) GetCostByTag(ctx context.Context, tagKey string, startTime, endTime time.Time) ([]CostResult, error) {
	return c.GetCost(ctx, nil, []string{"TAG:" + tagKey}, startTime, endTime, "DAILY")
}

// parseCostResults converts AWS Cost Explorer output to CostResult slice.
func (c *Client) parseCostResults(output *costexplorer.GetCostAndUsageOutput) ([]CostResult, error) {
	var results []CostResult

	for _, resultByTime := range output.ResultsByTime {
		startDate, err := time.Parse("2006-01-02", *resultByTime.TimePeriod.Start)
		if err != nil {
			return nil, fmt.Errorf("parsing start date: %w", err)
		}

		endDate, err := time.Parse("2006-01-02", *resultByTime.TimePeriod.End)
		if err != nil {
			return nil, fmt.Errorf("parsing end date: %w", err)
		}

		for _, group := range resultByTime.Groups {
			result := CostResult{
				StartDate: startDate,
				EndDate:   endDate,
				Tags:      make(map[string]string),
			}

			// Parse metrics
			if unblendedCost, ok := group.Metrics["UnblendedCost"]; ok {
				var amount float64
				if _, err := fmt.Sscanf(*unblendedCost.Amount, "%f", &amount); err == nil {
					result.Amount = amount
				}
				if unblendedCost.Unit != nil {
					result.Currency = *unblendedCost.Unit
				}
			}

			// Parse group keys
			// Keys depend on GroupBy. e.g. "SERVICE", "LINKED_ACCOUNT", "TAG:Name"
			// Usually [value] or [value, value] if multi-dim grouping
			for _, key := range group.Keys {
				// Simple heuristic mapping - this might need refinement based on exact GroupBy context
				// For now, we put everything in ServiceName as primary identifier if not parsed better
				if len(key) > 4 && key[:4] == "arn:" {
					result.ReservationARN = key // Heuristic for ARNs
				} else if len(key) == 12 && isNumeric(key) { // Simple check for account ID
					result.AccountID = key
				} else {
					result.ServiceName = key // Fallback
				}
			}

			results = append(results, result)
		}

		// Handle ungrouped results
		if len(resultByTime.Groups) == 0 && resultByTime.Total != nil {
			result := CostResult{
				StartDate: startDate,
				EndDate:   endDate,
			}

			if unblendedCost, ok := resultByTime.Total["UnblendedCost"]; ok {
				var amount float64
				if _, err := fmt.Sscanf(*unblendedCost.Amount, "%f", &amount); err == nil {
					result.Amount = amount
				}
				if unblendedCost.Unit != nil {
					result.Currency = *unblendedCost.Unit
				}
			}

			results = append(results, result)
		}
	}

	return results, nil
}

func isNumeric(s string) bool {
	for _, c := range s {
		if c < '0' || c > '9' {
			return false
		}
	}
	return true
}

// GetReservationUtilization retrieves reservation utilization metrics.
func (c *Client) GetReservationUtilization(ctx context.Context, startTime, endTime time.Time) (*costexplorer.GetReservationUtilizationOutput, error) {
	input := &costexplorer.GetReservationUtilizationInput{
		TimePeriod: &types.DateInterval{
			Start: aws.String(startTime.Format("2006-01-02")),
			End:   aws.String(endTime.Format("2006-01-02")),
		},
		Granularity: types.GranularityDaily, // Or make configurable
	}

	output, err := WithRetry(ctx, DefaultRetryConfig(), func(ctx context.Context) (*costexplorer.GetReservationUtilizationOutput, error, bool) {
		out, err := c.ceClient.GetReservationUtilization(ctx, input)
		if err != nil {
			return nil, err, isRetryableError(err)
		}
		return out, nil, false
	})
	
	return output, err
}

// GetSavingsPlanCoverage retrieves savings plan coverage metrics.
func (c *Client) GetSavingsPlansCoverage(ctx context.Context, startTime, endTime time.Time) (*costexplorer.GetSavingsPlansCoverageOutput, error) {
	input := &costexplorer.GetSavingsPlansCoverageInput{
		TimePeriod: &types.DateInterval{
			Start: aws.String(startTime.Format("2006-01-02")),
			End:   aws.String(endTime.Format("2006-01-02")),
		},
		Granularity: types.GranularityDaily,
	}

	output, err := WithRetry(ctx, DefaultRetryConfig(), func(ctx context.Context) (*costexplorer.GetSavingsPlansCoverageOutput, error, bool) {
		out, err := c.ceClient.GetSavingsPlansCoverage(ctx, input)
		if err != nil {
			return nil, err, isRetryableError(err)
		}
		return out, nil, false
	})

	return output, err
}

// ValidateCredentials checks if the client credentials are valid by making a lightweight API call.
func (c *Client) ValidateCredentials(ctx context.Context) error {
	// Use a minimal time range to validate credentials
	now := time.Now()
	yesterday := now.AddDate(0, 0, -1)

	input := &costexplorer.GetCostAndUsageInput{
		TimePeriod: &types.DateInterval{
			Start: aws.String(yesterday.Format("2006-01-02")),
			End:   aws.String(now.Format("2006-01-02")),
		},
		Granularity: types.GranularityDaily,
		Metrics:     []string{"UnblendedCost"},
	}

	_, err := WithRetry(ctx, DefaultRetryConfig(), func(ctx context.Context) (*costexplorer.GetCostAndUsageOutput, error, bool) {
		out, err := c.ceClient.GetCostAndUsage(ctx, input)
		if err != nil {
			return nil, err, isRetryableError(err)
		}
		return out, nil, false
	})

	if err != nil {
		return fmt.Errorf("validating credentials: %w", err)
	}

	return nil
}

// GetSupportedRegions returns the list of supported AWS regions.
// Cost Explorer is a global service, so region doesn't affect data access.
func (c *Client) GetSupportedRegions(_ context.Context) ([]string, error) {
	return []string{
		"us-east-1",
		"us-east-2",
		"us-west-1",
		"us-west-2",
		"eu-west-1",
		"eu-west-2",
		"eu-west-3",
		"eu-central-1",
		"eu-north-1",
		"ap-northeast-1",
		"ap-northeast-2",
		"ap-northeast-3",
		"ap-southeast-1",
		"ap-southeast-2",
		"ap-south-1",
		"sa-east-1",
		"ca-central-1",
	}, nil
}

// Region returns the configured AWS region.
func (c *Client) Region() string {
	return c.region
}