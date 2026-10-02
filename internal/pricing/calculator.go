// Package pricing implements the FinFocus plugin interface for AWS Cost Explorer.
package pricing

import (
	"context"
	"errors"
	"fmt"
	"regexp"
	"strings"
	"time"

	"github.com/aws/aws-sdk-go-v2/service/costexplorer/types"
	"github.com/rs/zerolog"
	"github.com/rs/zerolog/log"
	"github.com/rshade/finfocus-plugin-aws-ce/internal/client"
	"github.com/rshade/finfocus-spec/sdk/go/pluginsdk"
	pbc "github.com/rshade/finfocus-spec/sdk/go/proto/finfocus/v1"
	"google.golang.org/grpc/codes"
	"google.golang.org/grpc/status"
	"google.golang.org/protobuf/types/known/timestamppb"
)

// Calculator implements the FinFocus plugin interface for AWS Cost Explorer.
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
	logger := log.With().Str("component", "finfocus-plugin-aws-ce").Logger()

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
	// Log operation timing using SDK helper
	done := pluginsdk.LogOperation(c.logger, "GetActualCost")
	defer done()

	// Validate request using SDK validation helper
	if err := pluginsdk.ValidateActualCostRequest(req); err != nil {
		return nil, status.Error(codes.InvalidArgument, err.Error())
	}

	resourceID := req.GetResourceId()
	arn := req.GetArn()

	// Contextual logger
	logEvent := c.logger.Debug().Str("resource_id", resourceID)
	if arn != "" {
		logEvent.Str("arn", arn)
	}
	logEvent.Msg("GetActualCost request received")

	startTime := req.GetStart().AsTime().UTC()
	endTime := req.GetEnd().AsTime().UTC()
	if !endTime.After(startTime) {
		return nil, status.Error(codes.InvalidArgument, "start must be before end")
	}

	plan, err := c.planCostQuery(req)
	if err != nil {
		return nil, err
	}
	if err := validateCostLookback(plan, startTime); err != nil {
		c.logger.Error().Time("start", startTime).Err(err).Msg("Date range exceeds AWS limits")
		return nil, err
	}

	if err := c.initClient(ctx); err != nil {
		return nil, status.Errorf(codes.Internal, "client initialization failed: %v", err)
	}

	cacheKey := fmt.Sprintf("cost:%s:%d:%d", plan.cacheID, req.GetStart().GetSeconds(), req.GetEnd().GetSeconds())
	if c.cache != nil {
		if results, ok := c.cache.Get(cacheKey); ok {
			c.logger.Debug().Msg("Cache hit for cost query")
			return c.buildResponse(results), nil
		}
	}

	var clientCosts []client.CostResult
	if plan.resourceLevel {
		clientCosts, err = c.ceClient.GetCostWithResources(ctx, plan.resourceID, plan.accountID, startTime, endTime)
	} else {
		clientCosts, err = c.ceClient.GetCost(ctx, nil, []string{"SERVICE"}, startTime, endTime, "DAILY")
	}
	if err != nil {
		c.logger.Error().Err(err).Msg("Failed to retrieve costs from AWS")
		return nil, mapCostError(resourceID, err)
	}

	costs, err := aggregateCosts(clientCosts)
	if err != nil {
		c.logger.Error().Err(err).Msg("Failed to aggregate costs")
		return nil, mapCostError(resourceID, err)
	}
	if len(costs) == 0 {
		return nil, status.Error(codes.NotFound, pluginsdk.NoDataError(resourceID).Error())
	}

	c.logger.Info().Int("results_count", len(costs)).Msg("Retrieved costs from AWS")

	if c.cache != nil {
		if err := c.cache.Set(cacheKey, costs); err != nil {
			c.logger.Warn().Err(err).Msg("Failed to cache results")
		}
	}

	return c.buildResponse(costs), nil
}

type costQueryPlan struct {
	resourceLevel bool
	resourceID    string
	accountID     string
	cacheID       string
}

// ec2InstanceIDPattern is an EC2 instance id: 8 to 17 lowercase hex characters.
var ec2InstanceIDPattern = regexp.MustCompile(`^i-[0-9a-f]{8,17}$`)

func (c *Calculator) planCostQuery(req *pbc.GetActualCostRequest) (costQueryPlan, error) {
	resourceID := req.GetResourceId()
	plan := costQueryPlan{cacheID: resourceID}
	if id, ok := ec2InstanceID(resourceID); ok {
		plan.resourceLevel = true
		plan.resourceID = id
		plan.cacheID = id
	}
	arn := req.GetArn()
	if arn == "" {
		return plan, nil
	}
	parsed, err := ParseARN(arn)
	if err != nil {
		if resourceID == "" {
			return costQueryPlan{}, status.Error(codes.InvalidArgument, "malformed ARN and empty resource id")
		}
		if plan.resourceLevel {
			c.logger.Warn().Err(err).Str("arn", arn).Str("resource_id", plan.resourceID).
				Msg("Malformed ARN; using ResourceId as the EC2 instance id")
		} else {
			c.logger.Warn().Err(err).Str("arn", arn).Str("resource_id", resourceID).
				Msg("Malformed ARN; ResourceId is not an EC2 instance id, querying service totals")
		}
		return plan, nil
	}
	if parsed.Service != "ec2" {
		return costQueryPlan{}, status.Errorf(codes.InvalidArgument, "resource-level cost is not available for service %s", parsed.Service)
	}
	instanceID, ok := ec2InstanceID(parsed.Resource)
	if !ok {
		return costQueryPlan{}, status.Errorf(codes.InvalidArgument, "resource-level cost is not available for service ec2 resource %s", parsed.Resource)
	}
	if resourceID != "" && resourceID != instanceID && !strings.HasSuffix(parsed.Resource, resourceID) {
		c.logger.Warn().
			Str("resource_id", resourceID).
			Str("arn_resource", parsed.Resource).
			Msg("Identifier mismatch: ResourceId does not match ARN resource component. Using ARN as source of truth.")
	}
	plan.resourceLevel = true
	plan.resourceID = instanceID
	plan.accountID = parsed.AccountID
	plan.cacheID = instanceID
	return plan, nil
}

func ec2InstanceID(resource string) (string, bool) {
	resource = strings.TrimPrefix(resource, "instance/")
	if ec2InstanceIDPattern.MatchString(resource) {
		return resource, true
	}
	return "", false
}

func validateCostLookback(plan costQueryPlan, start time.Time) error {
	now := time.Now().UTC()
	if plan.resourceLevel {
		if start.Before(now.AddDate(0, 0, -14)) {
			return status.Error(codes.InvalidArgument, "resource-level cost data covers the last 14 days only")
		}
		return nil
	}
	limit := now.AddDate(0, -14, 0)
	if start.Before(limit) {
		return status.Errorf(codes.InvalidArgument, "invalid time range: start time (%s) exceeds 14 months lookback limit", start.Format(time.RFC3339))
	}
	return nil
}

func mapCostError(resourceID string, err error) error {
	switch {
	case err == nil:
		return nil
	case errors.Is(err, client.ErrNoCostData):
		return status.Error(codes.NotFound, pluginsdk.NoDataError(resourceID).Error())
	case errors.Is(err, client.ErrMixedCurrency):
		return status.Error(codes.FailedPrecondition, err.Error())
	case errors.Is(err, client.ErrAmountMissing), errors.Is(err, client.ErrAmountUnparseable):
		return status.Error(codes.Internal, err.Error())
	case errors.Is(err, client.ErrInvalidTimeRange):
		return status.Error(codes.InvalidArgument, err.Error())
	default:
		return status.Error(codes.Internal, err.Error())
	}
}

func (c *Calculator) buildResponse(costs []CostEntry) *pbc.GetActualCostResponse {
	results := make([]*pbc.ActualCostResult, 0, len(costs))
	for _, cost := range costs {
		usage := 0.0
		unit := ""
		if cost.HasUsage {
			usage = cost.UsageAmount
			unit = cost.UsageUnit
		}
		results = append(results, &pbc.ActualCostResult{
			Timestamp:   timestamppb.New(cost.Timestamp.UTC()),
			Cost:        cost.Amount,
			UsageAmount: usage,
			UsageUnit:   unit,
			Source:      "aws-ce",
			FocusRecord: focusFor(cost),
		})
	}

	return &pbc.GetActualCostResponse{
		Results:      results,
		FallbackHint: pbc.FallbackHint_FALLBACK_HINT_NONE,
	}
}

// GetServiceActualCost retrieves actual costs for a specific AWS service.
func (c *Calculator) GetServiceActualCost(ctx context.Context, serviceName string, startTime, endTime time.Time) (float64, string, error) {
	// Log operation timing using SDK helper
	done := pluginsdk.LogOperation(c.logger, "GetServiceActualCost")
	defer done()

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
	// Log operation timing using SDK helper
	done := pluginsdk.LogOperation(c.logger, "GetAccountActualCost")
	defer done()

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
