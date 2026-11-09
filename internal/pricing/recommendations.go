package pricing

import (
	"context"
	"errors"
	"math"
	"regexp"
	"slices"
	"sort"
	"strings"
	"time"

	"github.com/aws/aws-sdk-go-v2/aws"
	"github.com/aws/aws-sdk-go-v2/service/costexplorer"
	"github.com/aws/aws-sdk-go-v2/service/costexplorer/types"
	"github.com/rs/zerolog"
	"github.com/rshade/finfocus-plugin-aws-ce/internal/client"
	"github.com/rshade/finfocus-spec/sdk/go/pluginsdk"
	pbc "github.com/rshade/finfocus-spec/sdk/go/proto/finfocus/v1"
	"google.golang.org/grpc/codes"
	"google.golang.org/grpc/status"
	"google.golang.org/protobuf/proto"
	"google.golang.org/protobuf/reflect/protoreflect"
)

// GetRecommendations reads AWS optimization opportunities, anomalies, and Spot
// comparisons using call-scoped credentials. Each selected source fails on its
// own; see recommendationSourceRun.
func (c *Calculator) GetRecommendations(ctx context.Context, req *pbc.GetRecommendationsRequest) (*pbc.GetRecommendationsResponse, error) {
	logger := c.traceLogger(ctx)
	done := pluginsdk.LogOperation(logger, "GetRecommendations")
	defer done()
	// Anomaly-only pages use AWS tokens, while combined pages use SDK tokens.
	validationReq := req
	if explicitAnomalyRequest(req) {
		validationReq = proto.Clone(req).(*pbc.GetRecommendationsRequest)
		validationReq.PageToken = ""
	}
	if err := validateRecommendationRequest(validationReq); err != nil {
		return nil, err
	}
	if explicitAnomalyRequest(req) {
		return c.getAnomalyRecommendations(ctx, req)
	}
	period := req.GetProjectionPeriod()
	if period == "" {
		period = "monthly"
	}
	f := req.GetFilter()
	rights, plans := recommendationSources(f)
	targets := req.GetTargetResources()
	if len(targets) > 0 {
		var targetRights, targetPlans bool
		for _, target := range targets {
			r, p := recommendationSources(&pbc.RecommendationFilter{Provider: target.GetProvider(), ResourceType: target.GetResourceType()})
			targetRights = targetRights || r
			targetPlans = targetPlans || p
		}
		rights, plans = rights && targetRights, plans && targetPlans
	}
	reservations := selectedReservationServices(req)
	needAnomalies := combinedAnomalyRequest(req)
	needCostExplorer := rights || plans || len(reservations) > 0 || needAnomalies
	spotSelected := c.spotComparison && spotFilterAllows(f) && slices.ContainsFunc(targets, wantsSpotComparison)
	spotNeedsClient := spotSelected && (c.spotQuote == nil || c.onDemandQuote == nil)

	var pages int
	if needCostExplorer {
		ctx = c.withCostBudget(ctx, &pages)
	}
	var ce *client.Client
	var clientErr error
	if needCostExplorer || spotNeedsClient {
		var err error
		ce, _, err = c.clientForCall(ctx, logger)
		if err != nil {
			if _, ok := status.FromError(err); ok {
				return nil, err
			}
			clientErr = status.Error(codes.Internal, "recommendation client initialization failed")
		}
	}

	sources := &recommendationSourceRun{ctx: ctx, logger: logger}
	ceSource := func(name string, fetch func(api *client.Client) ([]*pbc.Recommendation, error)) error {
		return sources.run(name, func() ([]*pbc.Recommendation, error) {
			if ce == nil {
				return nil, clientErr
			}
			return fetch(ce)
		})
	}
	if rights {
		if err := ceSource("rightsizing", func(api *client.Client) ([]*pbc.Recommendation, error) {
			in := costexplorer.GetRightsizingRecommendationInput{Service: aws.String("AmazonEC2"), Configuration: &types.RightsizingRecommendationConfiguration{BenefitsConsidered: true, RecommendationTarget: types.RecommendationTargetSameInstanceFamily}, Filter: recommendationAWSFilter(f)}
			data, err := api.ListRightsizingRecommendations(ctx, in)
			if err != nil {
				return nil, recommendationFailure(err)
			}
			return mapRightsizingRecommendations(logger, data), nil
		}); err != nil {
			return nil, err
		}
	}
	if plans {
		for _, kind := range []types.SupportedSavingsPlansType{types.SupportedSavingsPlansTypeComputeSp, types.SupportedSavingsPlansTypeEc2InstanceSp} {
			if err := ceSource("savings plans "+string(kind), func(api *client.Client) ([]*pbc.Recommendation, error) {
				in := costexplorer.GetSavingsPlansPurchaseRecommendationInput{SavingsPlansType: kind, LookbackPeriodInDays: types.LookbackPeriodInDaysThirtyDays, TermInYears: types.TermInYearsOneYear, PaymentOption: types.PaymentOptionNoUpfront, AccountScope: types.AccountScopePayer, Filter: recommendationAWSFilter(f)}
				data, err := api.ListSavingsPlansRecommendations(ctx, in)
				if err != nil {
					return nil, recommendationFailure(err)
				}
				mapped, err := mapSavingsPlansRecommendations(data)
				if err != nil {
					logger.Error().Err(err).Str("plan_type", string(kind)).Msg("invalid AWS Savings Plans recommendation data")
					return nil, status.Error(codes.DataLoss, "invalid AWS recommendation data: Savings Plans")
				}
				return mapped, nil
			}); err != nil {
				return nil, err
			}
		}
	}
	for _, service := range reservations {
		if err := ceSource("reservations "+service.code, func(api *client.Client) ([]*pbc.Recommendation, error) {
			in := costexplorer.GetReservationPurchaseRecommendationInput{
				Service: aws.String(service.name), AccountScope: types.AccountScopePayer,
				LookbackPeriodInDays: types.LookbackPeriodInDaysThirtyDays,
				TermInYears:          types.TermInYearsOneYear, PaymentOption: types.PaymentOptionNoUpfront,
			}
			// This API accepts an AccountId directly; do not reuse the positive
			// dimension filters supported by rightsizing and Savings Plans.
			if account := f.GetAccountId(); account != "" {
				in.AccountId = aws.String(account)
			}
			reservationPages, err := api.ListReservationRecommendations(ctx, in)
			if err != nil {
				return nil, recommendationFailure(err)
			}
			mapped, err := mapReservationRecommendations(service.code, reservationPages)
			if err != nil {
				logger.Error().Err(err).Str("service", service.code).Msg("invalid AWS reservation recommendation data")
				return nil, status.Error(codes.DataLoss, "invalid AWS recommendation data: reservations")
			}
			return mapped, nil
		}); err != nil {
			return nil, err
		}
	}
	if spotSelected {
		if err := sources.run("spot", func() ([]*pbc.Recommendation, error) {
			if spotNeedsClient && ce == nil {
				return nil, clientErr
			}
			return c.spotRecommendations(ctx, logger, targets), nil
		}); err != nil {
			return nil, err
		}
	}
	recs := sources.recs
	if err := projectRecommendationSavings(recs, period); err != nil {
		return nil, status.Error(codes.DataLoss, "invalid AWS recommendation projection")
	}
	// Anomalies carry no monthly savings, so they join after projection.
	if needAnomalies {
		if err := ceSource("anomalies", func(api *client.Client) ([]*pbc.Recommendation, error) {
			return collectAnomalyRecommendations(ctx, api)
		}); err != nil {
			return nil, err
		}
		recs = sources.recs
	}
	if err := sources.result(); err != nil {
		return nil, err
	}
	selected := make([]*pbc.Recommendation, 0, len(recs))
	for _, r := range recs {
		if recommendationStrictMatch(r, f, req.GetTargetResources()) {
			selected = append(selected, r)
		}
	}
	// Canonicalize documented EC2 aliases before applying the SDK's exact type filter.
	if f != nil {
		f = proto.Clone(f).(*pbc.RecommendationFilter)
		f.ResourceType = canonicalRecommendationType(f.GetResourceType())
		// The SDK converts days to time.Duration, which overflows beyond 106751 days.
		// Strict matching below handles these unusually broad age windows safely.
		if f.GetMaxAgeDays() > 106751 {
			f.MaxAgeDays = 0
		}
	}
	selected = pluginsdk.ApplyRecommendationFilter(selected, f)
	selected = pluginsdk.ApplyRecommendationVisibility(selected, pluginsdk.RecommendationVisibility{IncludeDismissed: req.GetIncludeDismissed(), ExcludedIDs: req.GetExcludedRecommendationIds()})
	// SDK sorting is stable; pre-sort IDs to make all equal-key pages deterministic.
	sort.Slice(selected, func(i, j int) bool { return selected[i].GetId() < selected[j].GetId() })
	selected = pluginsdk.SortRecommendations(selected, f.GetSortBy(), f.GetSortOrder())
	currency := ""
	total := 0.0
	for _, r := range selected {
		if currency != "" && currency != r.GetImpact().GetCurrency() {
			return nil, status.Error(codes.DataLoss, "mixed recommendation currencies")
		}
		currency = r.GetImpact().GetCurrency()
		total += r.GetImpact().GetEstimatedSavings()
		if math.IsInf(total, 0) {
			return nil, status.Error(codes.DataLoss, "recommendation savings overflow")
		}
	}
	page, next, err := pluginsdk.PaginateRecommendations(selected, req.GetPageSize(), req.GetPageToken())
	if err != nil {
		return nil, status.Error(codes.InvalidArgument, "invalid recommendation page token")
	}
	return &pbc.GetRecommendationsResponse{Recommendations: page, NextPageToken: next, Summary: pluginsdk.CalculateRecommendationSummary(page, period)}, nil
}

// recommendationSourceRun collects the sources of one GetRecommendations call.
// A failed source is logged and its rows are dropped, unless the failure is hard
// (cancellation, deadline, or an exhausted request budget), which ends the call.
// When every source fails, the first failure is returned so callers see an error
// rather than an empty success.
type recommendationSourceRun struct {
	ctx       context.Context
	logger    zerolog.Logger
	recs      []*pbc.Recommendation
	succeeded int
	firstErr  error
}

func (s *recommendationSourceRun) run(name string, fetch func() ([]*pbc.Recommendation, error)) error {
	got, err := fetch()
	if err == nil {
		s.recs = append(s.recs, got...)
		s.succeeded++
		return nil
	}
	if s.ctx.Err() != nil || recommendationHardFailure(err) {
		return err
	}
	s.logger.Warn().Str("source", name).Str("code", status.Code(err).String()).Msg("recommendation source failed; returning other sources")
	if s.firstErr == nil {
		s.firstErr = err
	}
	return nil
}

func (s *recommendationSourceRun) result() error {
	if s.succeeded == 0 {
		return s.firstErr
	}
	return nil
}

func recommendationHardFailure(err error) bool {
	switch status.Code(err) {
	case codes.Canceled, codes.DeadlineExceeded, codes.ResourceExhausted:
		return true
	}
	return false
}

var recommendationResourceTypePattern = regexp.MustCompile(`^[a-zA-Z][a-zA-Z0-9_\-:/]*$`)

func validateRecommendationRequest(req *pbc.GetRecommendationsRequest) error {
	invalid := func() error { return status.Error(codes.InvalidArgument, "invalid recommendation request") }
	if req == nil {
		return invalid()
	}
	if req.GetPageToken() != "" {
		if _, err := pluginsdk.DecodePageToken(req.GetPageToken()); err != nil {
			return invalid()
		}
	}
	switch req.GetProjectionPeriod() {
	case "", "daily", "monthly", "annual":
	default:
		return invalid()
	}
	f := req.GetFilter()
	if pluginsdk.ValidateRecommendationFilter(f) != nil {
		return invalid()
	}
	if f != nil {
		for _, v := range []float64{f.GetMinEstimatedSavings(), f.GetMinConfidenceScore()} {
			if math.IsNaN(v) || math.IsInf(v, 0) {
				return invalid()
			}
		}
		for _, field := range []protoreflect.Name{"category", "action_type", "priority", "sort_by", "sort_order"} {
			d := f.ProtoReflect().Descriptor().Fields().ByName(field)
			if d.Enum().Values().ByNumber(f.ProtoReflect().Get(d).Enum()) == nil {
				return invalid()
			}
		}
	}
	if len(req.GetTargetResources()) > 100 {
		return invalid()
	}
	for _, r := range req.GetTargetResources() {
		if pluginsdk.ValidateResourceDescriptor(r) != nil || r.GetProvider() == "" || r.GetResourceType() == "" || !recommendationResourceTypePattern.MatchString(r.GetResourceType()) {
			return invalid()
		}
		switch r.GetProvider() {
		case "aws", "azure", "gcp", "kubernetes", "custom", "aws-native", "azure-native", "azurerm", "google-native", "google":
		default:
			return invalid()
		}
	}
	return nil
}
func canonicalRecommendationType(s string) string {
	switch s {
	case "ec2", "aws:ec2/instance:Instance", "ec2:Instance":
		return "ec2:Instance"
	case "rds", "AmazonRDS":
		return "rds:Account"
	case "redshift", "AmazonRedshift":
		return "redshift:Account"
	case "elasticache", "AmazonElastiCache":
		return "elasticache:Account"
	case "es", "opensearch", "AmazonES":
		return "es:Account"
	}
	return s
}
func recommendationSources(f *pbc.RecommendationFilter) (bool, bool) {
	if f.GetProvider() != "" && f.GetProvider() != "aws" || f.GetSource() != "" && f.GetSource() != recommendationSource || f.GetCategory() != 0 && f.GetCategory() != pbc.RecommendationCategory_RECOMMENDATION_CATEGORY_COST {
		return false, false
	}
	rights, plans := true, true
	switch f.GetActionType() {
	case 0:
	case pbc.RecommendationActionType_RECOMMENDATION_ACTION_TYPE_RIGHTSIZE, pbc.RecommendationActionType_RECOMMENDATION_ACTION_TYPE_TERMINATE:
		plans = false
	case pbc.RecommendationActionType_RECOMMENDATION_ACTION_TYPE_PURCHASE_COMMITMENT:
		rights = false
	default:
		return false, false
	}
	switch canonicalRecommendationType(f.GetResourceType()) {
	case "":
	case "ec2:Instance":
		plans = false
	case "savingsplans:Account":
		rights = false
	default:
		return false, false
	}
	return rights, plans
}
func recommendationAWSFilter(f *pbc.RecommendationFilter) *types.Expression {
	var terms []types.Expression
	if f.GetAccountId() != "" {
		terms = append(terms, types.Expression{Dimensions: &types.DimensionValues{Key: types.DimensionLinkedAccount, Values: []string{f.GetAccountId()}}})
	}
	// Rightsizing REGION filters require display names; select canonical codes locally.

	if len(terms) == 0 {
		return nil
	}
	if len(terms) == 1 {
		return &terms[0]
	}
	return &types.Expression{And: terms}
}
func recommendationTagsMatch(actual, wanted map[string]string) bool {
	for k, v := range wanted {
		value, ok := actual[k]
		if !ok || value != v {
			return false
		}
	}
	return true
}
func recommendationStrictMatch(r *pbc.Recommendation, f *pbc.RecommendationFilter, targets []*pbc.ResourceDescriptor) bool {
	if f.GetMinConfidenceScore() > 0 && r.ConfidenceScore == nil || f.GetMaxAgeDays() > 0 && r.GetCreatedAt() == nil || !recommendationTagsMatch(r.GetResource().GetTags(), f.GetTags()) {
		return false
	}
	if f.GetMaxAgeDays() > 106751 && time.Since(r.GetCreatedAt().AsTime()).Hours()/24 > float64(f.GetMaxAgeDays()) {
		return false
	}
	if len(targets) == 0 {
		return true
	}
	resource := r.GetResource()
	for _, t := range targets {
		if t.GetProvider() != resource.GetProvider() || canonicalRecommendationType(t.GetResourceType()) != resource.GetResourceType() || t.GetSku() != "" && t.GetSku() != resource.GetSku() || t.GetRegion() != "" && t.GetRegion() != resource.GetRegion() || t.GetId() != "" && t.GetId() != resource.GetId() || !recommendationTagsMatch(resource.GetTags(), t.GetTags()) {
			continue
		}
		if t.GetArn() != "" {
			// Only an exact EC2 ARN whose account, region and resource match can constrain an instance.
			parts := strings.SplitN(t.GetArn(), ":", 6)
			if len(parts) != 6 || parts[0] != "arn" || !recommendationARNPartitionMatches(parts[1], parts[3]) || parts[2] != "ec2" || resource.GetResourceType() != "ec2:Instance" || parts[3] != resource.GetRegion() || parts[4] != r.GetMetadata()["account_id"] || parts[5] != "instance/"+resource.GetId() {
				continue
			}
		}
		return true
	}
	return false
}
func recommendationFailure(err error) error {
	switch {
	case errors.Is(err, context.Canceled):
		return status.Error(codes.Canceled, "recommendation request canceled")
	case errors.Is(err, context.DeadlineExceeded):
		return status.Error(codes.DeadlineExceeded, "recommendation request deadline exceeded")
	case errors.Is(err, client.ErrRateLimited):
		return status.Error(codes.ResourceExhausted, "recommendation request budget exhausted")
	}
	if code, ok := awsAPIErrorCode(err); ok {
		if code == "DataUnavailableException" {
			return status.Error(codes.FailedPrecondition, "AWS recommendation dataset unavailable")
		}
		if code == "Throttling" || code == "ThrottlingException" || code == "TooManyRequestsException" {
			return status.Error(codes.ResourceExhausted, "AWS recommendation request throttled")
		}
	}
	if mapped, ok := mapAWSAPIError(err); ok {
		return mapped
	}
	return status.Error(codes.Internal, "retrieving AWS recommendations failed")
}

// recommendationARNPartitionMatches rejects invented partitions and incompatible regional identities.
func recommendationARNPartitionMatches(partition, region string) bool {
	for _, pair := range [][2]string{{"cn-", "aws-cn"}, {"us-gov-", "aws-us-gov"}, {"us-iso-", "aws-iso"}, {"us-isob-", "aws-iso-b"}, {"eu-isoe-", "aws-iso-e"}, {"us-isof-", "aws-iso-f"}, {"eusc-", "aws-eusc"}} {
		if strings.HasPrefix(region, pair[0]) {
			return partition == pair[1]
		}
	}
	return partition == "aws"
}

// explicitAnomalyRequest selects the account-scoped API and its opaque AWS token.
func explicitAnomalyRequest(req *pbc.GetRecommendationsRequest) bool {
	f := req.GetFilter()
	return f.GetCategory() == pbc.RecommendationCategory_RECOMMENDATION_CATEGORY_ANOMALY || f.GetActionType() == pbc.RecommendationActionType_RECOMMENDATION_ACTION_TYPE_INVESTIGATE
}

// combinedAnomalyRequest avoids AWS calls when mapped anomalies cannot match.
func combinedAnomalyRequest(req *pbc.GetRecommendationsRequest) bool {
	f := req.GetFilter()
	if len(req.GetTargetResources()) > 0 || !anomalyRequest(req) ||
		f.GetProvider() != "" && f.GetProvider() != "aws" ||
		f.GetResourceId() != "" || f.GetMinEstimatedSavings() > 0 {
		return false
	}
	// Anomaly resource types are AWS service names, not optimization types.
	switch canonicalRecommendationType(f.GetResourceType()) {
	case "ec2:Instance", "savingsplans:Account", "rds:Account", "redshift:Account", "elasticache:Account", "es:Account":
		return false
	}
	return true
}

// collectAnomalyRecommendations collects AWS pages before the shared SDK pagination.
func collectAnomalyRecommendations(ctx context.Context, ce *client.Client) ([]*pbc.Recommendation, error) {
	start, end := anomalyWindow(time.Now().UTC())
	token := ""
	seen := map[string]bool{}
	var out []*pbc.Recommendation
	for range 100 {
		page, err := ce.GetAnomalies(ctx, start, end, token, 100)
		if err != nil {
			return nil, recommendationFailure(err)
		}
		if page == nil {
			return out, nil
		}
		for _, row := range page.Anomalies {
			out = append(out, mapAnomaly(row))
		}
		next := aws.ToString(page.NextPageToken)
		if next == "" {
			return out, nil
		}
		if seen[next] {
			return nil, status.Error(codes.Internal, "repeated anomaly page token")
		}
		seen[next] = true
		token = next
	}
	return nil, status.Error(codes.ResourceExhausted, "anomaly page limit exceeded")
}
