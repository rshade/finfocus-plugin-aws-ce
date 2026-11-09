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
	LatestRegionalSpotPrice(ctx context.Context, region, instanceType, product string) (client.RegionalSpotPrice, error)
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

// regional reports whether the target names no zone, so the quote is the region-level median.
func (q spotQuery) regional() bool {
	return q.availabilityZone == ""
}

const (
	spotRegionalRule     = "region-median"
	spotBasisRegion      = "region"
	spotRegionalIDKey    = "region"
	spotRegionalNotFound = "no spot price for the instance type in the region"
)

var (
	instanceTypePattern = regexp.MustCompile(`^[a-z0-9-]+\.[a-z0-9.]+$`)
	awsNamePattern      = regexp.MustCompile(`^[a-z0-9-]+$`)
)

func spotComparisonFromEnv() bool {
	return strings.EqualFold(strings.TrimSpace(os.Getenv(SpotComparisonEnv)), "true")
}

func wantsSpotComparison(resource *pbc.ResourceDescriptor) bool {
	return spotIneligibleReason(resource) == ""
}

// spotIneligibleReason names why a resource gets no Spot quote, or returns "" when it does.
// A zone or a region is enough to attempt the quote.
func spotIneligibleReason(resource *pbc.ResourceDescriptor) string {
	if !isEC2Instance(resource.GetResourceType()) {
		return "not an EC2 instance"
	}
	if instanceTypeOf(resource) == "" {
		return "no instance type"
	}
	if strings.TrimSpace(resource.GetRegion()) == "" && availabilityZoneOf(resource) == "" {
		return "no region or availability zone"
	}
	return ""
}

func logSpotSkip(logger zerolog.Logger, resource *pbc.ResourceDescriptor) {
	if !isEC2Instance(resource.GetResourceType()) {
		return
	}
	reason := spotIneligibleReason(resource)
	if reason == "" {
		return
	}
	logger.Debug().Str("resource_id", resource.GetId()).Str("reason", reason).Msg("spot comparison skipped")
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

type spotQuote struct {
	hourly      float64
	zones       int
	onDemand    float64
	hasOnDemand bool
	risk        spotInterruption
}

func (c *Calculator) quoteSpot(ctx context.Context, logger zerolog.Logger, req *pbc.GetProjectedCostRequest) (*pbc.GetProjectedCostResponse, error) {
	query, err := parseSpotQuery(req.GetResource())
	if err != nil {
		return nil, err
	}
	quote, err := c.spotQuoteFor(ctx, logger, query)
	if err != nil {
		return nil, err
	}
	hours := pluginsdk.DefaultMonthlyHours(req.GetUsageProfile())
	resp := buildSpotResponse(query, quote, hours, time.Now().UTC())
	if err := pluginsdk.ValidateGetProjectedCostResponse(resp); err != nil {
		return nil, status.Error(codes.Internal, "spot quote response is invalid")
	}
	if resp.GetSpotInterruptionRiskScore() == 0 {
		logger.Warn().Str("instance_type", query.instanceType).Msg("spot interruption score unavailable")
	}
	return resp, nil
}

func (c *Calculator) spotQuoteFor(ctx context.Context, logger zerolog.Logger, query spotQuery) (spotQuote, error) {
	spots := c.spotQuote
	demands := c.onDemandQuote
	risks := c.spotRisk
	perRequest := false
	if spots == nil || demands == nil {
		ce, per, callErr := c.clientForCall(ctx, logger)
		if callErr != nil {
			if _, ok := status.FromError(callErr); ok {
				return spotQuote{}, callErr
			}
			return spotQuote{}, status.Errorf(codes.Internal, "client initialization failed: %v", callErr)
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
	logger.Debug().Str("instance_type", query.instanceType).Str("region", query.region).Str("availability_zone", query.availabilityZone).Str("product", query.product).Bool("region_level", query.regional()).Msg("spot quote")

	var hourly float64
	var zones int
	if query.regional() {
		regional, err := spots.LatestRegionalSpotPrice(ctx, query.region, query.instanceType, query.product)
		if err != nil {
			return spotQuote{}, c.spotCallError(logger, perRequest, true, err)
		}
		hourly, zones = regional.Hourly, regional.Zones
	} else {
		price, err := spots.LatestSpotPrice(ctx, query.region, query.instanceType, query.product, query.availabilityZone)
		if err != nil {
			return spotQuote{}, c.spotCallError(logger, perRequest, false, err)
		}
		hourly = price
	}
	onDemand, hasOnDemand, err := demands.OnDemandHourly(ctx, query.region, query.instanceType, query.priceListOS)
	if err != nil {
		return spotQuote{}, c.spotCallError(logger, perRequest, query.regional(), err)
	}
	risk, err := risks.Interruption(ctx, query.region, query.advisorOS, query.instanceType)
	if err != nil {
		logger.Warn().Msg("spot interruption feed unavailable")
		risk = spotInterruption{}
	}
	return spotQuote{hourly: hourly, zones: zones, onDemand: onDemand, hasOnDemand: hasOnDemand, risk: risk}, nil
}

func (c *Calculator) spotCallError(logger zerolog.Logger, perRequest, regional bool, err error) error {
	if errors.Is(err, client.ErrNoSpotPrice) {
		if regional {
			return status.Error(codes.NotFound, spotRegionalNotFound)
		}
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
	if zone != "" && !awsNamePattern.MatchString(zone) {
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

func buildSpotResponse(query spotQuery, quote spotQuote, hours float64, now time.Time) *pbc.GetProjectedCostResponse {
	hourly, onDemand, hasOnDemand, risk := quote.hourly, quote.onDemand, quote.hasOnDemand, quote.risk
	monthly := hourly * hours
	detail := fmt.Sprintf("Spot %s %s in %s at %s USD per hour for %s hours", query.product, query.instanceType, spotLocation(query, quote), strconv.FormatFloat(hourly, 'f', -1, 64), strconv.FormatFloat(hours, 'f', -1, 64))
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
		"spot_product_description": query.product,
	}
	spotBasisMetadata(resp.Metadata, query, quote)
	if risk.Label != "" {
		resp.Metadata["spot_interruption_label"] = risk.Label
	}
	return resp
}

// spotLocation names where the Spot price applies: the zone, or the region with the number
// of zones in the median.
func spotLocation(query spotQuery, quote spotQuote) string {
	if !query.regional() {
		return query.availabilityZone
	}
	return fmt.Sprintf("%s (median of %d zones)", query.region, quote.zones)
}

// spotBasisMetadata records whether the Spot price came from an explicit zone or the regional
// median. Explicit-zone responses keep their original keys, so a missing spot_az_basis means zone.
func spotBasisMetadata(meta map[string]string, query spotQuery, quote spotQuote) {
	if !query.regional() {
		meta["spot_availability_zone"] = query.availabilityZone
		return
	}
	meta["spot_availability_zone"] = spotRegionalRule
	meta["spot_az_basis"] = spotBasisRegion
	meta["spot_zone_count"] = strconv.Itoa(quote.zones)
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
