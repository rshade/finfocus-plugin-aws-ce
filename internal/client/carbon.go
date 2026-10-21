package client

import (
	"context"
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"errors"
	"math"
	"slices"
	"time"

	"github.com/aws/aws-sdk-go-v2/aws"
	"github.com/aws/aws-sdk-go-v2/credentials/stscreds"
	"github.com/aws/aws-sdk-go-v2/service/sts"
	"github.com/aws/aws-sdk-go-v2/service/sustainability"
	st "github.com/aws/aws-sdk-go-v2/service/sustainability/types"
	"github.com/aws/smithy-go"
	"github.com/rshade/finfocus-spec/sdk/go/pluginsdk"
	pbc "github.com/rshade/finfocus-spec/sdk/go/proto/finfocus/v1"
	"google.golang.org/protobuf/proto"
	"google.golang.org/protobuf/types/known/timestamppb"
)

// SustainabilityAPI is the read-only AWS historical emissions operation.
type SustainabilityAPI interface {
	GetEstimatedCarbonEmissions(context.Context, *sustainability.GetEstimatedCarbonEmissionsInput, ...func(*sustainability.Options)) (*sustainability.GetEstimatedCarbonEmissionsOutput, error)
}

// SustainabilityClient reads caller-scoped reports without caching emissions.
type SustainabilityClient struct{ api SustainabilityAPI }

// CarbonPage contains normalized records and the private, raw AWS continuation token.
// The report handler must protect NextToken before returning it to a caller.
type CarbonPage struct {
	Records   []*pbc.CarbonEmissionRecord
	NextToken string
}

var (
	// ErrCarbonDataInvalid indicates malformed upstream data, never an inferred zero.
	ErrCarbonDataInvalid = errors.New("invalid AWS sustainability record")
	// ErrCarbonResponseEmpty indicates a missing response object, unlike an empty report.
	ErrCarbonResponseEmpty = errors.New("empty AWS sustainability response")
)

// ResolveCarbonCredentials snapshots the effective base credentials for one report page.
// The returned static keys are used for both cursor binding and client signing (or
// explicit target-role assumption), preventing a second default-chain lookup from
// changing the caller after pagination has been checked. Nothing is retained.
func ResolveCarbonCredentials(ctx context.Context, cfg Config) (Config, error) {
	if err := ctx.Err(); err != nil {
		return Config{}, err
	}
	if cfg.AccessKeyID != "" {
		return cfg, nil
	}
	base := cfg
	base.Region = "us-east-1"
	ac, err := loadAWSConfig(ctx, base)
	if err != nil {
		return Config{}, carbonCredentialError(ctx, err)
	}
	if ac.Credentials == nil {
		return Config{}, errors.New("AWS sustainability credentials unavailable")
	}
	credentials, err := ac.Credentials.Retrieve(ctx)
	if err != nil {
		return Config{}, carbonCredentialError(ctx, err)
	}
	if credentials.AccessKeyID == "" || credentials.SecretAccessKey == "" {
		return Config{}, errors.New("AWS sustainability credentials unavailable")
	}
	cfg.AccessKeyID, cfg.SecretAccessKey, cfg.SessionToken = credentials.AccessKeyID, credentials.SecretAccessKey, credentials.SessionToken
	return cfg, nil
}

func carbonCredentialError(ctx context.Context, err error) error {
	if ctx.Err() != nil {
		return ctx.Err()
	}
	if errors.Is(err, context.Canceled) {
		return context.Canceled
	}
	if errors.Is(err, context.DeadlineExceeded) {
		return context.DeadlineExceeded
	}
	// Credential providers can wrap AWS API failures (for example profile role
	// assumption). Preserve only the same safe code whitelist as report calls.
	return safeCarbonError(err)
}

// NewSustainabilityClient uses the normal credential chain with a us-east-1 endpoint.
// Usage regions are request filters and do not change the endpoint region.
func NewSustainabilityClient(ctx context.Context, cfg Config) (*SustainabilityClient, error) {
	cfg.Region = "us-east-1"
	ac, err := loadAWSConfig(ctx, cfg)
	if err != nil {
		return nil, errors.New("loading AWS sustainability configuration")
	}
	if cfg.BaseEndpoint != "" {
		ac.BaseEndpoint = aws.String(cfg.BaseEndpoint)
	}
	if cfg.httpClient != nil {
		ac.HTTPClient = cfg.httpClient
	}
	if cfg.RoleARN != "" {
		base := ac.Copy()
		ac.Credentials = aws.NewCredentialsCache(stscreds.NewAssumeRoleProvider(sts.NewFromConfig(base, func(o *sts.Options) { o.Retryer = aws.NopRetryer{} }), cfg.RoleARN))
	}
	return NewSustainabilityClientWithAPI(sustainability.NewFromConfig(ac, func(o *sustainability.Options) { o.Retryer = aws.NopRetryer{} })), nil
}

// NewSustainabilityClientWithAPI injects a read-only transport for offline fixtures.
func NewSustainabilityClientWithAPI(api SustainabilityAPI) *SustainabilityClient {
	return &SustainabilityClient{api: api}
}

// ReadCarbonPage fetches exactly one AWS page. Every retry consumes the page hook
// budget; the caller owns report pagination, token protection and the page cap.
func (c *SustainabilityClient) ReadCarbonPage(ctx context.Context, req *pbc.GetCarbonEmissionsRequest, awsToken string) (CarbonPage, error) {
	if err := pluginsdk.ValidateGetCarbonEmissionsRequest(req); err != nil {
		return CarbonPage{}, err
	}
	q := proto.CloneOf(req)
	pluginsdk.ApplyGetCarbonEmissionsDefaults(q)
	input := &sustainability.GetEstimatedCarbonEmissionsInput{
		TimePeriod:  &st.TimePeriod{Start: aws.Time(q.Start.AsTime()), End: aws.Time(q.End.AsTime())},
		Granularity: st.TimeGranularityMonthly, GroupBy: []st.Dimension{st.DimensionUsageAccountId, st.DimensionRegion, st.DimensionService},
		EmissionsTypes: []st.EmissionsType{carbonEmissionType(q)}, MaxResults: aws.Int32(q.PageSize),
	}
	if awsToken != "" {
		input.NextToken = aws.String(awsToken)
	}
	filters := map[string][]string{}
	for key, values := range map[string][]string{"USAGE_ACCOUNT_ID": q.AccountIds, "REGION": q.Regions, "SERVICE": q.Services} {
		if len(values) > 0 {
			filters[key] = values
		}
	}
	if len(filters) > 0 {
		input.FilterBy = &st.FilterExpression{Dimensions: filters}
	}
	var hookErr error
	out, err := WithRetry(ctx, DefaultRetryConfig(), func(ctx context.Context) (*sustainability.GetEstimatedCarbonEmissionsOutput, error, bool) {
		if err := ctx.Err(); err != nil {
			return nil, err, false
		}
		if err := runPageHook(ctx); err != nil {
			hookErr = err
			return nil, err, false
		}
		if c == nil || c.api == nil {
			return nil, ErrCarbonResponseEmpty, false
		}
		result, callErr := c.api.GetEstimatedCarbonEmissions(ctx, input)
		return result, callErr, isRetryableError(callErr)
	})
	if hookErr != nil {
		return CarbonPage{}, hookErr
	}
	if err != nil {
		return CarbonPage{}, safeCarbonError(err)
	}
	if out == nil {
		return CarbonPage{}, ErrCarbonResponseEmpty
	}
	page := CarbonPage{NextToken: aws.ToString(out.NextToken)}
	retrieved := timestamppb.Now()
	for _, row := range out.Results {
		record, err := normalizeCarbon(row, q, retrieved)
		if err != nil {
			return CarbonPage{}, err
		}
		page.Records = append(page.Records, record)
	}
	return page, nil
}

func carbonEmissionType(q *pbc.GetCarbonEmissionsRequest) st.EmissionsType {
	if q.Scope == pbc.CarbonEmissionsScope_CARBON_EMISSIONS_SCOPE_SCOPE_1 {
		return st.EmissionsTypeTotalScope1CarbonEmissions
	}
	location := q.Method == pbc.CarbonAccountingMethod_CARBON_ACCOUNTING_METHOD_LOCATION_BASED
	switch q.Scope {
	case pbc.CarbonEmissionsScope_CARBON_EMISSIONS_SCOPE_SCOPE_2:
		if location {
			return st.EmissionsTypeTotalScope2LbmCarbonEmissions
		}
		return st.EmissionsTypeTotalScope2MbmCarbonEmissions
	case pbc.CarbonEmissionsScope_CARBON_EMISSIONS_SCOPE_SCOPE_3:
		if location {
			return st.EmissionsTypeTotalScope3LbmCarbonEmissions
		}
		return st.EmissionsTypeTotalScope3MbmCarbonEmissions
	default:
		if location {
			return st.EmissionsTypeTotalLbmCarbonEmissions
		}
		return st.EmissionsTypeTotalMbmCarbonEmissions
	}
}

func normalizeCarbon(row st.EstimatedCarbonEmissions, q *pbc.GetCarbonEmissionsRequest, retrieved *timestamppb.Timestamp) (*pbc.CarbonEmissionRecord, error) {
	quantity, ok := row.EmissionsValues[string(carbonEmissionType(q))]
	if !ok || quantity.Value == nil || quantity.Unit != st.EmissionsUnitMtCo2e || row.TimePeriod == nil || row.TimePeriod.Start == nil || row.TimePeriod.End == nil {
		return nil, ErrCarbonDataInvalid
	}
	value := *quantity.Value * 1_000_000
	if math.IsNaN(value) || math.IsInf(value, 0) || value < 0 {
		return nil, ErrCarbonDataInvalid
	}
	record := &pbc.CarbonEmissionRecord{
		Provider: "aws", Source: "aws-sustainability",
		AccountId: row.DimensionsValues["USAGE_ACCOUNT_ID"],
		Region:    row.DimensionsValues["REGION"], Service: row.DimensionsValues["SERVICE"],
		Start: timestamppb.New(*row.TimePeriod.Start), End: timestamppb.New(*row.TimePeriod.End),
		Method: q.Method, Scope: q.Scope, ModelVersion: aws.ToString(row.ModelVersion),
		Value: proto.Float64(value), Unit: pluginsdk.CarbonEmissionsUnit, RetrievedAt: retrieved,
	}
	// A length-delimited JSON tuple avoids delimiter collisions in AWS dimensions.
	identity, _ := json.Marshal([]string{record.Provider, record.AccountId, record.Region, record.Service, record.Start.AsTime().Format(time.RFC3339Nano), record.End.AsTime().Format(time.RFC3339Nano), record.Method.String(), record.Scope.String()})
	digest := sha256.Sum256(identity)
	record.Id = hex.EncodeToString(digest[:])
	if pluginsdk.ValidateCarbonEmissionRecord(record) != nil || record.Start.AsTime().Before(q.Start.AsTime()) || record.End.AsTime().After(q.End.AsTime()) {
		return nil, ErrCarbonDataInvalid
	}
	for _, filter := range []struct {
		values []string
		value  string
	}{{q.AccountIds, record.AccountId}, {q.Regions, record.Region}, {q.Services, record.Service}} {
		if len(filter.values) > 0 && !slices.Contains(filter.values, filter.value) {
			return nil, ErrCarbonDataInvalid
		}
	}
	return record, nil
}

func safeCarbonError(err error) error {
	if errors.Is(err, context.Canceled) {
		return context.Canceled
	}
	if errors.Is(err, context.DeadlineExceeded) {
		return context.DeadlineExceeded
	}
	var api smithy.APIError
	if errors.As(err, &api) {
		code := "UpstreamError"
		switch api.ErrorCode() {
		case "AccessDeniedException", "AccessDenied", "UnauthorizedException",
			"ExpiredToken", "ExpiredTokenException", "InvalidClientTokenId", "UnrecognizedClientException", "AuthFailure",
			"ThrottlingException", "Throttling", "TooManyRequestsException", "ValidationException", "InternalServerException", "ServiceUnavailableException":
			code = api.ErrorCode()
		}
		return &smithy.GenericAPIError{Code: code, Message: "AWS sustainability request failed"}
	}
	// Never unwrap raw AWS errors, including transport and credential diagnostics.
	if errors.Is(err, ErrCarbonResponseEmpty) {
		return ErrCarbonResponseEmpty
	}
	return errors.New("AWS sustainability request failed")
}
