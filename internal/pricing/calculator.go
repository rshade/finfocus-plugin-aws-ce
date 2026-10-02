// Package pricing implements the FinFocus plugin interface for AWS Cost Explorer.
package pricing

import (
	"context"
	"errors"
	"fmt"
	"math/big"
	"regexp"
	"strings"
	"sync"
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
	// openClient builds the client for one call that carried credentials.
	// Nil uses client.NewClient. The result is not stored on ceClient.
	openClient func(context.Context, client.Config) (*client.Client, error)
	// maxRequestsPerMinute is the Cost Explorer page budget. Zero means no limit.
	maxRequestsPerMinute int
	requestMu            sync.Mutex
	requestTimes         []time.Time
}

var (
	_ pluginsdk.SupportsProvider             = (*Calculator)(nil)
	_ pluginsdk.PluginInfoProvider           = (*Calculator)(nil)
	_ pluginsdk.PerRequestCredentialConsumer = (*Calculator)(nil)
)

const (
	pluginName    = "aws-ce"
	pluginVersion = "0.1.0"
	// specVersion must keep the leading v. pluginsdk.ValidateSpecVersion rejects "0.7.0".
	specVersion   = "v0.7.0"
	supportedRPCs = "GetActualCost,Supports,GetPluginInfo,GetProjectedCost"
)

// NewCalculator creates a new AWS Cost Explorer cost calculator plugin.
func NewCalculator() *Calculator {
	base := pluginsdk.NewBasePlugin(pluginName)

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
		BasePlugin:           base,
		ceClient:             nil, // Will be initialized lazily
		cache:                cm,
		logger:               logger,
		maxRequestsPerMinute: maxRequestsPerMinuteFromEnv(),
	}
}

// traceLogger attaches the context trace id once. WithTrace on the returned
// logger writes trace_id a second time, so a request must not call it again.
func (c *Calculator) traceLogger(ctx context.Context) zerolog.Logger {
	return pluginsdk.WithTrace(ctx, c.logger)
}

// GetPluginInfo returns discovery metadata for this plugin.
// Capabilities lists actual costs only. Leaving it empty would make the server
// infer projected cost from GetProjectedCost, which still returns an error.
func (c *Calculator) GetPluginInfo(ctx context.Context, _ *pbc.GetPluginInfoRequest) (*pbc.GetPluginInfoResponse, error) {
	logger := c.traceLogger(ctx)
	done := pluginsdk.LogOperation(logger, "GetPluginInfo")
	defer done()

	return &pbc.GetPluginInfoResponse{
		Name:        pluginName,
		Version:     pluginVersion,
		SpecVersion: specVersion,
		Providers:   []string{"aws"},
		Metadata: map[string]string{
			"supported_rpcs": supportedRPCs,
		},
		Capabilities: []pbc.PluginCapability{
			pbc.PluginCapability_PLUGIN_CAPABILITY_ACTUAL_COSTS,
		},
	}, nil
}

// NewCalculatorWithClient creates a calculator with a pre-configured client.
func NewCalculatorWithClient(ceClient *client.Client) *Calculator {
	calc := NewCalculator()
	calc.ceClient = ceClient
	return calc
}

// Supports reports whether Cost Explorer can answer for the resource.
// An unsupported provider or an unusable resource returns Supported false and a
// reason, never a Go error. Server.Supports replaces plugin errors with
// codes.Internal, which would hide that reason. CapabilitiesEnum is left empty
// so the server can fill it. Cost Explorer is global, so region is not checked.
func (c *Calculator) Supports(ctx context.Context, req *pbc.SupportsRequest) (*pbc.SupportsResponse, error) {
	logger := c.traceLogger(ctx)
	done := pluginsdk.LogOperation(logger, "Supports")
	defer done()

	resource := req.GetResource()
	if resource == nil {
		return &pbc.SupportsResponse{
			Supported: false,
			Reason:    invalidResourceReason("resource descriptor is required"),
		}, nil
	}
	if resource.GetProvider() != "aws" {
		return &pbc.SupportsResponse{
			Supported: false,
			Reason: fmt.Sprintf(
				"provider %q is not supported; aws-ce only supports provider \"aws\"",
				resource.GetProvider(),
			),
		}, nil
	}
	if awsResourceIdentified(resource) {
		return &pbc.SupportsResponse{Supported: true}, nil
	}
	return &pbc.SupportsResponse{
		Supported: false,
		Reason:    invalidResourceReason("aws resource needs a non-empty id or an ARN ParseARN accepts"),
	}, nil
}

func invalidResourceReason(detail string) string {
	return pbc.ErrorCode_ERROR_CODE_INVALID_RESOURCE.String() + ": " + detail
}

// awsResourceIdentified is true when Arn parses or Id is non-empty.
// A malformed ARN does not reject a resource that still has an Id.
func awsResourceIdentified(resource *pbc.ResourceDescriptor) bool {
	if resource.GetId() != "" {
		return true
	}
	arn := resource.GetArn()
	if arn == "" {
		return false
	}
	_, err := ParseARN(arn)
	return err == nil
}

// initClient initializes the Cost Explorer client if not already done.
func (c *Calculator) initClient(ctx context.Context, logger zerolog.Logger) error {
	if c.ceClient != nil {
		return nil
	}

	ceClient, err := client.NewClient(ctx, client.Config{})
	if err != nil {
		logger.Error().Err(err).Msg("Failed to initialize Cost Explorer client")
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
	logger := c.traceLogger(ctx)
	done := pluginsdk.LogOperation(logger, "GetActualCost")
	defer done()

	// Validate request using SDK validation helper
	if err := pluginsdk.ValidateActualCostRequest(req); err != nil {
		return nil, actualCostRequestError(err)
	}

	resourceID := req.GetResourceId()
	arn := req.GetArn()

	logEvent := logger.Debug().Str("resource_id", resourceID)
	if arn != "" {
		logEvent.Str("arn", arn)
	}
	logEvent.Msg("GetActualCost request received")

	startTime := req.GetStart().AsTime().UTC()
	endTime := req.GetEnd().AsTime().UTC()
	if !endTime.After(startTime) {
		return nil, statusWithDetail(codes.InvalidArgument, "start must be before end", pbc.ErrorCode_ERROR_CODE_INVALID_TIME_RANGE)
	}

	plan, err := c.planCostQuery(logger, req)
	if err != nil {
		return nil, err
	}
	if err := validateCostLookback(plan, startTime); err != nil {
		logger.Error().Time("start", startTime).Err(err).Msg("Date range exceeds AWS limits")
		return nil, err
	}

	var pages int
	ctx = c.withCostBudget(ctx, &pages)

	ce, perRequest, err := c.clientForCall(ctx, logger)
	if err != nil {
		if _, ok := status.FromError(err); ok {
			return nil, err
		}
		return nil, status.Errorf(codes.Internal, "client initialization failed: %v", err)
	}

	cacheKey := actualCostCacheKey(plan, req)
	if !perRequest && c.cache != nil {
		if entry, ok := c.cache.getEntry(cacheKey); ok {
			logger.Info().Int("ce_requests", 0).Msg("Cache hit for cost query")
			return c.buildResponse(entry.Results, entry.ExpiresAt), nil
		}
	}

	var clientCosts []client.CostResult
	if plan.resourceLevel {
		clientCosts, err = ce.GetCostWithResources(ctx, plan.resourceID, plan.accountID, startTime, endTime)
	} else {
		clientCosts, err = ce.GetCost(ctx, nil, []string{"SERVICE"}, startTime, endTime, cacheGranularity)
	}
	if err != nil {
		if errors.Is(err, client.ErrRateLimited) {
			logger.Warn().Int("ce_requests", pages).Msg("Cost Explorer request limit reached")
			return nil, statusWithDetail(codes.ResourceExhausted, err.Error(), pbc.ErrorCode_ERROR_CODE_RATE_LIMITED)
		}
		if msg, ok := perRequestAWSFailure(perRequest, err); ok {
			logger.Error().Int("ce_requests", pages).Msg(msg)
			return nil, status.Error(codes.Internal, msg)
		}
		logger.Error().Err(err).Int("ce_requests", pages).Msg("Failed to retrieve costs from AWS")
		return nil, mapCostError(resourceID, err)
	}

	costs, err := aggregateCosts(clientCosts)
	if err != nil {
		logger.Error().Err(err).Msg("Failed to aggregate costs")
		return nil, mapCostError(resourceID, err)
	}
	if len(costs) == 0 {
		return nil, statusWithDetail(codes.NotFound, pluginsdk.NoDataError(resourceID).Error(), pbc.ErrorCode_ERROR_CODE_RESOURCE_NOT_FOUND)
	}

	ttl := actualCostTTL(endTime, costs, time.Now().UTC())
	expires := time.Now().UTC().Add(ttl)
	if !perRequest && c.cache != nil {
		stored, cacheErr := c.cache.SetWithTTL(cacheKey, costs, ttl)
		if cacheErr != nil {
			logger.Warn().Err(cacheErr).Msg("Failed to cache results")
		}
		if !stored.IsZero() {
			expires = stored
		}
	}

	logger.Info().Int("ce_requests", pages).Int("results_count", len(costs)).Msg("Retrieved costs from AWS")

	return c.buildResponse(costs, expires), nil
}

type costQueryPlan struct {
	resourceLevel bool
	resourceID    string
	accountID     string
	cacheID       string
}

// ec2InstanceIDPattern is an EC2 instance id: 8 to 17 lowercase hex characters.
var ec2InstanceIDPattern = regexp.MustCompile(`^i-[0-9a-f]{8,17}$`)

func (c *Calculator) planCostQuery(logger zerolog.Logger, req *pbc.GetActualCostRequest) (costQueryPlan, error) {
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
			return costQueryPlan{}, statusWithDetail(codes.InvalidArgument, "malformed ARN and empty resource id", pbc.ErrorCode_ERROR_CODE_INVALID_RESOURCE)
		}
		if plan.resourceLevel {
			logger.Warn().Err(err).Str("arn", arn).Str("resource_id", plan.resourceID).
				Msg("Malformed ARN; using ResourceId as the EC2 instance id")
		} else {
			logger.Warn().Err(err).Str("arn", arn).Str("resource_id", resourceID).
				Msg("Malformed ARN; ResourceId is not an EC2 instance id, querying service totals")
		}
		return plan, nil
	}
	if parsed.Service != "ec2" {
		return costQueryPlan{}, statusWithDetail(
			codes.InvalidArgument,
			fmt.Sprintf("resource-level cost is not available for service %s", parsed.Service),
			pbc.ErrorCode_ERROR_CODE_INVALID_RESOURCE,
		)
	}
	instanceID, ok := ec2InstanceID(parsed.Resource)
	if !ok {
		return costQueryPlan{}, statusWithDetail(
			codes.InvalidArgument,
			fmt.Sprintf("resource-level cost is not available for service ec2 resource %s", parsed.Resource),
			pbc.ErrorCode_ERROR_CODE_INVALID_RESOURCE,
		)
	}
	if resourceID != "" && resourceID != instanceID && !strings.HasSuffix(parsed.Resource, resourceID) {
		logger.Warn().
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
			return statusWithDetail(
				codes.InvalidArgument,
				"resource-level cost data covers the last 14 days only",
				pbc.ErrorCode_ERROR_CODE_INVALID_TIME_RANGE,
			)
		}
		return nil
	}
	limit := now.AddDate(0, -14, 0)
	if start.Before(limit) {
		return statusWithDetail(
			codes.InvalidArgument,
			fmt.Sprintf("invalid time range: start time (%s) exceeds 14 months lookback limit", start.Format(time.RFC3339)),
			pbc.ErrorCode_ERROR_CODE_INVALID_TIME_RANGE,
		)
	}
	return nil
}

func mapCostError(resourceID string, err error) error {
	switch {
	case err == nil:
		return nil
	case errors.Is(err, client.ErrNoCostData):
		return statusWithDetail(
			codes.NotFound,
			pluginsdk.NoDataError(resourceID).Error(),
			pbc.ErrorCode_ERROR_CODE_RESOURCE_NOT_FOUND,
		)
	case errors.Is(err, client.ErrMixedCurrency):
		// No ErrorCode matches mixed currencies. Leave the status without a detail.
		return status.Error(codes.FailedPrecondition, err.Error())
	case errors.Is(err, client.ErrAmountMissing), errors.Is(err, client.ErrAmountUnparseable):
		return status.Error(codes.Internal, err.Error())
	case errors.Is(err, client.ErrInvalidTimeRange):
		return statusWithDetail(codes.InvalidArgument, err.Error(), pbc.ErrorCode_ERROR_CODE_INVALID_TIME_RANGE)
	default:
		return status.Error(codes.Internal, err.Error())
	}
}

// actualCostRequestError keeps InvalidArgument and attaches a proto code only
// when the failure is a resource identity or a time range. Other bad input has
// no matching ErrorCode.
func actualCostRequestError(err error) error {
	msg := err.Error()
	switch {
	case errors.Is(err, pluginsdk.ErrActualCostResourceIDEmpty):
		return statusWithDetail(codes.InvalidArgument, msg, pbc.ErrorCode_ERROR_CODE_INVALID_RESOURCE)
	case errors.Is(err, pluginsdk.ErrActualCostTimeRangeInvalid),
		errors.Is(err, pluginsdk.ErrActualCostStartTimeNil),
		errors.Is(err, pluginsdk.ErrActualCostEndTimeNil):
		return statusWithDetail(codes.InvalidArgument, msg, pbc.ErrorCode_ERROR_CODE_INVALID_TIME_RANGE)
	default:
		return status.Error(codes.InvalidArgument, msg)
	}
}

// statusWithDetail attaches pbc.ErrorDetail without changing the gRPC code or message.
// WithDetails fails only when the detail type cannot be marshaled; the status is still returned.
func statusWithDetail(code codes.Code, msg string, errorCode pbc.ErrorCode) error {
	st := status.New(code, msg)
	detailed, err := st.WithDetails(&pbc.ErrorDetail{Code: errorCode, Message: msg})
	if err != nil {
		return st.Err()
	}
	return detailed.Err()
}

func (c *Calculator) buildResponse(costs []CostEntry, expires time.Time) *pbc.GetActualCostResponse {
	expiresAt := timestamppb.New(expires.UTC())
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
			ExpiresAt:   expiresAt,
		})
	}

	return &pbc.GetActualCostResponse{
		Results:      results,
		FallbackHint: pbc.FallbackHint_FALLBACK_HINT_NONE,
	}
}

// GetServiceActualCost retrieves actual costs for a specific AWS service.
// The float is one conversion of the decimal sum. A missing amount or mixed
// currencies is an error, not a zero or a last-currency total.
func (c *Calculator) GetServiceActualCost(ctx context.Context, serviceName string, startTime, endTime time.Time) (float64, string, error) {
	// Log operation timing using SDK helper
	done := pluginsdk.LogOperation(c.logger, "GetServiceActualCost")
	defer done()

	ce, perRequest, err := c.clientForCall(ctx, c.logger)
	if err != nil {
		if _, ok := status.FromError(err); ok {
			return 0, "", err
		}
		return 0, "", fmt.Errorf("client initialization failed: %w", err)
	}

	filter := &types.Expression{
		Dimensions: &types.DimensionValues{
			Key:    types.DimensionService,
			Values: []string{serviceName},
		},
	}
	// Group by UsageType to mimic previous GetServiceCost behavior
	costs, err := ce.GetCost(ctx, filter, []string{"USAGE_TYPE"}, startTime, endTime, "DAILY")
	if err != nil {
		if msg, ok := perRequestAWSFailure(perRequest, err); ok {
			c.logger.Error().Str("service", serviceName).Msg(msg)
			return 0, "", errors.New(msg)
		}
		c.logger.Error().Err(err).Str("service", serviceName).Msg("Failed to get service costs")
		return 0, "", fmt.Errorf("retrieving service costs: %w", err)
	}

	total, currency, err := sumDecimalCosts(costs)
	if err != nil {
		c.logger.Error().Err(err).Str("service", serviceName).Msg("Failed to sum service costs")
		return 0, "", err
	}
	return total, currency, nil
}

// GetAccountActualCost retrieves total actual costs for the AWS account.
// The float is one conversion of the decimal sum. A missing amount or mixed
// currencies is an error, not a zero or a last-currency total.
func (c *Calculator) GetAccountActualCost(ctx context.Context, startTime, endTime time.Time) (float64, string, error) {
	// Log operation timing using SDK helper
	done := pluginsdk.LogOperation(c.logger, "GetAccountActualCost")
	defer done()

	ce, perRequest, err := c.clientForCall(ctx, c.logger)
	if err != nil {
		if _, ok := status.FromError(err); ok {
			return 0, "", err
		}
		return 0, "", fmt.Errorf("client initialization failed: %w", err)
	}

	// Account cost typically aggregates by Service
	costs, err := ce.GetCost(ctx, nil, []string{"SERVICE"}, startTime, endTime, "DAILY")
	if err != nil {
		if msg, ok := perRequestAWSFailure(perRequest, err); ok {
			c.logger.Error().Msg(msg)
			return 0, "", errors.New(msg)
		}
		c.logger.Error().Err(err).Msg("Failed to get account costs")
		return 0, "", fmt.Errorf("retrieving account costs: %w", err)
	}

	total, currency, err := sumDecimalCosts(costs)
	if err != nil {
		c.logger.Error().Err(err).Msg("Failed to sum account costs")
		return 0, "", err
	}
	return total, currency, nil
}

// sumDecimalCosts adds AmountExact once and converts that rational to float64.
// A nil amount is an error. Empty currency stays the historical USD default.
func sumDecimalCosts(rows []client.CostResult) (float64, string, error) {
	var sum *big.Rat
	var currency string
	for _, row := range rows {
		if row.AmountExact == nil {
			return 0, "", fmt.Errorf("%w for group %q", client.ErrAmountMissing, row.ServiceName)
		}
		if row.Currency != "" {
			if currency == "" {
				currency = row.Currency
			} else if currency != row.Currency {
				return 0, "", fmt.Errorf("%w: %s and %s", client.ErrMixedCurrency, currency, row.Currency)
			}
		}
		sum = addCost(sum, row.AmountExact)
	}
	if currency == "" {
		currency = "USD"
	}
	if sum == nil {
		return 0, currency, nil
	}
	total, _ := sum.Float64()
	return total, currency, nil
}
