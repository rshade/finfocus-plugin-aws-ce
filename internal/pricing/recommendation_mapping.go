package pricing

import (
	"crypto/sha256"
	"encoding/json"
	"fmt"
	"math"
	"regexp"
	"strconv"
	"time"

	"github.com/aws/aws-sdk-go-v2/aws"
	"github.com/aws/aws-sdk-go-v2/service/costexplorer"
	"github.com/aws/aws-sdk-go-v2/service/costexplorer/types"
	"github.com/rshade/finfocus-spec/sdk/go/pluginsdk"
	pbc "github.com/rshade/finfocus-spec/sdk/go/proto/finfocus/v1"
	"google.golang.org/protobuf/types/known/timestamppb"
)

const recommendationSource = "aws-cost-explorer"

// recommendationID hashes an unambiguous tuple, independent of page order or retrieval time.
func recommendationID(parts ...string) string {
	b, _ := json.Marshal(parts)
	return fmt.Sprintf("%s:%x", recommendationSource, sha256.Sum256(b))
}

var recommendationDecimalPattern = regexp.MustCompile(`^[0-9]+(?:\.[0-9]+)?(?:[eE][+-]?[0-9]+)?$`)

func recommendationMoney(s string) (float64, error) {
	if !recommendationDecimalPattern.MatchString(s) {
		return 0, fmt.Errorf("invalid recommendation financial value")
	}
	v, e := strconv.ParseFloat(s, 64)
	if e != nil || math.IsNaN(v) || math.IsInf(v, 0) || v < 0 {
		return 0, fmt.Errorf("invalid recommendation financial value")
	}
	return v, nil
}
func newAWSRecommendation(account, resource, kind, monthly, currency string) (*pbc.Recommendation, error) {
	v, e := recommendationMoney(monthly)
	if e != nil {
		return nil, e
	}
	if currency == "" {
		return nil, fmt.Errorf("missing recommendation currency")
	}
	return &pbc.Recommendation{
		Category: pbc.RecommendationCategory_RECOMMENDATION_CATEGORY_COST,
		Priority: pbc.RecommendationPriority_RECOMMENDATION_PRIORITY_MEDIUM,
		Source:   recommendationSource,
		Resource: &pbc.ResourceRecommendationInfo{Id: resource, Provider: "aws", ResourceType: kind},
		Impact:   &pbc.RecommendationImpact{EstimatedSavings: v, Currency: currency, ProjectionPeriod: "monthly"},
		Metadata: map[string]string{
			"account_id":            account,
			"monthly_savings":       monthly,
			"projection_convention": "daily=monthly/30;monthly=monthly;annual=monthly*12",
		},
	}, nil
}
func setRecommendationTime(r *pbc.Recommendation, s string) error {
	if s == "" {
		return nil
	}
	v, e := time.Parse(time.RFC3339Nano, s)
	if e != nil {
		return fmt.Errorf("invalid recommendation generation timestamp")
	}
	r.CreatedAt = timestamppb.New(v.UTC())
	return r.CreatedAt.CheckValid()
}
func ec2Details(d *types.ResourceDetails) *types.EC2ResourceDetails {
	if d == nil {
		return nil
	}
	return d.EC2ResourceDetails
}

// mapRightsizingRecommendations selects the first AWS default target, or the first alternative when none is flagged.
func mapRightsizingRecommendations(pages []costexplorer.GetRightsizingRecommendationOutput) ([]*pbc.Recommendation, error) {
	var result []*pbc.Recommendation
	for _, page := range pages {
		for _, item := range page.RightsizingRecommendations {
			c := item.CurrentInstance
			if c == nil || aws.ToString(c.ResourceId) == "" || aws.ToString(item.AccountId) == "" {
				return nil, fmt.Errorf("missing rightsizing identity")
			}
			var monthly, currency, target string
			currency = aws.ToString(c.CurrencyCode)
			switch item.RightsizingType {
			case types.RightsizingTypeTerminate:
				if item.TerminateRecommendationDetail == nil {
					return nil, fmt.Errorf("missing terminate detail")
				}
				monthly = aws.ToString(item.TerminateRecommendationDetail.EstimatedMonthlySavings)
				if s := aws.ToString(item.TerminateRecommendationDetail.CurrencyCode); s != "" {
					currency = s
				}
			case types.RightsizingTypeModify:
				if item.ModifyRecommendationDetail == nil || len(item.ModifyRecommendationDetail.TargetInstances) == 0 {
					return nil, fmt.Errorf("missing modify target")
				}
				t := item.ModifyRecommendationDetail.TargetInstances[0]
				for _, candidate := range item.ModifyRecommendationDetail.TargetInstances {
					if candidate.DefaultTargetInstance {
						t = candidate
						break
					}
				}
				monthly = aws.ToString(t.EstimatedMonthlySavings)
				if s := aws.ToString(t.CurrencyCode); s != "" {
					currency = s
				}
				if d := ec2Details(t.ResourceDetails); d != nil {
					target = aws.ToString(d.InstanceType)
				}
				if target == "" {
					return nil, fmt.Errorf("missing target instance type")
				}
			default:
				return nil, fmt.Errorf("unknown rightsizing action")
			}
			r, e := newAWSRecommendation(aws.ToString(item.AccountId), aws.ToString(c.ResourceId), "ec2:Instance", monthly, currency)
			if e != nil {
				return nil, e
			}
			r.Resource.Name = aws.ToString(c.InstanceName)
			if d := ec2Details(c.ResourceDetails); d != nil {
				r.Resource.Sku = aws.ToString(d.InstanceType)
				r.Resource.Region = aws.ToString(d.Region)
			}
			r.Resource.Tags = map[string]string{}
			for _, tag := range c.Tags {
				if len(tag.Values) == 1 {
					r.Resource.Tags[aws.ToString(tag.Key)] = tag.Values[0]
				}
			}
			if item.RightsizingType == types.RightsizingTypeTerminate {
				r.ActionType = pbc.RecommendationActionType_RECOMMENDATION_ACTION_TYPE_TERMINATE
				r.ActionDetail = &pbc.Recommendation_Terminate{Terminate: &pbc.TerminateAction{}}
			} else {
				r.ActionType = pbc.RecommendationActionType_RECOMMENDATION_ACTION_TYPE_RIGHTSIZE
				r.ActionDetail = &pbc.Recommendation_Rightsize{Rightsize: &pbc.RightsizeAction{CurrentSku: r.Resource.Sku, CurrentInstanceType: r.Resource.Sku, RecommendedSku: target, RecommendedInstanceType: target}}
			}
			r.Id = recommendationID(aws.ToString(item.AccountId), recommendationSource, r.Resource.Id, r.Resource.Region, r.ActionType.String(), target)
			if page.Metadata != nil {
				if e = setRecommendationTime(r, aws.ToString(page.Metadata.GenerationTimestamp)); e != nil {
					return nil, e
				}
				r.Metadata["aws_recommendation_set_id"] = aws.ToString(page.Metadata.RecommendationId)
			}
			if e = pluginsdk.ValidateRecommendation(r); e != nil {
				return nil, fmt.Errorf("invalid rightsizing recommendation: %w", e)
			}
			result = append(result, r)
		}
	}
	return result, nil
}

// mapSavingsPlansRecommendations describes account purchase opportunities, not existing instances.
func mapSavingsPlansRecommendations(pages []costexplorer.GetSavingsPlansPurchaseRecommendationOutput) ([]*pbc.Recommendation, error) {
	var result []*pbc.Recommendation
	for _, page := range pages {
		p := page.SavingsPlansPurchaseRecommendation
		if p == nil {
			continue
		}
		if p.SavingsPlansType != types.SupportedSavingsPlansTypeComputeSp && p.SavingsPlansType != types.SupportedSavingsPlansTypeEc2InstanceSp {
			return nil, fmt.Errorf("unsupported savings plan type")
		}
		for _, d := range p.SavingsPlansPurchaseRecommendationDetails {
			account := aws.ToString(d.AccountId)
			if account == "" && p.AccountScope != types.AccountScopePayer {
				return nil, fmt.Errorf("missing savings plan account")
			}
			// AWS may omit AccountId for payer-scoped details, so label them by scope.
			label := account
			if label == "" {
				label = "payer"
			}
			currency := aws.ToString(d.CurrencyCode)
			if currency == "" && p.SavingsPlansPurchaseRecommendationSummary != nil {
				currency = aws.ToString(p.SavingsPlansPurchaseRecommendationSummary.CurrencyCode)
			}
			r, e := newAWSRecommendation(label, "account:"+label+":savings-plan", "savingsplans:Account", aws.ToString(d.EstimatedMonthlySavingsAmount), currency)
			if e != nil {
				return nil, e
			}
			if account == "" {
				delete(r.Metadata, "account_id")
			}
			r.ActionType = pbc.RecommendationActionType_RECOMMENDATION_ACTION_TYPE_PURCHASE_COMMITMENT
			r.ActionDetail = &pbc.Recommendation_Commitment{Commitment: &pbc.CommitmentAction{CommitmentType: "savings_plan", Term: string(p.TermInYears), PaymentOption: string(p.PaymentOption), Scope: "account"}}
			m := r.Metadata
			m["plan_type"] = string(p.SavingsPlansType)
			m["term"] = string(p.TermInYears)
			m["payment_option"] = string(p.PaymentOption)
			m["lookback_period"] = string(p.LookbackPeriodInDays)
			m["account_scope"] = string(p.AccountScope)
			for key, value := range map[string]*string{"hourly_commitment": d.HourlyCommitmentToPurchase, "upfront_cost": d.UpfrontCost} {
				if value != nil {
					if _, e = recommendationMoney(*value); e != nil {
						return nil, e
					}
					m[key] = *value
				}
			}
			offering := ""
			if d.SavingsPlansDetails != nil {
				r.Resource.Region = aws.ToString(d.SavingsPlansDetails.Region)
				r.Resource.Sku = aws.ToString(d.SavingsPlansDetails.InstanceFamily)
				offering = aws.ToString(d.SavingsPlansDetails.OfferingId)
				m["offering_id"] = offering
			}
			detailID := aws.ToString(d.RecommendationDetailId)
			if detailID != "" {
				r.Id = recommendationSource + ":" + label + ":" + detailID
			} else {
				r.Id = recommendationID(label, recommendationSource, r.Resource.Id, r.ActionType.String(), string(p.SavingsPlansType), r.Resource.Region, r.Resource.Sku, offering, string(p.TermInYears), string(p.PaymentOption), aws.ToString(d.HourlyCommitmentToPurchase), aws.ToString(d.UpfrontCost))
			}
			if page.Metadata != nil {
				if e = setRecommendationTime(r, aws.ToString(page.Metadata.GenerationTimestamp)); e != nil {
					return nil, e
				}
				m["aws_recommendation_set_id"] = aws.ToString(page.Metadata.RecommendationId)
			}
			if e = pluginsdk.ValidateRecommendation(r); e != nil {
				return nil, fmt.Errorf("invalid savings plan recommendation: %w", e)
			}
			result = append(result, r)
		}
	}
	return result, nil
}

// projectRecommendationSavings always projects from the original monthly estimate.
func projectRecommendationSavings(recs []*pbc.Recommendation, period string) error {
	if period == "" {
		period = "monthly"
	}
	factor := 1.0
	switch period {
	case "monthly":
	case "daily":
		factor = 1.0 / 30
	case "annual":
		factor = 12
	default:
		return fmt.Errorf("unsupported recommendation projection")
	}
	for _, r := range recs {
		if r == nil || r.Impact == nil {
			return fmt.Errorf("missing recommendation impact")
		}
		monthly, e := recommendationMoney(r.Metadata["monthly_savings"])
		if e != nil {
			return e
		}
		v := monthly * factor
		if math.IsInf(v, 0) || math.IsNaN(v) {
			return fmt.Errorf("recommendation projection overflow")
		}
		r.Impact.EstimatedSavings = v
		r.Impact.ProjectionPeriod = period
	}
	return nil
}
