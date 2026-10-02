package pricing

import (
	"bytes"
	"context"
	"strings"
	"testing"

	"github.com/aws/aws-sdk-go-v2/service/costexplorer"
	"github.com/aws/smithy-go"
	"github.com/rs/zerolog"
	"github.com/rshade/finfocus-plugin-aws-ce/internal/client"
	"github.com/rshade/finfocus-spec/sdk/go/pluginsdk"
	pbc "github.com/rshade/finfocus-spec/sdk/go/proto/finfocus/v1"
	"google.golang.org/grpc/codes"
	"google.golang.org/grpc/status"
)

// leakyAPIError's Error and ErrorMessage echo a role ARN.
// The mapping must use ErrorCode only, so the body stays out of the status and the log.
type leakyAPIError struct {
	code string
	body string
}

func (e *leakyAPIError) Error() string { return e.body }

func (e *leakyAPIError) ErrorCode() string { return e.code }

func (e *leakyAPIError) ErrorMessage() string { return e.body }

func (e *leakyAPIError) ErrorFault() smithy.ErrorFault { return smithy.FaultClient }

func TestGetActualCost_AWSErrorCodes(t *testing.T) {
	t.Parallel()

	const roleARN = "arn:aws:iam::123456789012:role/finfocus-denied"
	const secret = "ce69-denied-secret"
	body := "not authorized to perform: sts:AssumeRole on resource: " + roleARN + " secret " + secret

	tests := []struct {
		name      string
		code      string
		grpcCode  codes.Code
		detail    pbc.ErrorCode
		hasDetail bool
	}{
		{
			name:      "LimitExceededException",
			code:      "LimitExceededException",
			grpcCode:  codes.ResourceExhausted,
			detail:    pbc.ErrorCode_ERROR_CODE_RATE_LIMITED,
			hasDetail: true,
		},
		{
			name:      "AccessDenied",
			code:      "AccessDenied",
			grpcCode:  codes.PermissionDenied,
			detail:    pbc.ErrorCode_ERROR_CODE_PERMISSION_DENIED,
			hasDetail: true,
		},
		{
			name:      "ExpiredToken",
			code:      "ExpiredToken",
			grpcCode:  codes.Unauthenticated,
			detail:    pbc.ErrorCode_ERROR_CODE_INVALID_CREDENTIALS,
			hasDetail: true,
		},
		{
			name:     "ValidationException",
			code:     "ValidationException",
			grpcCode: codes.InvalidArgument,
		},
		{
			name:      "DataUnavailableException",
			code:      "DataUnavailableException",
			grpcCode:  codes.NotFound,
			detail:    pbc.ErrorCode_ERROR_CODE_RESOURCE_NOT_FOUND,
			hasDetail: true,
		},
		{
			name:     "unknown",
			code:     "ServiceException",
			grpcCode: codes.Internal,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()

			var buf bytes.Buffer
			mockAPI := &mockCostExplorerAPI{
				GetCostAndUsageFunc: func(context.Context, *costexplorer.GetCostAndUsageInput, ...func(*costexplorer.Options)) (*costexplorer.GetCostAndUsageOutput, error) {
					return nil, &leakyAPIError{code: tt.code, body: body}
				},
			}
			calc := NewCalculatorWithClient(client.NewClientWithAPI(mockAPI, "us-east-1"))
			calc.cache = nil
			calc.logger = zerolog.New(&buf)

			srv := pluginsdk.NewTestServer(t, calc)
			t.Cleanup(srv.Close)

			_, err := srv.Client().GetActualCost(context.Background(), serviceCostRequest())
			if status.Code(err) != tt.grpcCode {
				t.Fatalf("status.Code = %s, want %s (%v)", status.Code(err), tt.grpcCode, err)
			}
			msg := status.Convert(err).Message()
			wantMsg := "retrieving costs failed: " + tt.code
			if msg != wantMsg {
				t.Fatalf("message = %q, want %q", msg, wantMsg)
			}
			if strings.Contains(msg, roleARN) || strings.Contains(msg, secret) || strings.Contains(err.Error(), roleARN) || strings.Contains(err.Error(), secret) {
				t.Fatalf("status text contains credential material: %s", err)
			}
			logs := buf.String()
			if strings.Contains(logs, roleARN) || strings.Contains(logs, secret) || strings.Contains(logs, body) {
				t.Fatalf("log contains credential material: %s", logs)
			}
			details := errorDetailList(err)
			if !tt.hasDetail {
				if len(details) != 0 {
					t.Fatalf("ErrorDetail = %v, want none", details)
				}
				return
			}
			if len(details) != 1 || details[0] != tt.detail {
				t.Fatalf("ErrorDetail = %v, want [%s]", details, tt.detail)
			}
		})
	}
}

func errorDetailList(err error) []pbc.ErrorCode {
	var codesOut []pbc.ErrorCode
	for _, detail := range status.Convert(err).Details() {
		coded, ok := detail.(*pbc.ErrorDetail)
		if ok {
			codesOut = append(codesOut, coded.GetCode())
		}
	}
	return codesOut
}
