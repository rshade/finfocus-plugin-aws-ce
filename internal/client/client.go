// Package client provides AWS Cost Explorer API client implementation.
package client

import (
	"context"
	"errors"
	"fmt"
	"math/big"
	"strings"
	"time"

	"github.com/aws/aws-sdk-go-v2/aws"
	"github.com/aws/aws-sdk-go-v2/config"
	"github.com/aws/aws-sdk-go-v2/credentials/stscreds"
	"github.com/aws/aws-sdk-go-v2/service/costexplorer"
	"github.com/aws/aws-sdk-go-v2/service/costexplorer/types"
	"github.com/aws/aws-sdk-go-v2/service/sts"
)

// CostExplorerAPI defines the interface for AWS Cost Explorer operations.
// This interface allows for mocking in tests.
type CostExplorerAPI interface {
	GetCostAndUsage(ctx context.Context, params *costexplorer.GetCostAndUsageInput, optFns ...func(*costexplorer.Options)) (*costexplorer.GetCostAndUsageOutput, error)
	GetCostAndUsageWithResources(ctx context.Context, params *costexplorer.GetCostAndUsageWithResourcesInput, optFns ...func(*costexplorer.Options)) (*costexplorer.GetCostAndUsageWithResourcesOutput, error)
	GetCostForecast(ctx context.Context, params *costexplorer.GetCostForecastInput, optFns ...func(*costexplorer.Options)) (*costexplorer.GetCostForecastOutput, error)
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
	// BaseEndpoint overrides the Cost Explorer endpoint. Empty uses the AWS default.
	// Set this to a fake server in tests so no call leaves the process.
	BaseEndpoint string
	// AccessKeyID, when set, selects static credentials and skips the default chain.
	// SecretAccessKey is the matching secret. Neither value is logged.
	AccessKeyID     string
	SecretAccessKey string
	SessionToken    string
	// RoleARN, when set, is assumed for Cost Explorer calls.
	// STS is built from the static keys, or the default chain when they are empty.
	// That STS client does not use the assume-role provider.
	RoleARN string
	// httpClient, when set, handles AWS calls. Tests use it to reject a dial
	// that is not loopback. Nil keeps the SDK client.
	httpClient aws.HTTPClient
}

var (
	// ErrRegionMissing is a safe configuration diagnostic with no credential data.
	ErrRegionMissing = errors.New("missing AWS region; run export AWS_REGION=us-east-1 or configure region in your AWS profile")
	// ErrAmountMissing is returned when UnblendedCost has no Amount.
	ErrAmountMissing = errors.New("missing cost amount")
	// ErrAmountUnparseable is returned when UnblendedCost.Amount is not a decimal.
	ErrAmountUnparseable = errors.New("unparseable cost amount")
	// ErrMixedCurrency is returned when one response contains more than one currency.
	ErrMixedCurrency = errors.New("mixed currencies")
	// ErrNoCostData is returned when Cost Explorer returns no cost rows.
	ErrNoCostData = errors.New("no cost data")
	// ErrInvalidTimeRange is returned when the end instant is not after the start.
	ErrInvalidTimeRange = errors.New("start must be before end")
	// ErrPageCap is returned when Cost Explorer still has a NextPageToken after 100 pages.
	ErrPageCap = errors.New("cost explorer results exceed 100 pages")
	// ErrRateLimited is returned when the next page would exceed the caller's
	// per-minute budget. The Cost Explorer request is not made.
	ErrRateLimited = errors.New("cost explorer request limit reached")
)

type pageHookKey struct{}

// WithPageHook runs hook before each Cost Explorer page.
// A non-nil error skips that page and stops the query. A nil hook returns ctx.
func WithPageHook(ctx context.Context, hook func() error) context.Context {
	if hook == nil {
		return ctx
	}
	return context.WithValue(ctx, pageHookKey{}, hook)
}

func runPageHook(ctx context.Context) error {
	hook, _ := ctx.Value(pageHookKey{}).(func() error)
	if hook == nil {
		return nil
	}
	return hook()
}

type staticCredentials struct {
	accessKeyID     string
	secretAccessKey string
	sessionToken    string
}

func (s staticCredentials) Retrieve(context.Context) (aws.Credentials, error) {
	return aws.Credentials{
		AccessKeyID:     s.accessKeyID,
		SecretAccessKey: s.secretAccessKey,
		SessionToken:    s.sessionToken,
		Source:          "finfocus-plugin-aws-ce static config",
	}, nil
}

// NewClient creates a new AWS Cost Explorer client.
func NewClient(ctx context.Context, cfg Config) (*Client, error) {
	awsCfg, err := loadAWSConfig(ctx, cfg)
	if err != nil {
		return nil, fmt.Errorf("loading AWS config: %w", err)
	}
	if awsCfg.Region == "" {
		return nil, ErrRegionMissing
	}
	if cfg.BaseEndpoint != "" {
		awsCfg.BaseEndpoint = aws.String(cfg.BaseEndpoint)
	}
	if cfg.httpClient != nil {
		awsCfg.HTTPClient = cfg.httpClient
	}

	ceCfg := awsCfg
	if cfg.RoleARN != "" {
		// Snapshot the base credentials into the STS client first. The cost
		// client then uses the assume-role provider. Sharing that provider
		// with STS would recurse.
		stsClient := sts.NewFromConfig(awsCfg)
		ceCfg = awsCfg.Copy()
		ceCfg.Credentials = aws.NewCredentialsCache(
			stscreds.NewAssumeRoleProvider(stsClient, cfg.RoleARN),
		)
	}

	return &Client{
		ceClient: costexplorer.NewFromConfig(ceCfg),
		region:   ceCfg.Region,
	}, nil
}

func loadAWSConfig(ctx context.Context, cfg Config) (aws.Config, error) {
	var opts []func(*config.LoadOptions) error

	if cfg.Region != "" {
		opts = append(opts, config.WithRegion(cfg.Region))
	}

	if cfg.Profile != "" {
		opts = append(opts, config.WithSharedConfigProfile(cfg.Profile))
	}
	if cfg.AccessKeyID != "" {
		opts = append(opts, config.WithCredentialsProvider(staticCredentials{
			accessKeyID:     cfg.AccessKeyID,
			secretAccessKey: cfg.SecretAccessKey,
			sessionToken:    cfg.SessionToken,
		}))
	}

	return config.LoadDefaultConfig(ctx, opts...)
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
	// It is a single conversion of AmountExact for callers that still read float64.
	Amount float64
	// AmountExact is the decimal UnblendedCost. Nil means the amount was missing.
	AmountExact *big.Rat
	// Currency is the currency code (e.g., "USD").
	Currency string
	// UsageExact is the decimal UsageQuantity. Nil means the metric was absent.
	UsageExact *big.Rat
	// UsageUnit is the UsageQuantity unit. Empty when usage is absent.
	UsageUnit string
	// Estimated is true when the source ResultsByTime period is estimated.
	Estimated bool
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
	// Metric names the Cost Explorer metric stored in AmountExact.
	// AmortizedCost is selected only when exactly one commitment id is set and that
	// metric is present and parseable. Every other row uses UnblendedCost.
	Metric string
}

// ForecastResult represents the cost forecast for a resource.
type ForecastResult struct {
	// MeanValue is the mean forecasted cost.
	MeanValue float64
	// Currency is the currency code (e.g., "USD").
	Currency string
	// StartDate is the start of the forecast period.
	StartDate time.Time
	// EndDate is the end of the forecast period.
	EndDate time.Time
	// PredictionIntervalLowerBound is the lower bound of the prediction interval (if available).
	PredictionIntervalLowerBound *float64
	// PredictionIntervalUpperBound is the upper bound of the prediction interval (if available).
	PredictionIntervalUpperBound *float64
}

// GetCostForecast retrieves the cost forecast for a given time period.
func (c *Client) GetCostForecast(ctx context.Context, filter *types.Expression, startTime, endTime time.Time, granularity string) ([]ForecastResult, error) {
	// Validate granularity (AWS limitation: only DAILY and MONTHLY are supported)
	if granularity != string(types.GranularityDaily) && granularity != string(types.GranularityMonthly) {
		return nil, fmt.Errorf("invalid granularity: %s. Supported values are DAILY and MONTHLY", granularity)
	}

	interval, err := dateInterval(startTime, endTime)
	if err != nil {
		return nil, err
	}
	input := &costexplorer.GetCostForecastInput{
		TimePeriod:              interval,
		Granularity:             types.Granularity(granularity),
		Metric:                  types.MetricUnblendedCost,
		Filter:                  filter,
		PredictionIntervalLevel: aws.Int32(80), // Default to 80% confidence interval
	}

	output, err := WithRetry(ctx, DefaultRetryConfig(), func(ctx context.Context) (*costexplorer.GetCostForecastOutput, error, bool) {
		out, err := c.ceClient.GetCostForecast(ctx, input)
		if err != nil {
			return nil, err, isRetryableError(err)
		}
		return out, nil, false
	})

	if err != nil {
		return nil, fmt.Errorf("getting cost forecast: %w", err)
	}

	return c.parseForecastResults(output)
}

// parseForecastResults converts AWS Cost Explorer forecast output to ForecastResult slice.
func (c *Client) parseForecastResults(output *costexplorer.GetCostForecastOutput) ([]ForecastResult, error) {
	var results []ForecastResult

	for _, forecast := range output.ForecastResultsByTime {
		startDate, err := time.Parse("2006-01-02", *forecast.TimePeriod.Start)
		if err != nil {
			return nil, fmt.Errorf("parsing start date: %w", err)
		}

		endDate, err := time.Parse("2006-01-02", *forecast.TimePeriod.End)
		if err != nil {
			return nil, fmt.Errorf("parsing end date: %w", err)
		}

		result := ForecastResult{
			StartDate: startDate,
			EndDate:   endDate,
		}

		if forecast.MeanValue != nil {
			var amount float64
			if _, err := fmt.Sscanf(*forecast.MeanValue, "%f", &amount); err == nil {
				result.MeanValue = amount
			}
		}

		// Try to find currency from Total (ForecastResultsByTime entries don't have currency field usually)
		if output.Total != nil && output.Total.Unit != nil {
			result.Currency = *output.Total.Unit
		} else {
			result.Currency = "USD" // Default fallback
		}

		if forecast.PredictionIntervalLowerBound != nil {
			var lower float64
			if _, err := fmt.Sscanf(*forecast.PredictionIntervalLowerBound, "%f", &lower); err == nil {
				result.PredictionIntervalLowerBound = &lower
			}
		}

		if forecast.PredictionIntervalUpperBound != nil {
			var upper float64
			if _, err := fmt.Sscanf(*forecast.PredictionIntervalUpperBound, "%f", &upper); err == nil {
				result.PredictionIntervalUpperBound = &upper
			}
		}

		results = append(results, result)
	}

	return results, nil
}

func groupDefinitions(dimensions []string) []types.GroupDefinition {
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
	if len(groupDefinitions) == 0 {
		groupDefinitions = append(groupDefinitions, types.GroupDefinition{
			Type: types.GroupDefinitionTypeDimension,
			Key:  aws.String("SERVICE"),
		})
	}
	return groupDefinitions
}

// GetCost retrieves cost data with flexible filtering and grouping.
func (c *Client) GetCost(ctx context.Context, filter *types.Expression, dimensions []string, startTime, endTime time.Time, granularity string) ([]CostResult, error) {
	if granularity == "" {
		granularity = string(types.GranularityDaily)
	}
	interval, err := dateInterval(startTime, endTime)
	if err != nil {
		return nil, err
	}
	groups := groupDefinitions(dimensions)
	metrics := []string{metricUnblendedCost, metricUsageQuantity}
	if commitmentDimension(dimensions) {
		metrics = append(metrics, metricAmortizedCost)
	}
	return c.collectCosts(ctx, dimensions, func(ctx context.Context, token *string) ([]types.ResultByTime, *string, error) {
		input := &costexplorer.GetCostAndUsageInput{
			TimePeriod:    interval,
			Granularity:   types.Granularity(granularity),
			Metrics:       metrics,
			NextPageToken: token,
			GroupBy:       groups,
			Filter:        filter,
		}
		output, err := WithRetry(ctx, DefaultRetryConfig(), func(ctx context.Context) (*costexplorer.GetCostAndUsageOutput, error, bool) {
			out, err := c.ceClient.GetCostAndUsage(ctx, input)
			if err != nil {
				return nil, err, isRetryableError(err)
			}
			return out, nil, false
		})
		if err != nil {
			return nil, nil, fmt.Errorf("getting cost and usage: %w", err)
		}
		if output == nil {
			return nil, nil, fmt.Errorf("getting cost and usage: empty response")
		}
		return output.ResultsByTime, output.NextPageToken, nil
	})
}

// GetCostWithResources retrieves resource-level cost for an EC2 instance id.
// resourceID is the Cost Explorer RESOURCE_ID value, never a full ARN.
func (c *Client) GetCostWithResources(ctx context.Context, resourceID, accountID string, startTime, endTime time.Time) ([]CostResult, error) {
	interval, err := dateInterval(startTime, endTime)
	if err != nil {
		return nil, err
	}
	parts := []types.Expression{
		{
			Dimensions: &types.DimensionValues{
				Key:    types.DimensionService,
				Values: []string{ec2ComputeService},
			},
		},
		{
			Dimensions: &types.DimensionValues{
				Key:    types.DimensionResourceId,
				Values: []string{resourceID},
			},
		},
	}
	if accountID != "" {
		parts = append(parts, types.Expression{
			Dimensions: &types.DimensionValues{
				Key:    types.DimensionLinkedAccount,
				Values: []string{accountID},
			},
		})
	}
	filter := &types.Expression{And: parts}
	groups := []types.GroupDefinition{{
		Type: types.GroupDefinitionTypeDimension,
		Key:  aws.String(string(types.DimensionResourceId)),
	}}
	return c.collectCosts(ctx, []string{string(types.DimensionResourceId)}, func(ctx context.Context, token *string) ([]types.ResultByTime, *string, error) {
		input := &costexplorer.GetCostAndUsageWithResourcesInput{
			TimePeriod:    interval,
			Granularity:   types.GranularityDaily,
			Metrics:       []string{metricUnblendedCost, metricUsageQuantity},
			NextPageToken: token,
			GroupBy:       groups,
			Filter:        filter,
		}
		output, err := WithRetry(ctx, DefaultRetryConfig(), func(ctx context.Context) (*costexplorer.GetCostAndUsageWithResourcesOutput, error, bool) {
			out, err := c.ceClient.GetCostAndUsageWithResources(ctx, input)
			if err != nil {
				return nil, err, isRetryableError(err)
			}
			return out, nil, false
		})
		if err != nil {
			return nil, nil, fmt.Errorf("getting cost and usage with resources: %w", err)
		}
		if output == nil {
			return nil, nil, fmt.Errorf("getting cost and usage with resources: empty response")
		}
		return output.ResultsByTime, output.NextPageToken, nil
	})
}

func (c *Client) collectCosts(ctx context.Context, dimensions []string, fetch func(context.Context, *string) ([]types.ResultByTime, *string, error)) ([]CostResult, error) {
	var all []CostResult
	var token *string
	for page := 0; page < 100; page++ {
		if err := runPageHook(ctx); err != nil {
			return nil, err
		}
		periods, next, err := fetch(ctx, token)
		if err != nil {
			return nil, err
		}
		rows, err := parseResultPeriods(periods, dimensions)
		if err != nil {
			return nil, fmt.Errorf("parsing cost results: %w", err)
		}
		all = append(all, rows...)
		if next == nil || *next == "" {
			if len(all) == 0 {
				return nil, ErrNoCostData
			}
			return all, nil
		}
		token = next
	}
	return nil, fmt.Errorf("%w; NextPageToken still set", ErrPageCap)
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

const (
	metricUnblendedCost = "UnblendedCost"
	metricAmortizedCost = "AmortizedCost"
	metricUsageQuantity = "UsageQuantity"
	dimService          = "SERVICE"
	dimReservationID    = "RESERVATION_ID"
	dimSavingsPlanARN   = "SAVINGS_PLAN_ARN"
)

func commitmentDimension(dimensions []string) bool {
	for _, dim := range dimensions {
		if dim == dimReservationID || dim == dimSavingsPlanARN {
			return true
		}
	}
	return false
}

func parseResultPeriods(periods []types.ResultByTime, dimensions []string) ([]CostResult, error) {
	var results []CostResult
	for _, resultByTime := range periods {
		if resultByTime.TimePeriod == nil || resultByTime.TimePeriod.Start == nil || resultByTime.TimePeriod.End == nil {
			return nil, fmt.Errorf("cost result missing time period")
		}
		startDate, err := parseCETime(*resultByTime.TimePeriod.Start)
		if err != nil {
			return nil, fmt.Errorf("parsing start date: %w", err)
		}
		endDate, err := parseCETime(*resultByTime.TimePeriod.End)
		if err != nil {
			return nil, fmt.Errorf("parsing end date: %w", err)
		}

		for _, group := range resultByTime.Groups {
			row, err := rowFromMetrics(group.Metrics, startDate, endDate, resultByTime.Estimated, group.Keys, dimensions)
			if err != nil {
				return nil, err
			}
			results = append(results, row)
		}

		if len(resultByTime.Groups) == 0 && len(resultByTime.Total) > 0 {
			if _, ok := resultByTime.Total[metricUnblendedCost]; ok {
				row, err := rowFromMetrics(resultByTime.Total, startDate, endDate, resultByTime.Estimated, nil, nil)
				if err != nil {
					return nil, err
				}
				results = append(results, row)
			}
		}
	}
	return results, nil
}

func rowFromMetrics(metrics map[string]types.MetricValue, start, end time.Time, estimated bool, keys, dimensions []string) (CostResult, error) {
	label := ""
	if len(keys) > 0 {
		label = keys[0]
	}
	service, reservation, savings := groupIdentity(dimensions, keys)
	amount, currency, metric, err := selectedAmount(metrics, reservation, savings, label)
	if err != nil {
		return CostResult{}, err
	}
	asFloat, _ := amount.Float64()
	row := CostResult{
		Amount:         asFloat,
		AmountExact:    amount,
		Currency:       currency,
		StartDate:      start,
		EndDate:        end,
		ServiceName:    service,
		Estimated:      estimated,
		Tags:           map[string]string{},
		ReservationARN: reservation,
		SavingsPlanARN: savings,
		Metric:         metric,
	}
	if usage, ok := metrics[metricUsageQuantity]; ok && usage.Amount != nil {
		parsed, unit, err := parseDecimalAmount(usage)
		if err != nil {
			return CostResult{}, fmt.Errorf("usage: %w for group %q", err, label)
		}
		row.UsageExact = parsed
		row.UsageUnit = unit
	}
	return row, nil
}

// groupIdentity aligns Keys with the requested dimensions, in GroupBy order.
// An empty or blank commitment key is not an id. Every other dimension,
// including RESOURCE_ID beside a reservation id, sets ServiceName.
// A lone commitment dimension leaves ServiceName empty.
func groupIdentity(dimensions, keys []string) (service, reservation, savings string) {
	dims := dimensions
	if len(dims) == 0 {
		dims = []string{dimService}
	}
	if len(dims) == 1 {
		key := ""
		if len(keys) > 0 {
			key = keys[0]
		}
		switch dims[0] {
		case dimService:
			return key, "", ""
		case dimReservationID:
			return "", commitmentID(key), ""
		case dimSavingsPlanARN:
			return "", "", commitmentID(key)
		default:
			return key, "", ""
		}
	}
	for i, dim := range dims {
		key := ""
		if i < len(keys) {
			key = keys[i]
		}
		switch dim {
		case dimReservationID:
			reservation = commitmentID(key)
		case dimSavingsPlanARN:
			savings = commitmentID(key)
		default:
			service = key
		}
	}
	return service, reservation, savings
}

func commitmentID(key string) string {
	return strings.TrimSpace(key)
}

// selectedAmount uses AmortizedCost only for a single commitment id when that
// metric is present. A present value that cannot be parsed is an error.
// BlendedCost is never read.
func selectedAmount(metrics map[string]types.MetricValue, reservation, savings, label string) (*big.Rat, string, string, error) {
	singleCommitment := (reservation != "") != (savings != "")
	if singleCommitment {
		if mv, ok := metrics[metricAmortizedCost]; ok {
			amount, currency, err := parseDecimalAmount(mv)
			if err != nil {
				return nil, "", "", fmt.Errorf("amortized: %w for group %q", err, label)
			}
			return amount, currency, metricAmortizedCost, nil
		}
	}
	mv, ok := metrics[metricUnblendedCost]
	if !ok {
		return nil, "", "", fmt.Errorf("%w for group %q", ErrAmountMissing, label)
	}
	amount, currency, err := parseDecimalAmount(mv)
	if err != nil {
		return nil, "", "", fmt.Errorf("%w for group %q", err, label)
	}
	return amount, currency, metricUnblendedCost, nil
}

func parseDecimalAmount(mv types.MetricValue) (*big.Rat, string, error) {
	if mv.Amount == nil {
		return nil, "", ErrAmountMissing
	}
	raw := strings.TrimSpace(*mv.Amount)
	amount, ok := new(big.Rat).SetString(raw)
	if !ok {
		return nil, "", fmt.Errorf("%w: %q", ErrAmountUnparseable, raw)
	}
	unit := ""
	if mv.Unit != nil {
		unit = *mv.Unit
	}
	return amount, unit, nil
}

func parseCETime(value string) (time.Time, error) {
	if t, err := time.ParseInLocation("2006-01-02", value, time.UTC); err == nil {
		return t, nil
	}
	if t, err := time.Parse(time.RFC3339Nano, value); err == nil {
		return t.UTC(), nil
	}
	return time.Time{}, fmt.Errorf("unrecognized cost explorer time %q", value)
}

func isNumeric(s string) bool {
	for _, c := range s {
		if c < '0' || c > '9' {
			return false
		}
	}
	return true
}

// ValidateCredentials checks if the client credentials are valid by making a lightweight API call.
func (c *Client) ValidateCredentials(ctx context.Context) error {
	// Use a minimal time range to validate credentials
	now := time.Now()
	yesterday := now.AddDate(0, 0, -1)

	interval, err := dateInterval(yesterday, now)
	if err != nil {
		return err
	}
	input := &costexplorer.GetCostAndUsageInput{
		TimePeriod:  interval,
		Granularity: types.GranularityDaily,
		Metrics:     []string{metricUnblendedCost},
	}

	_, err = WithRetry(ctx, DefaultRetryConfig(), func(ctx context.Context) (*costexplorer.GetCostAndUsageOutput, error, bool) {
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
