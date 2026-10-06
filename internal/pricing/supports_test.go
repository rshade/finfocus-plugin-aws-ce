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
		},
		{
			name: "malformed ARN with Id",
			req: &pbc.SupportsRequest{Resource: &pbc.ResourceDescriptor{
				Provider: "aws",
				Arn:      "not-an-arn",
				Id:       "bucket-logs",
			}},
			supported: true,
		},
		{
			name: "missing identifiers",
			req: &pbc.SupportsRequest{Resource: &pbc.ResourceDescriptor{
				Provider:     "aws",
				ResourceType: "ec2",
				Region:       "eu-west-1",
			}},
			supported: false,
			reasonHas: invalidResource,
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
			if len(resp.GetCapabilitiesEnum()) != 1 || resp.GetCapabilitiesEnum()[0] != pbc.PluginCapability_PLUGIN_CAPABILITY_ACTUAL_COSTS {
				t.Fatalf("typed capabilities = %v, want actual costs", resp.GetCapabilitiesEnum())
			}
			if len(resp.GetCapabilities()) != 0 || len(resp.GetSupportedMetrics()) != 0 {
				t.Fatalf("plugin filled legacy capabilities or metrics: %#v", resp)
			}
			switch {
			case tc.supported:
				if resp.GetReason() != "" {
					t.Fatalf("supported reason = %q, want empty", resp.GetReason())
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
