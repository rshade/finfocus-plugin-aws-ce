package pricing

import (
	"context"
	"strings"
	"testing"

	"github.com/rshade/finfocus-spec/sdk/go/pluginsdk"
	pbc "github.com/rshade/finfocus-spec/sdk/go/proto/finfocus/v1"
)

const ec2InstanceARN = "arn:aws:ec2:us-east-1:123456789012:instance/i-1234567890abcdef0"

func TestSupports(t *testing.T) {
	t.Parallel()

	invalidResource := pbc.ErrorCode_ERROR_CODE_INVALID_RESOURCE.String()
	if !strings.Contains(invalidResource, "ERROR_CODE_INVALID_RESOURCE") {
		t.Fatalf("ErrorCode string %q does not contain ERROR_CODE_INVALID_RESOURCE", invalidResource)
	}

	calc := NewCalculator()
	tests := []struct {
		name        string
		req         *pbc.SupportsRequest
		supported   bool
		reasonHas   string
		plainReason bool
		scope       bool
	}{
		{
			name: "valid EC2 ARN",
			req: &pbc.SupportsRequest{Resource: &pbc.ResourceDescriptor{
				Provider: "aws",
				Arn:      ec2InstanceARN,
				Region:   "us-east-1",
			}},
			supported: true,
		},
		{
			name: "valid Id without ARN",
			req: &pbc.SupportsRequest{Resource: &pbc.ResourceDescriptor{
				Provider: "aws",
				Id:       "i-1234567890abcdef0",
				Region:   "not-a-region",
			}},
			supported: true,
		},
		{
			name: "parseable non-EC2 ARN",
			req: &pbc.SupportsRequest{Resource: &pbc.ResourceDescriptor{
				Provider: "aws",
				Arn:      "arn:aws:s3:::my_bucket",
			}},
			supported: true,
			reasonHas: "unverified",
		},
		{
			name: "malformed ARN with Id",
			req: &pbc.SupportsRequest{Resource: &pbc.ResourceDescriptor{
				Provider: "aws",
				Arn:      "not-an-arn",
				Id:       "bucket-logs",
			}},
			supported: false, reasonHas: invalidResource,
		},
		{
			name: "missing identifiers",
			req: &pbc.SupportsRequest{Resource: &pbc.ResourceDescriptor{
				Provider:     "aws",
				ResourceType: "ec2",
				Region:       "eu-west-1",
			}},
			supported: true,
			reasonHas: "unverified",
		},
		{
			name: "malformed ARN with no Id",
			req: &pbc.SupportsRequest{Resource: &pbc.ResourceDescriptor{
				Provider: "aws",
				Arn:      "not-an-arn",
			}},
			supported: false,
			reasonHas: invalidResource,
		},
		{
			name: "provider gcp",
			req: &pbc.SupportsRequest{Resource: &pbc.ResourceDescriptor{
				Provider: "gcp",
				Arn:      ec2InstanceARN,
				Id:       "i-1234567890abcdef0",
			}},
			supported:   false,
			plainReason: true,
		},
		{
			name: "provider AWS",
			req: &pbc.SupportsRequest{Resource: &pbc.ResourceDescriptor{
				Provider: "AWS",
				Id:       "i-1234567890abcdef0",
			}},
			supported:   false,
			plainReason: true,
		},
		{
			name: "Pulumi EC2 instance without identifiers",
			req: &pbc.SupportsRequest{Resource: &pbc.ResourceDescriptor{
				Provider:     "aws",
				ResourceType: "aws:ec2/instance:Instance",
				Region:       "us-east-1",
				Sku:          "t3.micro",
			}},
			supported: true,
			reasonHas: "14 days",
		},
		{
			name: "Pulumi account scope without identifiers",
			req: &pbc.SupportsRequest{Resource: &pbc.ResourceDescriptor{
				Provider:     "aws",
				ResourceType: "aws:account",
			}},
			supported: true,
			scope:     true,
		},
		{
			name: "Pulumi service scope without identifiers",
			req: &pbc.SupportsRequest{Resource: &pbc.ResourceDescriptor{
				Provider:     "aws",
				ResourceType: "aws:service",
			}},
			supported: true,
			scope:     true,
		},
		{
			name: "Pulumi S3 bucket without identifiers",
			req: &pbc.SupportsRequest{Resource: &pbc.ResourceDescriptor{
				Provider:     "aws",
				ResourceType: "aws:s3/bucket:Bucket",
			}},
			supported: true,
			reasonHas: "unverified",
		},
		{
			name:      "nil resource",
			req:       &pbc.SupportsRequest{},
			supported: false,
			reasonHas: invalidResource,
		},
		{
			name:      "nil request",
			req:       nil,
			supported: false,
			reasonHas: invalidResource,
		},
	}

	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			t.Parallel()
			resp, err := calc.Supports(context.Background(), tc.req)
			if err != nil {
				t.Fatalf("Supports returned error %v; unsupported resources must return Supported false", err)
			}
			if resp == nil {
				t.Fatal("Supports returned nil response")
			}
			if resp.GetSupported() != tc.supported {
				t.Fatalf("supported = %v, want %v (reason %q)", resp.GetSupported(), tc.supported, resp.GetReason())
			}
			if len(resp.GetCapabilitiesEnum()) != 4 || resp.GetCapabilitiesEnum()[0] != pbc.PluginCapability_PLUGIN_CAPABILITY_ACTUAL_COSTS || resp.GetCapabilitiesEnum()[2] != pbc.PluginCapability_PLUGIN_CAPABILITY_BUDGETS || resp.GetCapabilitiesEnum()[3] != pbc.PluginCapability_PLUGIN_CAPABILITY_RECOMMENDATIONS {
				t.Fatalf("typed capabilities = %v, want actual, projected, budgets, and recommendations", resp.GetCapabilitiesEnum())
			}
			if len(resp.GetCapabilities()) != 0 || len(resp.GetSupportedMetrics()) != 0 {
				t.Fatalf("plugin filled legacy capabilities or metrics: %#v", resp)
			}
			switch {
			case tc.supported && tc.scope:
				if resp.GetReason() != "" {
					t.Fatalf("scope reason = %q, want empty", resp.GetReason())
				}
			case tc.supported:
				if !strings.Contains(resp.GetReason(), "14 days") || !strings.Contains(resp.GetReason(), "opt-in") {
					t.Fatalf("supported reason = %q, want resource limitations", resp.GetReason())
				}
				if tc.reasonHas != "" && !strings.Contains(resp.GetReason(), tc.reasonHas) {
					t.Fatalf("reason %q does not contain %q", resp.GetReason(), tc.reasonHas)
				}
			case tc.plainReason:
				reason := resp.GetReason()
				if reason == "" {
					t.Fatal("unsupported provider reason is empty")
				}
				if strings.Contains(reason, "ERROR_CODE_") {
					t.Fatalf("non-aws reason %q names an error code; ERROR_CODE_UNSUPPORTED does not exist", reason)
				}
			default:
				if !strings.Contains(resp.GetReason(), tc.reasonHas) {
					t.Fatalf("reason %q does not contain %q", resp.GetReason(), tc.reasonHas)
				}
			}
		})
	}
}

func TestSupports_GRPC(t *testing.T) {
	t.Parallel()

	srv := pluginsdk.NewTestServer(t, NewCalculator())
	t.Cleanup(srv.Close)

	ctx := context.Background()
	resp, err := srv.Client().Supports(ctx, &pbc.SupportsRequest{
		Resource: &pbc.ResourceDescriptor{
			Provider: "aws",
			Arn:      ec2InstanceARN,
		},
	})
	if err != nil {
		t.Fatalf("Supports rpc: %v", err)
	}
	if !resp.GetSupported() {
		t.Fatalf("supported = false, reason %q", resp.GetReason())
	}
	if !hasCapability(resp.GetCapabilitiesEnum(), pbc.PluginCapability_PLUGIN_CAPABILITY_ACTUAL_COSTS) {
		t.Fatalf("server did not fill capabilities from an empty CapabilitiesEnum: %v", resp.GetCapabilitiesEnum())
	}

	resp, err = srv.Client().Supports(ctx, &pbc.SupportsRequest{
		Resource: &pbc.ResourceDescriptor{
			Provider: "gcp",
			Id:       "i-1234567890abcdef0",
		},
	})
	if err != nil {
		t.Fatalf("gcp Supports rpc returned %v; a plugin error becomes codes.Internal", err)
	}
	if resp.GetSupported() {
		t.Fatal("provider gcp supported = true")
	}
	if strings.Contains(resp.GetReason(), "ERROR_CODE_") {
		t.Fatalf("gcp reason %q names an error code", resp.GetReason())
	}
}

func hasCapability(caps []pbc.PluginCapability, want pbc.PluginCapability) bool {
	for _, cap := range caps {
		if cap == want {
			return true
		}
	}
	return false
}

func TestSupportsEC2InstanceStaysSupportedForActuals(t *testing.T) {
	t.Parallel()

	resp, err := NewCalculator().Supports(context.Background(), &pbc.SupportsRequest{Resource: &pbc.ResourceDescriptor{
		Provider:     "aws",
		ResourceType: "aws:ec2/instance:Instance",
		Region:       "us-west-2",
	}})
	if err != nil {
		t.Fatal(err)
	}
	if !resp.GetSupported() {
		t.Fatalf("EC2 instance declined for every capability (%q); that would also remove actual costs", resp.GetReason())
	}
}

// TestSupportsProjectedCostCapability checks that a PROJECTED_COSTS request is declined, with a reason, for
// anything GetProjectedCost rejects before calling AWS, while ACTUAL_COSTS and unspecified answers are unchanged.
func TestSupportsProjectedCostCapability(t *testing.T) {
	t.Parallel()

	projected := pbc.PluginCapability_PLUGIN_CAPABILITY_PROJECTED_COSTS
	actual := pbc.PluginCapability_PLUGIN_CAPABILITY_ACTUAL_COSTS
	unspecified := pbc.PluginCapability_PLUGIN_CAPABILITY_UNSPECIFIED
	s3 := &pbc.ResourceDescriptor{Provider: "aws", ResourceType: "aws:s3/bucket:Bucket"}
	ec2TypeOnly := &pbc.ResourceDescriptor{Provider: "aws", ResourceType: pulumiEC2InstanceType, Region: "us-west-2"}
	ec2Priced := &pbc.ResourceDescriptor{
		Provider: "aws", ResourceType: pulumiEC2InstanceType, Sku: "t3.micro", Region: "us-west-2",
	}
	account := &pbc.ResourceDescriptor{Provider: "aws", ResourceType: "aws:account", Sku: "account", Region: "us-east-1"}
	service := &pbc.ResourceDescriptor{
		Provider: "aws", ResourceType: "aws:service", Id: "Amazon Simple Storage Service", Sku: "service", Region: "us-east-1",
	}

	tests := []struct {
		name       string
		resource   *pbc.ResourceDescriptor
		capability pbc.PluginCapability
		spot       bool
		supported  bool
		reasonHas  string
	}{
		{name: "S3 projected is declined", resource: s3, capability: projected, reasonHas: "cannot forecast"},
		{name: "S3 actual is unchanged", resource: s3, capability: actual, supported: true, reasonHas: "unverified"},
		{name: "S3 unspecified is unchanged", resource: s3, capability: unspecified, supported: true, reasonHas: "unverified"},
		{name: "unknown capability reads as unspecified", resource: s3, capability: 999, supported: true},
		{
			name: "EC2 projected without spot comparison is declined", resource: ec2Priced, capability: projected,
			reasonHas: SpotComparisonEnv,
		},
		{name: "EC2 actual without spot comparison is unchanged", resource: ec2TypeOnly, capability: actual, supported: true},
		{name: "eligible EC2 projected with spot comparison", resource: ec2Priced, capability: projected, spot: true, supported: true},
		{
			name: "EC2 without instance type is declined with spot comparison", resource: ec2TypeOnly,
			capability: projected, spot: true, reasonHas: "no instance type",
		},
		{
			name: "EC2 with a malformed instance type is declined with spot comparison",
			resource: &pbc.ResourceDescriptor{
				Provider: "aws", ResourceType: pulumiEC2InstanceType, Sku: "t3_micro", Region: "us-west-2",
			},
			capability: projected, spot: true, reasonHas: "instance type is invalid",
		},
		{
			name: "EC2 with an unsupported operating system is declined with spot comparison",
			resource: &pbc.ResourceDescriptor{
				Provider: "aws", ResourceType: pulumiEC2InstanceType, Sku: "t3.micro", Region: "us-west-2",
				Tags: map[string]string{"os": "freebsd"},
			},
			capability: projected, spot: true, reasonHas: "operating system is unsupported",
		},
		{name: "account projected", resource: account, capability: projected, supported: true},
		{name: "service projected", resource: service, capability: projected, supported: true},
		{
			name:       "service without id is declined",
			resource:   &pbc.ResourceDescriptor{Provider: "aws", ResourceType: "aws:service", Sku: "service", Region: "us-east-1"},
			capability: projected, reasonHas: "cannot forecast",
		},
		{
			name:       "account without sku is declined",
			resource:   &pbc.ResourceDescriptor{Provider: "aws", ResourceType: "aws:account", Region: "us-east-1"},
			capability: projected, reasonHas: "sku",
		},
		{
			name:       "other provider keeps its reason",
			resource:   &pbc.ResourceDescriptor{Provider: "gcp", ResourceType: "aws:account"},
			capability: projected, reasonHas: "provider",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()
			calc := NewCalculator()
			calc.spotComparison = tt.spot
			resp, err := calc.Supports(context.Background(), &pbc.SupportsRequest{Resource: tt.resource, Capability: tt.capability})
			if err != nil {
				t.Fatal(err)
			}
			if resp.GetSupported() != tt.supported {
				t.Fatalf("supported = %v, want %v (reason %q)", resp.GetSupported(), tt.supported, resp.GetReason())
			}
			if !tt.supported && resp.GetReason() == "" {
				t.Fatal("a decline must carry a reason")
			}
			if !strings.Contains(resp.GetReason(), tt.reasonHas) {
				t.Fatalf("reason %q does not mention %q", resp.GetReason(), tt.reasonHas)
			}
		})
	}
}
