package pricing

import (
	"context"
	"errors"
	"fmt"

	"github.com/rs/zerolog"
	"github.com/rshade/finfocus-plugin-aws-ce/internal/client"
	"github.com/rshade/finfocus-spec/sdk/go/pluginsdk"
	pbc "github.com/rshade/finfocus-spec/sdk/go/proto/finfocus/v1"
	"google.golang.org/grpc/codes"
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
		logger.Error().Err(err).Msg("Failed to initialize Cost Explorer client")
		return nil, true, fmt.Errorf("initializing Cost Explorer client: %w", err)
	}
	return ce, true, nil
}

func invalidCredentials(err error) error {
	return statusWithDetail(codes.InvalidArgument, err.Error(), pbc.ErrorCode_ERROR_CODE_INVALID_CREDENTIALS)
}

func configFromCredentials(creds pluginsdk.Credentials) (client.Config, error) {
	access, hasAccess := creds.Get("access_key_id")
	secret, hasSecret := creds.Get("secret_access_key")
	token, hasToken := creds.Get("session_token")
	role, _ := creds.Get("role_arn")

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
