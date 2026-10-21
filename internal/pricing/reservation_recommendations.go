package pricing

import (
	"encoding/json"
	"fmt"
	"math"

	"github.com/aws/aws-sdk-go-v2/aws"
	"github.com/aws/aws-sdk-go-v2/service/costexplorer"
	"github.com/aws/aws-sdk-go-v2/service/costexplorer/types"
	"github.com/rshade/finfocus-spec/sdk/go/pluginsdk"
	pbc "github.com/rshade/finfocus-spec/sdk/go/proto/finfocus/v1"
)

type reservationService struct {
	code, name, resourceType string
}

// Cost Explorer expects billing service names, rather than service codes.
// EC2 reservations are deliberately excluded in favor of Savings Plans.
var reservationServices = []reservationService{
	{"AmazonRDS", "Amazon Relational Database Service", "rds:Account"},
	{"AmazonRedshift", "Amazon Redshift", "redshift:Account"},
	{"AmazonElastiCache", "Amazon ElastiCache", "elasticache:Account"},
	{"AmazonES", "Amazon Elasticsearch Service", "es:Account"},
}

func selectedReservationServices(req *pbc.GetRecommendationsRequest) []reservationService {
	f := req.GetFilter()
	if f.GetProvider() != "" && f.GetProvider() != "aws" ||
		f.GetSource() != "" && f.GetSource() != recommendationSource ||
		f.GetCategory() != 0 && f.GetCategory() != pbc.RecommendationCategory_RECOMMENDATION_CATEGORY_COST ||
		f.GetActionType() != 0 && f.GetActionType() != pbc.RecommendationActionType_RECOMMENDATION_ACTION_TYPE_PURCHASE_COMMITMENT {
		return nil
	}
	var selected []reservationService
	for _, service := range reservationServices {
		if kind := canonicalRecommendationType(f.GetResourceType()); kind != "" && kind != service.resourceType {
			continue
		}
		if len(req.GetTargetResources()) > 0 {
			match := false
			for _, target := range req.GetTargetResources() {
				if target.GetProvider() == "aws" && canonicalRecommendationType(target.GetResourceType()) == service.resourceType {
					match = true
					break
				}
			}
			if !match {
				continue
			}
		}
		selected = append(selected, service)
	}
	return selected
}

func mapReservationRecommendations(serviceCode string, pages []costexplorer.GetReservationPurchaseRecommendationOutput) ([]*pbc.Recommendation, error) {
	var service reservationService
	for _, candidate := range reservationServices {
		if candidate.code == serviceCode {
			service = candidate
			break
		}
	}
	if service.code == "" {
		return nil, fmt.Errorf("unsupported reservation service")
	}
	var result []*pbc.Recommendation
	for _, page := range pages {
		for _, group := range page.Recommendations {
			for _, detail := range group.RecommendationDetails {
				r, err := mapReservationDetail(service, group, detail)
				if err != nil {
					return nil, err
				}
				if page.Metadata != nil {
					if err := setRecommendationTime(r, aws.ToString(page.Metadata.GenerationTimestamp)); err != nil {
						return nil, err
					}
					r.Metadata["aws_recommendation_set_id"] = aws.ToString(page.Metadata.RecommendationId)
				}
				if err := pluginsdk.ValidateRecommendation(r); err != nil {
					return nil, fmt.Errorf("invalid reservation recommendation: %w", err)
				}
				result = append(result, r)
			}
		}
	}
	return result, nil
}

func mapReservationDetail(service reservationService, group types.ReservationPurchaseRecommendation, d types.ReservationPurchaseRecommendationDetail) (*pbc.Recommendation, error) {
	account := aws.ToString(d.AccountId)
	if account == "" && group.AccountScope != types.AccountScopePayer {
		return nil, fmt.Errorf("missing reservation account")
	}
	label := account
	if label == "" {
		label = "payer"
	}
	region, sku, err := reservationInstanceDetails(service.code, d.InstanceDetails)
	if err != nil {
		return nil, err
	}
	quantity, err := recommendationMoney(aws.ToString(d.RecommendedNumberOfInstancesToPurchase))
	if err != nil || quantity <= 0 || math.Trunc(quantity) != quantity || quantity > 1<<53 {
		return nil, fmt.Errorf("invalid reservation quantity")
	}
	currency := aws.ToString(d.CurrencyCode)
	if currency == "" && group.RecommendationSummary != nil {
		currency = aws.ToString(group.RecommendationSummary.CurrencyCode)
	}
	r, err := newAWSRecommendation(label, "account:"+label+":reserved-instance:"+service.code, service.resourceType, aws.ToString(d.EstimatedMonthlySavingsAmount), currency)
	if err != nil {
		return nil, err
	}
	r.Resource.Region, r.Resource.Sku = region, sku
	r.ActionType = pbc.RecommendationActionType_RECOMMENDATION_ACTION_TYPE_PURCHASE_COMMITMENT
	r.ActionDetail = &pbc.Recommendation_Commitment{Commitment: &pbc.CommitmentAction{
		CommitmentType: "reserved_instance", Term: string(group.TermInYears), PaymentOption: string(group.PaymentOption),
		RecommendedQuantity: quantity, Scope: "account",
	}}
	m := r.Metadata
	if account == "" {
		delete(m, "account_id")
	}
	m["service"], m["aws_service"] = service.code, service.name
	m["term"], m["payment_option"] = string(group.TermInYears), string(group.PaymentOption)
	m["account_scope"], m["lookback_period"] = string(group.AccountScope), string(group.LookbackPeriodInDays)
	m["recommended_quantity"] = aws.ToString(d.RecommendedNumberOfInstancesToPurchase)
	for key, value := range map[string]*string{
		"upfront_cost": d.UpfrontCost, "recurring_monthly_cost": d.RecurringStandardMonthlyCost,
		"estimated_break_even_months": d.EstimatedBreakEvenInMonths, "estimated_monthly_on_demand_cost": d.EstimatedMonthlyOnDemandCost,
		"estimated_monthly_savings_percentage": d.EstimatedMonthlySavingsPercentage,
	} {
		if value != nil {
			if _, err := recommendationMoney(*value); err != nil {
				return nil, err
			}
			m[key] = *value
		}
	}
	// Service-specific fields distinguish, for example, database engines, license
	// models and deployment options sharing the same account, region and SKU.
	identity, err := json.Marshal(d.InstanceDetails)
	if err != nil {
		return nil, fmt.Errorf("invalid reservation instance details")
	}
	m["instance_details"] = string(identity)
	r.Id = recommendationID(label, service.code, r.Resource.Id, string(group.AccountScope), string(group.TermInYears), string(group.PaymentOption), string(group.LookbackPeriodInDays), string(identity))
	return r, nil
}

func reservationInstanceDetails(service string, details *types.InstanceDetails) (string, string, error) {
	if details == nil {
		return "", "", fmt.Errorf("missing reservation instance details")
	}
	var region, sku string
	switch {
	case service == "AmazonRDS" && details.RDSInstanceDetails != nil:
		d := details.RDSInstanceDetails
		region, sku = aws.ToString(d.Region), aws.ToString(d.InstanceType)
	case service == "AmazonRedshift" && details.RedshiftInstanceDetails != nil:
		d := details.RedshiftInstanceDetails
		region, sku = aws.ToString(d.Region), aws.ToString(d.NodeType)
	case service == "AmazonElastiCache" && details.ElastiCacheInstanceDetails != nil:
		d := details.ElastiCacheInstanceDetails
		region, sku = aws.ToString(d.Region), aws.ToString(d.NodeType)
	case service == "AmazonES" && details.ESInstanceDetails != nil:
		d := details.ESInstanceDetails
		region = aws.ToString(d.Region)
		if aws.ToString(d.InstanceClass) != "" && aws.ToString(d.InstanceSize) != "" {
			sku = aws.ToString(d.InstanceClass) + "." + aws.ToString(d.InstanceSize)
		}
	}
	if region == "" || sku == "" {
		return "", "", fmt.Errorf("missing or mismatched reservation service, region or instance type")
	}
	return region, sku, nil
}
