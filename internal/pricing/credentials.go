package pricing

import (
	"context"
	"errors"
	"regexp"
	"strings"

	"github.com/aws/smithy-go"
	"github.com/rs/zerolog"
	"github.com/rshade/finfocus-plugin-aws-ce/internal/client"
	"github.com/rshade/finfocus-spec/sdk/go/pluginsdk"
	pbc "github.com/rshade/finfocus-spec/sdk/go/proto/finfocus/v1"
	"google.golang.org/grpc/codes"
	"google.golang.org/grpc/status"
)

// errPerRequestCredentialsIncomplete is an access key, secret, or session token
// that is not a usable pair. The text has no credential name or value.
var errPerRequestCredentialsIncomplete = errors.New("per-request credentials are incomplete")

// ConsumesPerRequestCredentials opts the plugin into host-supplied credentials.
// The empty method is the whole declaration. Serve does not call it.
func (c *Calculator) ConsumesPerRequestCredentials() {}

// clientForCall returns the Cost Explorer client for this call.
// An empty credential set keeps the lazy default client. A non-empty set is
// used for this call only and is not stored on c.ceClient.
func (c *Calculator) clientForCall(ctx context.Context, logger zerolog.Logger) (*client.Client, bool, error) {
	creds, err := pluginsdk.ExtractCredentials(ctx)
	if err != nil {
		return nil, false, invalidCredentials(err)
	}
	if creds.Len() == 0 {
		if err := c.initClient(ctx, logger); err != nil {
			return nil, false, err
		}
		return c.ceClient, false, nil
	}

	cfg, err := configFromCredentials(creds)
	if err != nil {
		return nil, true, invalidCredentials(err)
	}
	open := c.openClient
	if open == nil {
		open = client.NewClient
	}
	ce, err := open(ctx, cfg)
	if err != nil {
		logger.Error().Msg("Failed to initialize per-request Cost Explorer client")
		return nil, true, status.Error(codes.Internal, "per-request client initialization failed")
	}
	return ce, true, nil
}

func invalidCredentials(err error) error {
	return statusWithDetail(codes.InvalidArgument, err.Error(), pbc.ErrorCode_ERROR_CODE_INVALID_CREDENTIALS)
}

// perRequestAWSFailure is the status and log text for a per-request AWS call.
// API failures use ErrorCode only. Generic failures use fixed text.
// Error and ErrorMessage may contain credentials, so neither is logged.
func perRequestAWSFailure(perRequest bool, err error) (string, bool) {
	if !perRequest {
		return "", false
	}
	code, ok := awsAPIErrorCode(err)
	if !ok {
		return "retrieving costs failed", true
	}
	return awsAPIErrorText(code), true
}

// awsAPIErrorCode returns smithy.APIError.ErrorCode. Error and ErrorMessage
// are not read: they can contain a role ARN.
func awsAPIErrorCode(err error) (string, bool) {
	var api smithy.APIError
	if err == nil || !errors.As(err, &api) {
		return "", false
	}
	return api.ErrorCode(), true
}

func awsAPIErrorText(code string) string {
	if code == "" {
		return "retrieving costs failed"
	}
	return "retrieving costs failed: " + code
}

// mapAWSAPIError classifies a Cost Explorer failure by ErrorCode.
// Unknown codes stay Internal with no ErrorDetail. The status text is the
// safe code string, never Error or ErrorMessage.
func mapAWSAPIError(err error) (error, bool) {
	code, ok := awsAPIErrorCode(err)
	if !ok {
		return nil, false
	}
	msg := awsAPIErrorText(code)
	switch code {
	case "LimitExceededException", "RequestLimitExceeded":
		return statusWithDetail(codes.ResourceExhausted, msg, pbc.ErrorCode_ERROR_CODE_RATE_LIMITED), true
	case "AccessDenied", "AccessDeniedException":
		return statusWithDetail(codes.PermissionDenied, msg, pbc.ErrorCode_ERROR_CODE_PERMISSION_DENIED), true
	case "ExpiredToken", "ExpiredTokenException", "InvalidClientTokenId", "UnrecognizedClientException", "AuthFailure":
		return statusWithDetail(codes.Unauthenticated, msg, pbc.ErrorCode_ERROR_CODE_INVALID_CREDENTIALS), true
	case "ValidationException", "InvalidParameterException", "InvalidParameterValue", "InvalidNextTokenException":
		return status.Error(codes.InvalidArgument, msg), true
	case "DataUnavailableException":
		return statusWithDetail(codes.NotFound, msg, pbc.ErrorCode_ERROR_CODE_RESOURCE_NOT_FOUND), true
	default:
		return status.Error(codes.Internal, msg), true
	}
}

var credentialRoleARNPattern = regexp.MustCompile(`^arn:aws(?:-[a-z0-9-]+)?:iam::[0-9]{12}:role/[^[:space:]]+$`)

func configFromCredentials(creds pluginsdk.Credentials) (client.Config, error) {
	for _, name := range creds.Names() {
		switch name {
		case "access_key_id", "secret_access_key", "session_token", "role_arn":
		default:
			return client.Config{}, errors.New("unsupported per-request credential key")
		}
		value, _ := creds.Get(name)
		if strings.TrimSpace(value) == "" {
			return client.Config{}, errPerRequestCredentialsIncomplete
		}
	}

	access, hasAccess := creds.Get("access_key_id")
	secret, hasSecret := creds.Get("secret_access_key")
	token, hasToken := creds.Get("session_token")
	role, _ := creds.Get("role_arn")
	if role != "" && !credentialRoleARNPattern.MatchString(role) {
		return client.Config{}, errors.New("per-request credential role ARN is invalid")
	}

	if hasAccess || hasSecret || hasToken {
		if !hasAccess || !hasSecret {
			return client.Config{}, errPerRequestCredentialsIncomplete
		}
		return client.Config{
			AccessKeyID:     access,
			SecretAccessKey: secret,
			SessionToken:    token,
			RoleARN:         role,
		}, nil
	}
	if role != "" {
		return client.Config{RoleARN: role}, nil
	}
	return client.Config{}, errPerRequestCredentialsIncomplete
}
