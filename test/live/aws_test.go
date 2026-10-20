//go:build awslive

package live

import (
	"context"
	"errors"
	"fmt"
	"math"
	"os"
	"testing"
	"time"

	"github.com/aws/aws-sdk-go-v2/aws"
	"github.com/aws/aws-sdk-go-v2/config"
	"github.com/aws/aws-sdk-go-v2/service/costexplorer"
	"github.com/aws/aws-sdk-go-v2/service/sts"
	"github.com/aws/smithy-go"
	"github.com/rshade/finfocus-plugin-aws-ce/internal/testutil"
	"github.com/rshade/finfocus-spec/sdk/go/pluginsdk"
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
		t.Skip("Set FINFOCUS_AWS_LIVE=true with the awslive build tag for billable checks")
	}
	if os.Getenv("FINFOCUS_CORE_BINARY") == "" {
		t.Fatal("set FINFOCUS_CORE_BINARY; make core-install builds the pinned core")
	}
	for _, key := range []string{"AWS_ENDPOINT_URL", "AWS_ENDPOINT_URL_COST_EXPLORER", "AWS_ENDPOINT_URL_STS"} {
		t.Setenv(key, "")
	}
	t.Setenv("AWS_IGNORE_CONFIGURED_ENDPOINT_URLS", "true")
	t.Setenv("AWS_EC2_METADATA_DISABLED", "true")
	ctx, cancel := context.WithTimeout(context.Background(), 110*time.Second)
	defer cancel()
	cfg, err := config.LoadDefaultConfig(ctx, config.WithRegion("us-east-1"), config.WithRetryMaxAttempts(1))
	if err != nil {
		t.Fatal("AWS configuration failed:", awsFailureCode(err))
	}
	if _, err := cfg.Credentials.Retrieve(ctx); err != nil {
		if os.Getenv("FINFOCUS_AWS_LIVE_REQUIRED") == "true" {
			t.Fatal("required AWS credentials unavailable:", awsFailureCode(err))
		}
		t.Skip("AWS credentials unavailable; no cost verification performed")
	}
	identity, err := sts.NewFromConfig(cfg).GetCallerIdentity(ctx, &sts.GetCallerIdentityInput{})
	if err != nil || identity == nil || aws.ToString(identity.Account) == "" {
		t.Fatal("AWS identity failed:", awsFailureCode(err))
	}
	t.Log("AWS identity verified; credentials and principal are not printed")
	service := os.Getenv("FINFOCUS_AWS_LIVE_SERVICE")
	if service == "" {
		service = "Amazon Simple Storage Service"
	}
	end := time.Now().UTC().Truncate(24*time.Hour).AddDate(0, 0, -2)
	start := end.AddDate(0, 0, -7)
	baseline, err := fetchLiveBaseline(ctx, costexplorer.NewFromConfig(cfg), service, start, end)
	if err != nil {
		// Report only classification, never an AWS response that may include identity.
		t.Fatal("independent AWS baseline unavailable or incomparable:", awsFailureCode(err))
	}
	total, currency, err := baselineTotal(baseline, service)
	if err != nil {
		t.Fatal(err)
	}
	env := map[string]string{
		"AWS_REGION": "us-east-1", "AWS_DEFAULT_REGION": "us-east-1", "AWS_MAX_ATTEMPTS": "1",
		"AWS_IGNORE_CONFIGURED_ENDPOINT_URLS": "true", "FINFOCUS_AWS_CE_MAX_REQUESTS_PER_MINUTE": "3",
		"FINFOCUS_PLUGIN_PORT": "0", "FINFOCUS_LOG_FILE": "", "FINFOCUS_LOG_LEVEL": "error",
	}
	proc := testutil.StartPlugin(t, env, "--port", "0")
	info, err := proc.Client.GetPluginInfo(ctx, &pbc.GetPluginInfoRequest{})
	if err != nil || info.GetName() != "aws-ce" || info.GetSpecVersion() != pluginsdk.SpecVersion {
		t.Fatal("GetPluginInfo failed or returned an unexpected spec version")
	}
	for _, provider := range []string{"aws", "azure"} {
		result, callErr := proc.Client.Supports(ctx, &pbc.SupportsRequest{Resource: &pbc.ResourceDescriptor{Provider: provider, ResourceType: "aws:service", Id: service}})
		if callErr != nil || result.GetSupported() != (provider == "aws") {
			t.Fatal("Supports provider result differs")
		}
	}
	_, err = proc.Client.GetActualCost(ctx, &pbc.GetActualCostRequest{})
	if status.Code(err) != codes.InvalidArgument {
		t.Fatal("invalid request reached unexpected status:", status.Code(err))
	}
	request := &pbc.GetActualCostRequest{
		ResourceId: service, Start: timestamppb.New(start), End: timestamppb.New(end),
		BillingAccountId: aws.ToString(identity.Account),
		Resource:         &pbc.ResourceDescriptor{Provider: "aws", ResourceType: "aws:service", Id: service},
	}
	queryCtx, queryCancel := context.WithTimeout(ctx, 45*time.Second)
	defer queryCancel()
	result, err := proc.Client.GetActualCost(queryCtx, request)
	if err != nil {
		t.Fatal("plugin actual cost failed:", status.Code(err))
	}
	if err := compareLiveCost(baseline, result, aws.ToString(identity.Account), "UnblendedCost"); err != nil {
		t.Fatal(err)
	}
	if err := proc.Stop(); err != nil {
		t.Fatal("plugin shutdown failed")
	}
	rows, err := testutil.RunCoreActualCost(t, testutil.CoreActualCostConfig{
		Start: start, End: end, ResourceType: "aws:service", ResourceID: service, Env: env,
	})
	if err != nil {
		t.Fatal(err)
	}
	if len(rows) != 1 || rows[0].Currency != currency || math.Abs(rows[0].TotalCost-total) > 1e-9*math.Max(1, math.Abs(total)) {
		t.Fatal("core total or currency differs from independent AWS baseline")
	}
	t.Logf("Live core and plugin comparison passed for %s..%s; FOCUS, amount, currency and estimate verified", start.Format(time.DateOnly), end.Format(time.DateOnly))
}
