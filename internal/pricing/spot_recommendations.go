package pricing

import (
	"context"
	"crypto/sha256"
	"encoding/json"
	"errors"
	"fmt"
	"math"
	"strconv"
	"strings"

	"github.com/rs/zerolog"
	"github.com/rshade/finfocus-spec/sdk/go/pluginsdk"
	pbc "github.com/rshade/finfocus-spec/sdk/go/proto/finfocus/v1"
)

const spotRecommendationSource = "aws-spot-pricing"

var (
	errSpotNoOnDemandPrice = errors.New("no on-demand price for the instance type")
	errSpotNotCheaper      = errors.New("spot price is not below the on-demand price")
	errSpotNoResourceID    = errors.New("spot recommendation target has no resource id")
	errSpotInvalid         = errors.New("invalid spot recommendation")
)

func (c *Calculator) spotRecommendations(ctx context.Context, logger zerolog.Logger, targets []*pbc.ResourceDescriptor) []*pbc.Recommendation {
	hours := pluginsdk.DefaultMonthlyHours(pbc.UsageProfile_USAGE_PROFILE_UNSPECIFIED)
	seen := map[string]bool{}
	var recs []*pbc.Recommendation
	for _, target := range targets {
		if !wantsSpotComparison(target) {
			logSpotSkip(logger, target)
			continue
		}
		if ctx.Err() != nil {
			logger.Warn().Msg("spot recommendations stopped: request context ended")
			break
		}
		query, err := parseSpotQuery(target)
		if err != nil {
			logger.Debug().Err(err).Str("resource_id", target.GetId()).Msg("spot recommendation skipped: invalid target")
			continue
		}
		quote, err := c.spotQuoteFor(ctx, logger, query)
		if err != nil {
			logger.Warn().Err(err).Str("resource_id", target.GetId()).Msg("spot recommendation skipped: quote unavailable")
			continue
		}
		rec, err := buildSpotRecommendation(target, query, quote, hours)
		if err != nil {
			event := logger.Warn()
			if errors.Is(err, errSpotInvalid) {
				event = logger.Error()
			}
			event.Err(err).Str("resource_id", target.GetId()).Str("instance_type", query.instanceType).Msg("spot recommendation skipped")
			continue
		}
		if seen[rec.GetId()] {
			continue
		}
		seen[rec.GetId()] = true
		recs = append(recs, rec)
	}
	return recs
}

func spotFilterAllows(f *pbc.RecommendationFilter) bool {
	if f.GetProvider() != "" && f.GetProvider() != "aws" ||
		f.GetSource() != "" && f.GetSource() != spotRecommendationSource ||
		f.GetCategory() != 0 && f.GetCategory() != pbc.RecommendationCategory_RECOMMENDATION_CATEGORY_COST ||
		f.GetActionType() != 0 && f.GetActionType() != pbc.RecommendationActionType_RECOMMENDATION_ACTION_TYPE_MIGRATE {
		return false
	}
	kind := canonicalRecommendationType(f.GetResourceType())
	return kind == "" || kind == "ec2:Instance"
}

func buildSpotRecommendation(target *pbc.ResourceDescriptor, query spotQuery, quote spotQuote, hours float64) (*pbc.Recommendation, error) {
	if !quote.hasOnDemand || quote.onDemand <= 0 {
		return nil, errSpotNoOnDemandPrice
	}
	if quote.hourly >= quote.onDemand {
		return nil, errSpotNotCheaper
	}
	resourceID, account := spotTargetIdentity(target)
	if resourceID == "" {
		return nil, errSpotNoResourceID
	}
	current := roundMoney(quote.onDemand * hours)
	projected := roundMoney(quote.hourly * hours)
	savings := roundMoney(current - projected)
	percentage := roundMoney(savings / current * 100)
	monthlySavings := strconv.FormatFloat(savings, 'f', -1, 64)

	rec := &pbc.Recommendation{
		Id:         spotRecommendationID(resourceID, query),
		Category:   pbc.RecommendationCategory_RECOMMENDATION_CATEGORY_COST,
		ActionType: pbc.RecommendationActionType_RECOMMENDATION_ACTION_TYPE_MIGRATE,
		Priority:   pbc.RecommendationPriority_RECOMMENDATION_PRIORITY_MEDIUM,
		Source:     spotRecommendationSource,
		Resource: &pbc.ResourceRecommendationInfo{
			Id:           resourceID,
			Provider:     "aws",
			ResourceType: "ec2:Instance",
			Sku:          query.instanceType,
			Region:       query.region,
			Tags:         target.GetTags(),
		},
		Impact: &pbc.RecommendationImpact{
			EstimatedSavings:  savings,
			Currency:          "USD",
			ProjectionPeriod:  "monthly",
			CurrentCost:       current,
			ProjectedCost:     projected,
			SavingsPercentage: percentage,
		},
		Description: fmt.Sprintf("Move %s (%s) from on-demand to Spot capacity in %s", resourceID, query.instanceType, spotLocation(query, quote)),
		Reasoning: []string{
			fmt.Sprintf("Spot %s costs %s USD per hour against %s USD per hour on demand, over %s hours.",
				query.product, formatRate(quote.hourly), formatRate(quote.onDemand), formatRate(hours)),
			"Spot capacity can be interrupted with two minutes of notice. Use it for fault-tolerant workloads.",
		},
		PricingCategory: pbc.FocusPricingCategory_FOCUS_PRICING_CATEGORY_DYNAMIC,
		Metadata: map[string]string{
			"monthly_savings":          monthlySavings,
			"projection_convention":    "daily=monthly/30;monthly=monthly;annual=monthly*12",
			"spot_product_description": query.product,
			"spot_hourly_price":        formatRate(quote.hourly),
			"on_demand_hourly_price":   formatRate(quote.onDemand),
		},
	}
	if query.regional() {
		rec.Reasoning = append(rec.Reasoning, fmt.Sprintf("No availability zone is set, so the Spot price is the median of the latest prices in %d zones of %s.", quote.zones, query.region))
	}
	spotBasisMetadata(rec.Metadata, query, quote)
	if account != "" {
		rec.Metadata["account_id"] = account
	}
	if quote.risk.Label != "" {
		rec.Metadata["spot_interruption_label"] = quote.risk.Label
	}
	if quote.risk.Known && quote.risk.Score > 0 {
		rec.SpotInterruptionRiskScore = quote.risk.Score
	}
	if err := pluginsdk.ValidateRecommendation(rec); err != nil {
		return nil, fmt.Errorf("%w: %w", errSpotInvalid, err)
	}
	return rec, nil
}

// spotRecommendationID hashes inputs that do not change with price or risk, so
// a dismissal survives market moves. A region-level target hashes the fixed
// spotRegionalIDKey instead of a zone, so the zone count and the median can move
// without changing the id. An explicit zone hashes exactly as it did before.
func spotRecommendationID(resourceID string, query spotQuery) string {
	location := query.availabilityZone
	if query.regional() {
		location = spotRegionalIDKey
	}
	b, _ := json.Marshal([]string{
		spotRecommendationSource, resourceID, query.region, location,
		query.instanceType, query.product, pbc.RecommendationActionType_RECOMMENDATION_ACTION_TYPE_MIGRATE.String(),
	})
	return fmt.Sprintf("%s:%x", spotRecommendationSource, sha256.Sum256(b))
}

func spotTargetIdentity(target *pbc.ResourceDescriptor) (resourceID, account string) {
	resourceID = target.GetId()
	if parts := strings.SplitN(target.GetArn(), ":", 6); len(parts) == 6 {
		account = parts[4]
		if resourceID == "" {
			resourceID = strings.TrimPrefix(parts[5], "instance/")
		}
	}
	return resourceID, account
}

func roundMoney(v float64) float64 {
	return math.Round(v*1e6) / 1e6
}

func formatRate(v float64) string {
	return strconv.FormatFloat(v, 'f', -1, 64)
}
