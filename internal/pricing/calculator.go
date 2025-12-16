// Package pricing implements the PulumiCost plugin interface for AWS Cost Explorer.
package pricing

import (
	"context"
	"fmt"
	"strings"
	"time"

	"github.com/aws/aws-sdk-go-v2/service/costexplorer/types"
	"github.com/rs/zerolog"
	"github.com/rs/zerolog/log"
	"google.golang.org/protobuf/types/known/timestamppb"
	"github.com/rshade/pulumicost-plugin-aws-ce/internal/client"
	pbc "github.com/rshade/pulumicost-spec/sdk/go/proto/pulumicost/v1"
	"github.com/rshade/pulumicost-spec/sdk/go/pluginsdk"
)

// Calculator implements the PulumiCost plugin interface for AWS Cost Explorer.
type Calculator struct {
	*pluginsdk.BasePlugin
	ceClient *client.Client
	cache    *CacheManager
	logger   zerolog.Logger
}

// NewCalculator creates a new AWS Cost Explorer cost calculator plugin.
func NewCalculator() *Calculator {
	base := pluginsdk.NewBasePlugin("aws-ce")

	// Configure supported providers
	providers := []string{"aws"}
	for _, provider := range providers {
		base.Matcher().AddProvider(provider)
	}

	// Initialize cache with default settings
	cm, _ := NewCacheManager("", 24*time.Hour)

	// Configure logger with component field
	logger := log.With().Str("component", "pulumicost-plugin-aws-ce").Logger()

	return &Calculator{
		BasePlugin: base,
		ceClient:   nil, // Will be initialized lazily
		cache:      cm,
		logger:     logger,
	}
}

// NewCalculatorWithClient creates a calculator with a pre-configured client.
func NewCalculatorWithClient(ceClient *client.Client) *Calculator {
	calc := NewCalculator()
	calc.ceClient = ceClient
	return calc
}

// initClient initializes the Cost Explorer client if not already done.
func (c *Calculator) initClient(ctx context.Context) error {
	if c.ceClient != nil {
		return nil
	}

	ceClient, err := client.NewClient(ctx, client.Config{})
	if err != nil {
		c.logger.Error().Err(err).Msg("Failed to initialize Cost Explorer client")
		return fmt.Errorf("initializing Cost Explorer client: %w", err)
	}

	c.ceClient = ceClient
	return nil
}

// GetProjectedCost returns an error as this plugin only provides actual cost data.
func (c *Calculator) GetProjectedCost(_ context.Context, req *pbc.GetProjectedCostRequest) (*pbc.GetProjectedCostResponse, error) {
	// Check if we support this resource
	if !c.Matcher().Supports(req.Resource) {
		return nil, pluginsdk.NotSupportedError(req.Resource)
	}

	// ResourceDescriptor in this version does not have Id, using Type/Sku for logging
	c.logger.Debug().
		Str("resource_type", req.GetResource().GetResourceType()).
		Str("sku", req.GetResource().GetSku()).
		Msg("GetProjectedCost called but not supported")

	return nil, fmt.Errorf("projected cost not supported: aws-ce plugin provides actual cost data only; use aws-public plugin for projected costs")
}

// GetActualCost retrieves actual historical costs from AWS Cost Explorer.
func (c *Calculator) GetActualCost(ctx context.Context, req *pbc.GetActualCostRequest) (*pbc.GetActualCostResponse, error) {
	resourceID := req.GetResourceId()
	arn := req.GetArn()

	// Contextual logger
	logEvent := c.logger.Debug().Str("resource_id", resourceID)
	if arn != "" {
		logEvent.Str("arn", arn)
	}
	logEvent.Msg("GetActualCost request received")

	// Validate input
	if resourceID == "" {
		return nil, fmt.Errorf("invalid request: ResourceId is required")
	}

	// Initialize client if needed
	if err := c.initClient(ctx); err != nil {
		return nil, fmt.Errorf("client initialization failed: %w", err)
	}

	// Parse time range from protobuf Timestamp
	// Using AsTime() would require extra import or checks, doing manual conversion for safety/simplicity
	startTime := time.Unix(req.GetStart().GetSeconds(), int64(req.GetStart().GetNanos()))
	endTime := time.Unix(req.GetEnd().GetSeconds(), int64(req.GetEnd().GetNanos()))

	// Validate time range
	if endTime.Before(startTime) {
		c.logger.Error().
			Time("start", startTime).
			Time("end", endTime).
			Msg("Invalid time range")
		return nil, fmt.Errorf("invalid time range: end time (%v) is before start time (%v)", endTime, startTime)
	}

	// Validate 14 month lookback limit
	lookbackLimit := time.Now().AddDate(0, -14, 0)
	if startTime.Before(lookbackLimit) {
		c.logger.Error().
			Time("start", startTime).
			Time("limit", lookbackLimit).
			Msg("Date range exceeds AWS limits")
		return nil, fmt.Errorf("invalid time range: start time (%v) exceeds 14 months lookback limit", startTime)
	}

	// Resolve identifier to use for lookup (ARN takes precedence if present)
	lookupID := c.resolveIdentifier(req)

	// Generate cache key
	cacheKey := fmt.Sprintf("cost:%s:%d:%d", lookupID, req.GetStart().GetSeconds(), req.GetEnd().GetSeconds())

	// Check cache
	if c.cache != nil {
		if results, ok := c.cache.Get(cacheKey); ok {
			c.logger.Debug().Msg("Cache hit for cost query")
			return c.buildResponse(results), nil
		}
	}

	// Granularity and Dimensions are not in request, using defaults
	granularity := "DAILY"
	dimensions := []string{"SERVICE"}

	// Create filter
	var filter *types.Expression

	// Extract context from ARN if available
	var accountID string
	if arn != "" {
		if parsed, err := ParseARN(arn); err == nil {
			accountID = parsed.AccountID
		}
	}

	if lookupID != "" {
		resourceFilter := types.Expression{
			Dimensions: &types.DimensionValues{
				Key:    types.DimensionResourceId,
				Values: []string{lookupID},
			},
		}

		if accountID != "" {
			accountFilter := types.Expression{
				Dimensions: &types.DimensionValues{
					Key:    types.DimensionLinkedAccount,
					Values: []string{accountID},
				},
			}
			filter = &types.Expression{
				And: []types.Expression{resourceFilter, accountFilter},
			}
		} else {
			filter = &resourceFilter
		}
	}

	// Get costs from Cost Explorer
	start := time.Now()
	clientCosts, err := c.ceClient.GetCost(ctx, filter, dimensions, startTime, endTime, granularity)
	duration := time.Since(start)

	if err != nil {
		c.logger.Error().Err(err).Msg("Failed to retrieve costs from AWS")
		return nil, fmt.Errorf("retrieving costs: %w", err)
	}

	c.logger.Info().
		Int("results_count", len(clientCosts)).
		Dur("duration", duration).
		Msg("Retrieved costs from AWS")

	// If no costs found, return response with NoData hint
	if len(clientCosts) == 0 {
		return &pbc.GetActualCostResponse{
			Results:      []*pbc.ActualCostResult{},
			FallbackHint: pbc.FallbackHint_FALLBACK_HINT_RECOMMENDED,
		}, nil
	}

	// Map client costs to internal CostEntry
	var costs []CostEntry
	for _, cc := range clientCosts {
		costs = append(costs, CostEntry{
			Timestamp:        cc.StartDate,
			Amount:           cc.Amount,
			Currency:         cc.Currency,
			Service:          cc.ServiceName,
			AccountID:        cc.AccountID,
			Region:           cc.Region,
			AvailabilityZone: cc.AvailabilityZone,
			Tags:             cc.Tags,
			ReservationARN:   cc.ReservationARN,
			SavingsPlanARN:   cc.SavingsPlanARN,
		})
	}

	// Update cache
	if c.cache != nil {
		if err := c.cache.Set(cacheKey, costs); err != nil {
			c.logger.Warn().Err(err).Msg("Failed to cache results")
		}
	}

	return c.buildResponse(costs), nil
}

// resolveIdentifier determines which identifier to use for cost lookup.
// Returns ARN if available, otherwise ResourceId.
func (c *Calculator) resolveIdentifier(req *pbc.GetActualCostRequest) string {
	arn := req.GetArn()
	if arn == "" {
		return req.GetResourceId()
	}

	parsed, err := ParseARN(arn)
	if err != nil {
		c.logger.Warn().Err(err).Str("arn", arn).Msg("Malformed ARN provided; falling back to ResourceId")
		return req.GetResourceId()
	}

	// Verify identity: Check if ResourceId matches the parsed resource
	// parsed.Resource might be "instance/i-12345" or "function:name"
	// req.ResourceId might be "i-12345"
	resourceID := req.GetResourceId()
	
	// Check if the parsed resource ends with the request ResourceId
	// This handles "instance/i-123" vs "i-123"
	if !strings.HasSuffix(parsed.Resource, resourceID) {
		// Strict check failed, try contains for safety or just log
		c.logger.Warn().
			Str("resource_id", resourceID).
			Str("arn_resource", parsed.Resource).
			Msg("Identifier mismatch: ResourceId does not match ARN resource component. Using ARN as source of truth.")
	}

	return arn
}

func (c *Calculator) buildResponse(costs []CostEntry) *pbc.GetActualCostResponse {
	var results []*pbc.ActualCostResult
	
	for _, cost := range costs {
		// Create result
		res := &pbc.ActualCostResult{
			Timestamp:   timestamppb.New(cost.Timestamp),
			Cost:        cost.Amount,
			UsageAmount: 0, // Not available in simple mapping
			UsageUnit:   cost.Currency,
			Source:      "aws-ce",
			// FocusRecord: nil, // Leave empty for now
		}
		results = append(results, res)
	}

	return &pbc.GetActualCostResponse{
		Results:      results,
		FallbackHint: pbc.FallbackHint_FALLBACK_HINT_NONE,
	}
}

// GetServiceActualCost retrieves actual costs for a specific AWS service.
func (c *Calculator) GetServiceActualCost(ctx context.Context, serviceName string, startTime, endTime time.Time) (float64, string, error) {
	if err := c.initClient(ctx); err != nil {
		return 0, "", fmt.Errorf("client initialization failed: %w", err)
	}

	filter := &types.Expression{
		Dimensions: &types.DimensionValues{
			Key:    types.DimensionService,
			Values: []string{serviceName},
		},
	}
	// Group by UsageType to mimic previous GetServiceCost behavior
	costs, err := c.ceClient.GetCost(ctx, filter, []string{"USAGE_TYPE"}, startTime, endTime, "DAILY")
	if err != nil {
		c.logger.Error().Err(err).Str("service", serviceName).Msg("Failed to get service costs")
		return 0, "", fmt.Errorf("retrieving service costs: %w", err)
	}

	var totalCost float64
	currency := "USD"
	for _, cost := range costs {
		totalCost += cost.Amount
		if cost.Currency != "" {
			currency = cost.Currency
		}
	}

	return totalCost, currency, nil
}

// GetAccountActualCost retrieves total actual costs for the AWS account.
func (c *Calculator) GetAccountActualCost(ctx context.Context, startTime, endTime time.Time) (float64, string, error) {
	if err := c.initClient(ctx); err != nil {
		return 0, "", fmt.Errorf("client initialization failed: %w", err)
	}

	// Account cost typically aggregates by Service
	costs, err := c.ceClient.GetCost(ctx, nil, []string{"SERVICE"}, startTime, endTime, "DAILY")
	if err != nil {
		c.logger.Error().Err(err).Msg("Failed to get account costs")
		return 0, "", fmt.Errorf("retrieving account costs: %w", err)
	}

	var totalCost float64
	currency := "USD"
	for _, cost := range costs {
		totalCost += cost.Amount
		if cost.Currency != "" {
			currency = cost.Currency
		}
	}

	return totalCost, currency, nil
}