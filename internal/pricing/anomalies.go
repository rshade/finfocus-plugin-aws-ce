package pricing

import (
	"context"
	"strconv"
	"strings"
	"time"

	"github.com/aws/aws-sdk-go-v2/aws"
	"github.com/aws/aws-sdk-go-v2/service/costexplorer/types"
	"github.com/rshade/finfocus-spec/sdk/go/pluginsdk"
	pbc "github.com/rshade/finfocus-spec/sdk/go/proto/finfocus/v1"
	"google.golang.org/grpc/codes"
	"google.golang.org/grpc/status"
	"google.golang.org/protobuf/types/known/timestamppb"
)

const anomalyLookbackDays = 90

// GetRecommendations reports Cost Explorer anomalies. It does not score or
// detect anomalies itself. Other recommendation categories stay empty.
func (c *Calculator) GetRecommendations(ctx context.Context, req *pbc.GetRecommendationsRequest) (*pbc.GetRecommendationsResponse, error) {
	logger := c.traceLogger(ctx)
	done := pluginsdk.LogOperation(logger, "GetRecommendations")
	defer done()
	if req == nil {
		req = &pbc.GetRecommendationsRequest{}
	}
	if !anomalyRequest(req) {
		return emptyRecommendations(req.GetProjectionPeriod()), nil
	}
	if len(req.GetTargetResources()) > 0 {
		return nil, status.Error(codes.InvalidArgument, "Cost Explorer anomalies are account scoped and do not accept target resources")
	}

	start, end := anomalyWindow(time.Now().UTC())
	var pages int
	ctx = c.withCostBudget(ctx, &pages)
	ce, _, err := c.clientForCall(ctx, logger)
	if err != nil {
		if _, ok := status.FromError(err); ok {
			return nil, err
		}
		return nil, status.Errorf(codes.Internal, "client initialization failed: %v", err)
	}
	output, err := ce.GetAnomalies(ctx, start, end, req.GetPageToken(), req.GetPageSize())
	if err != nil {
		if mapped, ok := mapAWSAPIError(err); ok {
			return nil, mapped
		}
		return nil, status.Error(codes.Internal, "getting anomalies failed")
	}
	if output == nil {
		return emptyRecommendations(req.GetProjectionPeriod()), nil
	}
	return anomaliesResponse(output.Anomalies, aws.ToString(output.NextPageToken), req), nil
}

func anomalyRequest(req *pbc.GetRecommendationsRequest) bool {
	filter := req.GetFilter()
	if filter == nil {
		return true
	}
	category := filter.GetCategory()
	if category != pbc.RecommendationCategory_RECOMMENDATION_CATEGORY_UNSPECIFIED &&
		category != pbc.RecommendationCategory_RECOMMENDATION_CATEGORY_ANOMALY {
		return false
	}
	action := filter.GetActionType()
	if action != pbc.RecommendationActionType_RECOMMENDATION_ACTION_TYPE_UNSPECIFIED &&
		action != pbc.RecommendationActionType_RECOMMENDATION_ACTION_TYPE_INVESTIGATE {
		return false
	}
	source := filter.GetSource()
	return source == "" || source == "aws-cost-explorer"
}

func anomalyWindow(now time.Time) (string, string) {
	end := time.Date(now.Year(), now.Month(), now.Day(), 0, 0, 0, 0, time.UTC)
	start := end.AddDate(0, 0, -(anomalyLookbackDays - 1))
	return start.Format("2006-01-02"), end.Format("2006-01-02")
}

func emptyRecommendations(period string) *pbc.GetRecommendationsResponse {
	if period == "" {
		period = "monthly"
	}
	return &pbc.GetRecommendationsResponse{
		Recommendations: []*pbc.Recommendation{},
		Summary: &pbc.RecommendationSummary{
			ProjectionPeriod: period,
			Currency:         "USD",
		},
	}
}

func anomaliesResponse(rows []types.Anomaly, next string, req *pbc.GetRecommendationsRequest) *pbc.GetRecommendationsResponse {
	excluded := map[string]struct{}{}
	for _, id := range req.GetExcludedRecommendationIds() {
		excluded[id] = struct{}{}
	}
	period := req.GetProjectionPeriod()
	if period == "" {
		period = "monthly"
	}
	out := make([]*pbc.Recommendation, 0, len(rows))
	for _, row := range rows {
		id := aws.ToString(row.AnomalyId)
		if _, skip := excluded[id]; skip {
			continue
		}
		out = append(out, mapAnomaly(row))
	}
	category := pbc.RecommendationCategory_RECOMMENDATION_CATEGORY_ANOMALY.String()
	action := pbc.RecommendationActionType_RECOMMENDATION_ACTION_TYPE_INVESTIGATE.String()
	return &pbc.GetRecommendationsResponse{
		Recommendations: out,
		NextPageToken:   next,
		Summary: &pbc.RecommendationSummary{
			TotalRecommendations: int32(len(out)),
			Currency:             "USD",
			ProjectionPeriod:     period,
			CountByCategory:      map[string]int32{category: int32(len(out))},
			CountByActionType:    map[string]int32{action: int32(len(out))},
		},
	}
}

func mapAnomaly(row types.Anomaly) *pbc.Recommendation {
	id := aws.ToString(row.AnomalyId)
	description := aws.ToString(row.DimensionValue)
	if description == "" {
		description = "Cost Explorer anomaly " + id
	}
	rec := &pbc.Recommendation{
		Id:          id,
		Category:    pbc.RecommendationCategory_RECOMMENDATION_CATEGORY_ANOMALY,
		ActionType:  pbc.RecommendationActionType_RECOMMENDATION_ACTION_TYPE_INVESTIGATE,
		Description: description,
		Source:      "aws-cost-explorer",
		Metadata: map[string]string{
			"monitor_arn": aws.ToString(row.MonitorArn),
		},
	}
	if row.AnomalyScore != nil && row.AnomalyScore.CurrentScore >= 0 && row.AnomalyScore.CurrentScore <= 1 {
		score := row.AnomalyScore.CurrentScore
		rec.ConfidenceScore = &score
	}
	if row.AnomalyScore != nil {
		rec.Metadata["current_score"] = strconv.FormatFloat(row.AnomalyScore.CurrentScore, 'f', -1, 64)
		rec.Metadata["max_score"] = strconv.FormatFloat(row.AnomalyScore.MaxScore, 'f', -1, 64)
	}
	if row.Impact != nil {
		rec.Impact = &pbc.RecommendationImpact{
			Currency:         "USD",
			CurrentCost:      aws.ToFloat64(row.Impact.TotalActualSpend),
			ProjectedCost:    aws.ToFloat64(row.Impact.TotalExpectedSpend),
			ProjectionPeriod: "anomaly",
		}
		rec.Metadata["total_impact"] = strconv.FormatFloat(row.Impact.TotalImpact, 'f', -1, 64)
		rec.Metadata["max_impact"] = strconv.FormatFloat(row.Impact.MaxImpact, 'f', -1, 64)
	}
	if row.AnomalyEndDate != nil {
		if parsed, err := time.Parse("2006-01-02", *row.AnomalyEndDate); err == nil {
			rec.CreatedAt = timestamppb.New(parsed)
		}
	}
	if len(row.RootCauses) > 0 {
		cause := row.RootCauses[0]
		rec.Resource = &pbc.ResourceRecommendationInfo{
			Provider:     "aws",
			Name:         aws.ToString(cause.Service),
			ResourceType: aws.ToString(cause.Service),
			Region:       aws.ToString(cause.Region),
			Sku:          aws.ToString(cause.UsageType),
		}
		var reasons []string
		for _, item := range row.RootCauses {
			reasons = append(reasons, strings.TrimSpace(strings.Join([]string{
				aws.ToString(item.Service), aws.ToString(item.Region), aws.ToString(item.UsageType),
			}, " ")))
		}
		rec.Reasoning = reasons
	}
	return rec
}
