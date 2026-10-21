package pricing

import (
	"context"
	"encoding/json"
	"errors"
	"slices"
	"strings"
	"time"

	"github.com/rshade/finfocus-plugin-aws-ce/internal/client"
	"github.com/rshade/finfocus-spec/sdk/go/pluginsdk"
	pbc "github.com/rshade/finfocus-spec/sdk/go/proto/finfocus/v1"
	"google.golang.org/grpc/codes"
	"google.golang.org/grpc/status"
	"google.golang.org/protobuf/proto"
)

type carbonReader interface {
	ReadCarbonPage(context.Context, *pbc.GetCarbonEmissionsRequest, string) (client.CarbonPage, error)
}

// GetCarbonEmissions reports historical aggregate emissions when explicitly enabled.
func (c *Calculator) GetCarbonEmissions(ctx context.Context, req *pbc.GetCarbonEmissionsRequest) (*pbc.GetCarbonEmissionsResponse, error) {
	logger := c.traceLogger(ctx)
	done := pluginsdk.LogOperation(logger, "GetCarbonEmissions")
	defer done()
	if !c.carbonEnabled {
		return nil, status.Error(codes.FailedPrecondition, "historical carbon reporting requires carbon_enabled=true (FINFOCUS_AWS_CARBON_ENABLED)")
	}
	if pluginsdk.ValidateGetCarbonEmissionsRequest(req) != nil {
		return nil, status.Error(codes.InvalidArgument, "invalid historical carbon request")
	}
	for _, values := range [][]string{req.AccountIds, req.Regions, req.Services} {
		for _, value := range values {
			if strings.TrimSpace(value) == "" {
				return nil, status.Error(codes.InvalidArgument, "carbon filters require nonempty dimension values")
			}
		}
	}
	q := proto.CloneOf(req)
	pluginsdk.ApplyGetCarbonEmissionsDefaults(q)
	token := q.PageToken
	q.PageToken = ""
	q.AccountIds = carbonFilter(q.AccountIds)
	q.Regions = carbonFilter(q.Regions)
	q.Services = carbonFilter(q.Services)
	query, _ := proto.MarshalOptions{Deterministic: true}.Marshal(q)
	state := carbonCursor{Query: carbonDigest(query), Expires: c.currentTime().Add(15 * time.Minute).Unix()}
	if token != "" {
		checked, err := c.readCarbonQuery(token, state.Query)
		if err != nil {
			return nil, status.Error(codes.InvalidArgument, errCarbonCursor.Error())
		}
		state = checked
	}
	creds, err := pluginsdk.ExtractCredentials(ctx)
	if err != nil {
		return nil, status.Error(codes.InvalidArgument, "invalid credentials")
	}
	cfg := client.Config{}
	if creds.Len() != 0 {
		cfg, err = configFromCredentials(creds)
		if err != nil {
			return nil, status.Error(codes.InvalidArgument, "invalid credentials")
		}
	}
	cfg, err = client.ResolveCarbonCredentials(ctx, cfg)
	if err != nil {
		return nil, carbonStatus(err)
	}
	credentials, _ := json.Marshal([]string{cfg.AccessKeyID, cfg.SecretAccessKey, cfg.SessionToken, cfg.RoleARN})
	caller := carbonDigest(credentials)
	if token != "" {
		state, err = c.readCarbon(token, state.Query, caller)
		if err != nil {
			return nil, status.Error(codes.InvalidArgument, errCarbonCursor.Error())
		}
	}
	state.Caller = caller
	if err = ctx.Err(); err != nil {
		return nil, status.FromContextError(err).Err()
	}
	open := c.openCarbon
	if open == nil {
		open = func(ctx context.Context, cfg client.Config) (carbonReader, error) {
			return client.NewSustainabilityClient(ctx, cfg)
		}
	}
	reader, err := open(ctx, cfg)
	if err != nil {
		return nil, carbonStatus(err)
	}
	if reader == nil {
		return nil, status.Error(codes.Internal, "AWS sustainability client unavailable")
	}
	var attempts int
	ctx = c.withCostBudget(ctx, &attempts)
	page, err := reader.ReadCarbonPage(ctx, q, state.Token)
	if err != nil {
		return nil, carbonStatus(err)
	}
	response := &pbc.GetCarbonEmissionsResponse{Records: page.Records}
	if len(page.Records) > int(q.PageSize) || pluginsdk.ValidateGetCarbonEmissionsResponse(response) != nil {
		return nil, carbonStatus(client.ErrCarbonDataInvalid)
	}
	ids := map[string]bool{}
	for _, r := range page.Records {
		if ids[r.Id] || r.Provider != "aws" || r.Source != "aws-sustainability" || r.Method != q.Method || r.Scope != q.Scope || r.Start.AsTime().Before(q.Start.AsTime()) || r.End.AsTime().After(q.End.AsTime()) {
			return nil, carbonStatus(client.ErrCarbonDataInvalid)
		}
		ids[r.Id] = true
		for _, f := range []struct {
			values []string
			value  string
		}{{q.AccountIds, r.AccountId}, {q.Regions, r.Region}, {q.Services, r.Service}} {
			if len(f.values) > 0 && !slices.Contains(f.values, f.value) {
				return nil, carbonStatus(client.ErrCarbonDataInvalid)
			}
		}
	}
	state.Pages++
	if page.NextToken != "" {
		digest := carbonDigest([]byte(page.NextToken))
		if state.Pages >= 100 || len(page.NextToken) > 2000 || slices.Contains(state.Seen, digest) {
			return nil, status.Error(codes.Internal, "invalid AWS sustainability pagination")
		}
		state.Token = page.NextToken
		state.Seen = append(state.Seen, digest)
		response.NextPageToken, err = c.signCarbon(state)
		if err != nil {
			return nil, status.Error(codes.Internal, "invalid AWS sustainability pagination")
		}
	}
	return response, nil
}
func carbonFilter(values []string) []string { slices.Sort(values); return slices.Compact(values) }
func carbonStatus(err error) error {
	if errors.Is(err, context.Canceled) || errors.Is(err, context.DeadlineExceeded) {
		return status.FromContextError(err).Err()
	}
	if errors.Is(err, client.ErrRateLimited) {
		return status.Error(codes.ResourceExhausted, "AWS sustainability request budget exhausted")
	}
	code, _ := awsAPIErrorCode(err)
	mapped := codes.Internal
	switch code {
	case "ValidationException":
		mapped = codes.InvalidArgument
	case "AccessDenied", "AccessDeniedException", "UnauthorizedException":
		mapped = codes.PermissionDenied
	case "ExpiredToken", "ExpiredTokenException", "InvalidClientTokenId", "UnrecognizedClientException", "AuthFailure":
		mapped = codes.Unauthenticated
	case "Throttling", "ThrottlingException", "TooManyRequestsException":
		mapped = codes.ResourceExhausted
	case "InternalServerException", "ServiceUnavailableException":
		mapped = codes.Unavailable
	}
	return status.Error(mapped, "AWS sustainability request failed")
}
