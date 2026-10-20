package pricing

import (
	"context"
	"errors"
	"fmt"
	"os"
	"regexp"
	"strconv"
	"strings"
	"time"

	"github.com/rs/zerolog"
	"github.com/rshade/finfocus-plugin-aws-ce/internal/client"
	"github.com/rshade/finfocus-spec/sdk/go/pluginsdk"
	pbc "github.com/rshade/finfocus-spec/sdk/go/proto/finfocus/v1"
	"google.golang.org/grpc/codes"
	"google.golang.org/grpc/status"
	"google.golang.org/protobuf/types/known/structpb"
)

type spotHourly interface {
	LatestSpotPrice(ctx context.Context, region, instanceType, product, availabilityZone string) (float64, error)
}

type onDemandHourly interface {
	OnDemandHourly(ctx context.Context, region, instanceType, operatingSystem string) (float64, bool, error)
}

type interruptionLookup interface {
	Interruption(ctx context.Context, region, operatingSystem, instanceType string) (spotInterruption, error)
}

type spotInterruption struct {
	Label string
	Score float64
	Known bool
}

type emptyInterruption struct{}

func (emptyInterruption) Interruption(context.Context, string, string, string) (spotInterruption, error) {
	return spotInterruption{}, nil
}

type spotQuery struct {
	region           string
	instanceType     string
	availabilityZone string
	product          string
	advisorOS        string
	priceListOS      string
}

var (
	instanceTypePattern = regexp.MustCompile(`^[a-z0-9-]+\.[a-z0-9.]+$`)
	awsNamePattern      = regexp.MustCompile(`^[a-z0-9-]+$`)
)

func spotComparisonFromEnv() bool {
	return strings.EqualFold(strings.TrimSpace(os.Getenv(SpotComparisonEnv)), "true")
}

func wantsSpotComparison(resource *pbc.ResourceDescriptor) bool {
	return isEC2Instance(resource.GetResourceType()) && instanceTypeOf(resource) != "" && availabilityZoneOf(resource) != ""
}

func isEC2Instance(resourceType string) bool {
	kind := strings.ToLower(strings.TrimSpace(resourceType))
	switch kind {
	case "ec2", "aws:ec2", strings.ToLower(pulumiEC2InstanceType):
		return true
	default:
		return strings.Contains(kind, "ec2/instance")
	}
}

func (c *Calculator) quoteSpot(ctx context.Context, logger zerolog.Logger, req *pbc.GetProjectedCostRequest) (*pbc.GetProjectedCostResponse, error) {
	query, err := parseSpotQuery(req.GetResource())
	if err != nil {
		return nil, err
	}
	spots := c.spotQuote
	demands := c.onDemandQuote
	risks := c.spotRisk
	perRequest := false
	if spots == nil || demands == nil {
		ce, per, callErr := c.clientForCall(ctx, logger)
		if callErr != nil {
			if _, ok := status.FromError(callErr); ok {
				return nil, callErr
			}
			return nil, status.Errorf(codes.Internal, "client initialization failed: %v", callErr)
		}
		perRequest = per
		if spots == nil {
			spots = ce
		}
		if demands == nil {
			demands = ce
		}
	}
	if risks == nil {
		if pluginsdk.IsTestMode() {
			risks = emptyInterruption{}
		} else {
			risks = defaultSpotAdvisor
		}
	}
	logger.Debug().Str("instance_type", query.instanceType).Str("region", query.region).Str("availability_zone", query.availabilityZone).Str("product", query.product).Msg("spot quote")

	hourly, err := spots.LatestSpotPrice(ctx, query.region, query.instanceType, query.product, query.availabilityZone)
	if err != nil {
		return nil, c.spotCallError(logger, perRequest, err)
	}
	onDemand, hasOnDemand, err := demands.OnDemandHourly(ctx, query.region, query.instanceType, query.priceListOS)
	if err != nil {
		return nil, c.spotCallError(logger, perRequest, err)
	}
	risk, err := risks.Interruption(ctx, query.region, query.advisorOS, query.instanceType)
	if err != nil {
		logger.Warn().Msg("spot interruption feed unavailable")
		risk = spotInterruption{}
	}
	hours := pluginsdk.DefaultMonthlyHours(req.GetUsageProfile())
	resp := buildSpotResponse(query, hourly, hours, onDemand, hasOnDemand, risk, time.Now().UTC())
	if err := pluginsdk.ValidateGetProjectedCostResponse(resp); err != nil {
		return nil, status.Error(codes.Internal, "spot quote response is invalid")
	}
	if resp.GetSpotInterruptionRiskScore() == 0 {
		logger.Warn().Str("instance_type", query.instanceType).Msg("spot interruption score unavailable")
	}
	return resp, nil
}

func (c *Calculator) spotCallError(logger zerolog.Logger, perRequest bool, err error) error {
	if errors.Is(err, client.ErrNoSpotPrice) {
		return status.Error(codes.NotFound, "no spot price for the instance type and availability zone")
	}
	if errors.Is(err, client.ErrSpotClientUnconfigured) {
		return status.Error(codes.FailedPrecondition, client.ErrSpotClientUnconfigured.Error())
	}
	if errors.Is(err, client.ErrSpotHistoryTruncated) || errors.Is(err, client.ErrPriceListTruncated) {
		return status.Error(codes.Internal, "spot quote page limit reached")
	}
	if mapped, ok := mapAWSAPIError(err); ok {
		if perRequest {
			logger.Error().Msg(status.Convert(mapped).Message())
		}
		return mapped
	}
	return status.Error(codes.Internal, "getting spot quote failed")
}

func parseSpotQuery(resource *pbc.ResourceDescriptor) (spotQuery, error) {
	instanceType := instanceTypeOf(resource)
	if !instanceTypePattern.MatchString(instanceType) {
		return spotQuery{}, status.Error(codes.InvalidArgument, "spot quote instance type is invalid")
	}
	zone := availabilityZoneOf(resource)
	if !awsNamePattern.MatchString(zone) {
		return spotQuery{}, status.Error(codes.InvalidArgument, "spot quote availability zone is invalid")
	}
	region := strings.TrimSpace(resource.GetRegion())
	if !awsNamePattern.MatchString(region) {
		return spotQuery{}, status.Error(codes.InvalidArgument, "spot quote requires a region")
	}
	product, advisorOS, priceListOS, ok := spotOperatingSystem(resource)
	if !ok {
		return spotQuery{}, status.Error(codes.InvalidArgument, "spot quote operating system is unsupported")
	}
	return spotQuery{
		region:           region,
		instanceType:     instanceType,
		availabilityZone: zone,
		product:          product,
		advisorOS:        advisorOS,
		priceListOS:      priceListOS,
	}, nil
}

func buildSpotResponse(query spotQuery, hourly, hours, onDemand float64, hasOnDemand bool, risk spotInterruption, now time.Time) *pbc.GetProjectedCostResponse {
	monthly := hourly * hours
	detail := fmt.Sprintf("Spot %s %s in %s at %s USD per hour for %s hours", query.product, query.instanceType, query.availabilityZone, strconv.FormatFloat(hourly, 'f', -1, 64), strconv.FormatFloat(hours, 'f', -1, 64))
	opts := []pluginsdk.GetProjectedCostResponseOption{
		pluginsdk.WithProjectedCostDetails(hourly, "USD", monthly, detail),
		pluginsdk.WithProjectedCostPricingCategory(pbc.FocusPricingCategory_FOCUS_PRICING_CATEGORY_DYNAMIC),
		pluginsdk.WithProjectedCostExpiresAt(now.Add(client.PriceCacheTTL)),
	}
	if risk.Known && risk.Score > 0 {
		opts = append(opts, pluginsdk.WithProjectedCostSpotRisk(risk.Score))
	}
	if hasOnDemand {
		onDemandMonthly := onDemand * hours
		opts = append(opts,
			pluginsdk.WithProjectedCostOnDemandEquivalent(onDemandMonthly),
			pluginsdk.WithProjectedCostPriceOptions(&pbc.PriceOption{
				Category:        pbc.FocusPricingCategory_FOCUS_PRICING_CATEGORY_STANDARD,
				Model:           "On-Demand",
				UnitPrice:       onDemand,
				MonthlyCost:     onDemandMonthly,
				SavingsFraction: savingsFraction(hourly, onDemand),
			}),
		)
	}
	resp := pluginsdk.NewGetProjectedCostResponse(opts...)
	resp.Metadata = map[string]string{
		"spot_availability_zone":   query.availabilityZone,
		"spot_product_description": query.product,
	}
	if risk.Label != "" {
		resp.Metadata["spot_interruption_label"] = risk.Label
	}
	return resp
}

func savingsFraction(primary, option float64) float64 {
	if primary == 0 {
		return 0
	}
	return (primary - option) / primary
}

func instanceTypeOf(resource *pbc.ResourceDescriptor) string {
	if value := strings.ToLower(firstAttribute(resource.GetAttributes(), "instanceType", "instance_type")); value != "" {
		return value
	}
	if sku := strings.ToLower(strings.TrimSpace(resource.GetSku())); sku != "" {
		return sku
	}
	return strings.ToLower(firstTag(resource.GetTags(), "instanceType", "instance_type"))
}

func availabilityZoneOf(resource *pbc.ResourceDescriptor) string {
	if value := firstAttribute(resource.GetAttributes(), "availabilityZone", "availability_zone"); value != "" {
		return value
	}
	return firstTag(resource.GetTags(), "availabilityZone", "availability_zone")
}

func spotOperatingSystem(resource *pbc.ResourceDescriptor) (product, advisorOS, priceListOS string, ok bool) {
	raw := firstAttribute(resource.GetAttributes(), "operatingSystem", "operating_system", "platform")
	if raw == "" {
		raw = firstTag(resource.GetTags(), "operatingSystem", "operating_system", "platform", "os")
	}
	switch strings.ToLower(strings.TrimSpace(raw)) {
	case "", "linux", "linux/unix":
		return "Linux/UNIX", "Linux", "Linux", true
	case "windows":
		return "Windows", "Windows", "Windows", true
	case "rhel", "red hat enterprise linux":
		return "Red Hat Enterprise Linux", "Linux", "RHEL", true
	case "suse", "suse linux":
		return "SUSE Linux", "Linux", "SUSE", true
	default:
		return "", "", "", false
	}
}

func firstAttribute(attrs *structpb.Struct, paths ...string) string {
	for _, path := range paths {
		value, ok := pluginsdk.AttributeValue(attrs, path)
		if !ok {
			continue
		}
		if text := strings.TrimSpace(value.GetStringValue()); text != "" {
			return text
		}
	}
	return ""
}

func firstTag(tags map[string]string, names ...string) string {
	for _, name := range names {
		for key, value := range tags {
			if strings.EqualFold(key, name) {
				if text := strings.TrimSpace(value); text != "" {
					return text
				}
			}
		}
	}
	return ""
}
