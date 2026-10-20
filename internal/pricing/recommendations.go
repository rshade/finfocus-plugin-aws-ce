package pricing

import (
	"context"
	"errors"
	"math"
	"regexp"
	"sort"
	"strings"
	"time"

	"github.com/aws/aws-sdk-go-v2/aws"
	"github.com/aws/aws-sdk-go-v2/service/costexplorer"
	"github.com/aws/aws-sdk-go-v2/service/costexplorer/types"
	"github.com/rshade/finfocus-plugin-aws-ce/internal/client"
	"github.com/rshade/finfocus-spec/sdk/go/pluginsdk"
	pbc "github.com/rshade/finfocus-spec/sdk/go/proto/finfocus/v1"
	"google.golang.org/grpc/codes"
	"google.golang.org/grpc/status"
	"google.golang.org/protobuf/proto"
	"google.golang.org/protobuf/reflect/protoreflect"
)

// GetRecommendations reads AWS optimization opportunities using credentials scoped to this call.
func (c *Calculator) GetRecommendations(ctx context.Context, req *pbc.GetRecommendationsRequest) (*pbc.GetRecommendationsResponse, error) {
	logger := c.traceLogger(ctx)
	done := pluginsdk.LogOperation(logger, "GetRecommendations")
	defer done()
	if err := validateRecommendationRequest(req); err != nil {
		return nil, err
	}
	period := req.GetProjectionPeriod()
	if period == "" {
		period = "monthly"
	}
	f := req.GetFilter()
	rights, plans := recommendationSources(f)
	if targets := req.GetTargetResources(); len(targets) > 0 {
		var targetRights, targetPlans bool
		for _, target := range targets {
			r, p := recommendationSources(&pbc.RecommendationFilter{Provider: target.GetProvider(), ResourceType: target.GetResourceType()})
			targetRights = targetRights || r
			targetPlans = targetPlans || p
		}
		rights, plans = rights && targetRights, plans && targetPlans
	}
	var recs []*pbc.Recommendation
	if rights || plans {
		var pages int
		ctx = c.withCostBudget(ctx, &pages)
		ce, _, err := c.clientForCall(ctx, logger)
		if err != nil {
			if _, ok := status.FromError(err); ok {
				return nil, err
			}
			return nil, status.Error(codes.Internal, "recommendation client initialization failed")
		}
		if rights {
			in := costexplorer.GetRightsizingRecommendationInput{Service: aws.String("AmazonEC2"), Configuration: &types.RightsizingRecommendationConfiguration{BenefitsConsidered: true, RecommendationTarget: types.RecommendationTargetSameInstanceFamily}, Filter: recommendationAWSFilter(f)}
			data, err := ce.ListRightsizingRecommendations(ctx, in)
			if err != nil {
				return nil, recommendationFailure(err)
			}
			mapped, err := mapRightsizingRecommendations(data)
			if err != nil {
				return nil, status.Error(codes.DataLoss, "invalid AWS recommendation data")
			}
			recs = append(recs, mapped...)
		}
		if plans {
			for _, kind := range []types.SupportedSavingsPlansType{types.SupportedSavingsPlansTypeComputeSp, types.SupportedSavingsPlansTypeEc2InstanceSp} {
				in := costexplorer.GetSavingsPlansPurchaseRecommendationInput{SavingsPlansType: kind, LookbackPeriodInDays: types.LookbackPeriodInDaysThirtyDays, TermInYears: types.TermInYearsOneYear, PaymentOption: types.PaymentOptionNoUpfront, AccountScope: types.AccountScopePayer, Filter: recommendationAWSFilter(f)}
				data, err := ce.ListSavingsPlansRecommendations(ctx, in)
				if err != nil {
					return nil, recommendationFailure(err)
				}
				mapped, err := mapSavingsPlansRecommendations(data)
				if err != nil {
					return nil, status.Error(codes.DataLoss, "invalid AWS recommendation data")
				}
				recs = append(recs, mapped...)
			}
		}
	}
	if err := projectRecommendationSavings(recs, period); err != nil {
		return nil, status.Error(codes.DataLoss, "invalid AWS recommendation projection")
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
