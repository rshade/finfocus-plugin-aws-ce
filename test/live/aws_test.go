//go:build awslive

package live

import (
	"context"
	"errors"
	"fmt"
	"os"
	"testing"
	"time"

	"github.com/aws/aws-sdk-go-v2/aws"
	"github.com/aws/aws-sdk-go-v2/config"
	"github.com/aws/aws-sdk-go-v2/service/costexplorer"
	"github.com/aws/aws-sdk-go-v2/service/costexplorer/types"
	"github.com/aws/aws-sdk-go-v2/service/sts"
	"github.com/aws/smithy-go"
	"github.com/rshade/finfocus-plugin-aws-ce/internal/testutil"
	pbc "github.com/rshade/finfocus-spec/sdk/go/proto/finfocus/v1"
	"google.golang.org/grpc/codes"
	"google.golang.org/grpc/status"
	"google.golang.org/protobuf/types/known/timestamppb"
)

func awsFailureCode(err error) string {
	var api smithy.APIError
	if errors.As(err, &api) {
		return api.ErrorCode()
	}
	return fmt.Sprintf("%T", err)
}

func TestAWSActualCost(t *testing.T) {
	if os.Getenv("FINFOCUS_AWS_LIVE") != "true" {
		t.Skip("Set FINFOCUS_AWS_LIVE=true with the awslive build tag for explicitly authorized billable checks")
	}
	for _, key := range []string{"AWS_ENDPOINT_URL", "AWS_ENDPOINT_URL_COST_EXPLORER", "AWS_ENDPOINT_URL_STS"} {
		t.Setenv(key, "")
	}
	t.Setenv("AWS_IGNORE_CONFIGURED_ENDPOINT_URLS", "true")
	ctx, cancel := context.WithTimeout(context.Background(), 40*time.Second)
	defer cancel()
	cfg, err := config.LoadDefaultConfig(ctx, config.WithRegion("us-east-1"), config.WithRetryMaxAttempts(1))
	if err != nil {
		t.Fatal("AWS configuration failed:", awsFailureCode(err))
	}
	identity, err := sts.NewFromConfig(cfg).GetCallerIdentity(ctx, &sts.GetCallerIdentityInput{})
	if err != nil || aws.ToString(identity.Account) == "" {
		t.Fatal("AWS identity failed:", awsFailureCode(err))
	}
	t.Log("AWS identity verified; credentials and principal are not printed")
	end := time.Now().UTC().Truncate(24*time.Hour).AddDate(0, 0, -2)
	start := end.AddDate(0, 0, -7)
	baseline, baselineErr := costexplorer.NewFromConfig(cfg).GetCostAndUsage(ctx, &costexplorer.GetCostAndUsageInput{TimePeriod: &types.DateInterval{Start: aws.String(start.Format(time.DateOnly)), End: aws.String(end.Format(time.DateOnly))}, Granularity: types.GranularityDaily, Metrics: []string{"UnblendedCost", "UsageQuantity"}, GroupBy: []types.GroupDefinition{{Type: types.GroupDefinitionTypeDimension, Key: aws.String("SERVICE")}}})
	if baselineErr != nil && awsFailureCode(baselineErr) != "AccessDeniedException" {
		t.Fatal("AWS baseline failed:", awsFailureCode(baselineErr))
	}
	if baselineErr == nil && baseline == nil {
		t.Fatal("AWS returned no baseline response")
	}
	if baseline != nil && aws.ToString(baseline.NextPageToken) != "" {
		t.Fatal("baseline exceeds one-page check budget; no further CE call made")
	}
	proc := testutil.StartPlugin(t, map[string]string{"AWS_REGION": "us-east-1", "AWS_DEFAULT_REGION": "us-east-1", "AWS_MAX_ATTEMPTS": "1", "AWS_IGNORE_CONFIGURED_ENDPOINT_URLS": "true", "FINFOCUS_AWS_CE_MAX_REQUESTS_PER_MINUTE": "1", "FINFOCUS_PLUGIN_PORT": "0", "FINFOCUS_LOG_FILE": "", "FINFOCUS_LOG_LEVEL": "error"}, "--port", "0")
	info, err := proc.Client.GetPluginInfo(ctx, &pbc.GetPluginInfoRequest{})
	if err != nil || info.GetName() != "aws-ce" || info.GetSpecVersion() != "v0.7.5" {
		t.Fatal("GetPluginInfo failed")
	}
	for _, provider := range []string{"aws", "azure"} {
		result, callErr := proc.Client.Supports(ctx, &pbc.SupportsRequest{Resource: &pbc.ResourceDescriptor{Provider: provider, ResourceType: "s3", Id: "live-service-totals"}})
		if callErr != nil || result.GetSupported() != (provider == "aws") {
			t.Fatal("Supports provider result differs")
		}
	}
	_, err = proc.Client.GetActualCost(ctx, &pbc.GetActualCostRequest{})
	if status.Code(err) != codes.InvalidArgument {
		t.Fatal("invalid request reached unexpected status:", status.Code(err))
	}
	request := &pbc.GetActualCostRequest{ResourceId: fmt.Sprintf("live-service-totals-%d", time.Now().UnixNano()), Start: timestamppb.New(start), End: timestamppb.New(end), BillingAccountId: aws.ToString(identity.Account)}
	request.Resource = &pbc.ResourceDescriptor{Provider: "aws", ResourceType: "s3", Id: request.ResourceId}
	result, err := proc.Client.GetActualCost(ctx, request)
	t.Logf("period=%s..%s; Supports AWS=true Azure=false; GetPluginInfo aws-ce v0.7.5; invalid=InvalidArgument; actual=%s", start.Format(time.DateOnly), end.Format(time.DateOnly), status.Code(err))
	if baselineErr != nil {
		if status.Code(err) != codes.PermissionDenied || result != nil {
			t.Fatal("AWS AccessDenied must map to PermissionDenied:", status.Code(err))
		}
		if stopErr := proc.Stop(); stopErr != nil {
			t.Fatal("plugin shutdown failed")
		}
		t.Skip("BLOCKED-ON-INPUT: AWS role lacks ce:GetCostAndUsage; real denial mapping verified, cost amounts not verified")
	}
	if err != nil {
		t.Fatal("plugin actual cost failed:", status.Code(err))
	}
	if err := compareLiveCost(baseline.ResultsByTime, result, aws.ToString(identity.Account)); err != nil {
		t.Fatal(err)
	}
	t.Logf("Live AWS cost comparison passed: %d rows; dates, amounts, currency, caller account and Estimated match", len(result.GetResults()))
	if err := proc.Stop(); err != nil {
		t.Fatal("plugin shutdown failed")
	}
}
