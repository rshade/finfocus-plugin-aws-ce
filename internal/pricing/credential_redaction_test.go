package pricing

import (
	"bytes"
	"context"
	"errors"
	"strings"
	"testing"

	"github.com/aws/aws-sdk-go-v2/service/costexplorer"
	"github.com/rs/zerolog"
	"github.com/rshade/finfocus-plugin-aws-ce/internal/client"
	"google.golang.org/grpc/codes"
	"google.golang.org/grpc/status"
)

func TestCredentialShapeValidation(t *testing.T) {
	for _, entries := range []map[string]string{
		{"access_key_id": "   ", "secret_access_key": "secret-shape"},
		{"access_key_id": "key-shape", "secret_access_key": "  "},
		{"role_arn": "not-an-arn"},
		{"role_arn": "arn:aws:iam::123456789012:user/not-a-role"},
		{"role_arn": "arn:aws:iam::123:role/short-account"},
		{"access_key_id": "key-shape", "secret_access_key": "secret-shape", "unexpected_key": "unknown-value"},
	} {
		calc := NewCalculator()
		calc.cache = nil
		calc.openClient = func(context.Context, client.Config) (*client.Client, error) {
			t.Error("invalid credentials reached client initialization")
			return client.NewClientWithAPI(pricedCostAPI(), "us-east-1"), nil
		}
		_, err := calc.GetActualCost(credentialContext(t, entries), serviceCostRequest())
		if status.Code(err) != codes.InvalidArgument {
			t.Fatalf("invalid shape accepted: %v", err)
		}
	}
}

func TestCredentialValuesNeverLogged(t *testing.T) {
	entries := map[string]string{"access_key_id": "AKIAREDACTIONONLY", "secret_access_key": "redaction-secret-value", "session_token": "redaction-session-value", "role_arn": "arn:aws:iam::123456789012:role/redaction-value"}
	diagnostic := entries["access_key_id"] + " " + entries["secret_access_key"] + " " + entries["session_token"] + " " + entries["role_arn"]
	for _, scenario := range []string{"success", "initialization failure", "request failure"} {
		t.Run(scenario, func(t *testing.T) {
			var logs bytes.Buffer
			calc := NewCalculator()
			calc.cache = nil
			calc.logger = zerolog.New(&logs)
			calc.openClient = func(context.Context, client.Config) (*client.Client, error) {
				if scenario == "initialization failure" {
					return nil, errors.New(diagnostic)
				}
				api := pricedCostAPI()
				if scenario == "request failure" {
					api.GetCostAndUsageFunc = func(context.Context, *costexplorer.GetCostAndUsageInput, ...func(*costexplorer.Options)) (*costexplorer.GetCostAndUsageOutput, error) {
						return nil, errors.New(diagnostic)
					}
				}
				return client.NewClientWithAPI(api, "us-east-1"), nil
			}
			_, err := calc.GetActualCost(credentialContext(t, entries), serviceCostRequest())
			if scenario == "success" && err != nil {
				t.Fatal(err)
			}
			if scenario != "success" && err == nil {
				t.Fatal("failure hidden")
			}
			for _, value := range entries {
				if strings.Contains(logs.String(), value) || (err != nil && strings.Contains(err.Error(), value)) {
					t.Fatal("credential value leaked in log or returned status")
				}
			}
		})
	}
}
