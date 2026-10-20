package pricing

import (
	"bytes"
	"context"
	"errors"
	"google.golang.org/grpc"
	"google.golang.org/grpc/credentials/insecure"
	"net"
	"testing"

	"github.com/rs/zerolog"
	"github.com/rshade/finfocus-plugin-aws-ce/internal/client"
	"github.com/rshade/finfocus-spec/sdk/go/pluginsdk"
	pbc "github.com/rshade/finfocus-spec/sdk/go/proto/finfocus/v1"
	"github.com/stretchr/testify/require"
	"google.golang.org/grpc/metadata"
)

func TestRecommendationsWire(t *testing.T) {
	var logs bytes.Buffer
	calc := NewCalculator()
	calc.logger = zerolog.New(&logs)
	var configs []client.Config
	var apis []*recommendationAPI
	calc.openClient = func(_ context.Context, cfg client.Config) (*client.Client, error) {
		configs = append(configs, cfg)
		api := &recommendationAPI{rights: rightsizingFixture()}
		apis = append(apis, api)
		return client.NewClientWithAPI(api, "us-east-1"), nil
	}
	srv := pluginsdk.NewTestServer(t, calc)
	defer srv.Close()
	entries := map[string]string{"access_key_id": "synthetic-access", "secret_access_key": "synthetic-secret", "session_token": "synthetic-token", "role_arn": credentialRole}
	lis, err := net.Listen("tcp", "127.0.0.1:0")
	require.NoError(t, err)
	credentialServer := grpc.NewServer(grpc.UnaryInterceptor(pluginsdk.TracingUnaryServerInterceptorWithLogger(calc.logger)))
	pbc.RegisterCostSourceServiceServer(credentialServer, pluginsdk.NewServer(calc))
	go func() { _ = credentialServer.Serve(lis) }()
	defer credentialServer.Stop()
	conn, err := grpc.NewClient(lis.Addr().String(), grpc.WithTransportCredentials(insecure.NewCredentials()))
	require.NoError(t, err)
	defer func() { require.NoError(t, conn.Close()) }()
	rpc := pbc.NewCostSourceServiceClient(conn)
	ctx := context.Background()
	for k, v := range entries {
		ctx = metadata.AppendToOutgoingContext(ctx, pluginsdk.CredentialMetadataPrefix+k, v)
	}
	for _, callCtx := range []context.Context{ctx, context.Background()} {
		resp, err := rpc.GetRecommendations(callCtx, &pbc.GetRecommendationsRequest{})
		require.NoError(t, err)
		require.Len(t, resp.GetRecommendations(), 3)
		require.Equal(t, 612.5, resp.GetSummary().GetTotalEstimatedSavings())
	}
	require.Len(t, configs, 2)
	require.Equal(t, "synthetic-access", configs[0].AccessKeyID)
	require.Equal(t, "synthetic-secret", configs[0].SecretAccessKey)
	require.Equal(t, "synthetic-token", configs[0].SessionToken)
	require.Equal(t, credentialRole, configs[0].RoleARN)
	require.Empty(t, configs[1].AccessKeyID)
	require.Empty(t, configs[1].SecretAccessKey)
	require.Empty(t, configs[1].SessionToken)
	require.Empty(t, configs[1].RoleARN)
	for _, api := range apis {
		require.Equal(t, []string{"ec2", "COMPUTE_SP", "EC2_INSTANCE_SP"}, api.calls)
	}
	calc.openClient = func(context.Context, client.Config) (*client.Client, error) {
		return nil, errors.New("synthetic-access synthetic-secret synthetic-token " + credentialRole)
	}
	resp, err := rpc.GetRecommendations(ctx, &pbc.GetRecommendationsRequest{})
	require.Error(t, err)
	require.Nil(t, resp)
	for _, value := range entries {
		require.NotContains(t, err.Error(), value)
		require.NotContains(t, logs.String(), value)
	}
	sdkResp, sdkErr := srv.Client().GetRecommendations(context.Background(), &pbc.GetRecommendationsRequest{})
	require.NoError(t, sdkErr)
	require.Len(t, sdkResp.GetRecommendations(), 3)
	info, err := srv.Client().GetPluginInfo(context.Background(), &pbc.GetPluginInfoRequest{})
	require.NoError(t, err)
	require.Contains(t, info.GetCapabilities(), pbc.PluginCapability_PLUGIN_CAPABILITY_RECOMMENDATIONS)
	require.Contains(t, info.GetMetadata()["supported_rpcs"], "GetRecommendations")
	support, err := srv.Client().Supports(context.Background(), &pbc.SupportsRequest{Resource: &pbc.ResourceDescriptor{Provider: "aws", ResourceType: "ec2"}})
	require.NoError(t, err)
	require.Contains(t, support.GetCapabilitiesEnum(), pbc.PluginCapability_PLUGIN_CAPABILITY_RECOMMENDATIONS)
	require.True(t, support.GetCapabilities()["supports_recommendations"])
}
