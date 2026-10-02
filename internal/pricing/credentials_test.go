package pricing

import (
	"bytes"
	"context"
	"net"
	"strings"
	"testing"
	"time"

	"github.com/aws/aws-sdk-go-v2/service/costexplorer"
	"github.com/rs/zerolog"
	"github.com/rshade/finfocus-plugin-aws-ce/internal/client"
	"github.com/rshade/finfocus-spec/sdk/go/pluginsdk"
	pbc "github.com/rshade/finfocus-spec/sdk/go/proto/finfocus/v1"
	"google.golang.org/grpc"
	"google.golang.org/grpc/codes"
	"google.golang.org/grpc/credentials/insecure"
	"google.golang.org/grpc/metadata"
	"google.golang.org/grpc/status"
)

const (
	credentialSecret = "ce16-secret-value"
	credentialRole   = "arn:aws:iam::123456789012:role/finfocus-ce"
)

func TestGetActualCost_PerRequestCredentials(t *testing.T) {
	t.Parallel()

	var buf bytes.Buffer
	var got []client.Config
	apis := []*mockCostExplorerAPI{pricedCostAPI(), pricedCostAPI()}
	calc := NewCalculator()
	calc.cache = nil
	calc.logger = zerolog.New(&buf)
	var n int
	calc.openClient = func(_ context.Context, cfg client.Config) (*client.Client, error) {
		got = append(got, cfg)
		api := apis[n]
		n++
		return client.NewClientWithAPI(api, "us-east-1"), nil
	}

	req := serviceCostRequest()
	ctx := credentialContext(t, map[string]string{
		"access_key_id":     "AKIACE16ALPHA",
		"secret_access_key": credentialSecret,
	})
	resp, err := calc.GetActualCost(ctx, req)
	if err != nil {
		t.Fatalf("GetActualCost: %v", err)
	}
	if resp == nil || len(resp.GetResults()) != 1 || resp.GetResults()[0].GetCost() != 1 {
		t.Fatalf("cost result changed: %#v", resp)
	}
	if len(got) != 1 {
		t.Fatalf("openClient calls = %d, want 1", len(got))
	}
	if got[0].AccessKeyID != "AKIACE16ALPHA" || got[0].SecretAccessKey != credentialSecret {
		t.Fatalf("openClient config = %+v", got[0])
	}
	if strings.Contains(buf.String(), credentialSecret) || strings.Contains(buf.String(), "AKIACE16ALPHA") {
		t.Fatalf("credential material in logs: %s", buf.String())
	}

	ctx2 := credentialContext(t, map[string]string{
		"access_key_id":     "AKIACE16BETA",
		"secret_access_key": "ce16-other-secret",
	})
	if _, err := calc.GetActualCost(ctx2, req); err != nil {
		t.Fatalf("second GetActualCost: %v", err)
	}
	if len(got) != 2 || got[1].AccessKeyID != "AKIACE16BETA" {
		t.Fatalf("second config = %+v (calls=%d)", got, len(got))
	}
	if len(apis[0].costCalls) != 1 || len(apis[1].costCalls) != 1 {
		t.Fatalf("serving clients = %d and %d calls, want 1 and 1", len(apis[0].costCalls), len(apis[1].costCalls))
	}
	if strings.Contains(buf.String(), "ce16-other-secret") || strings.Contains(buf.String(), "AKIACE16BETA") {
		t.Fatalf("second credential material in logs: %s", buf.String())
	}
}

func TestGetActualCost_CredentialShapes(t *testing.T) {
	t.Parallel()

	const token = "ce16-session-token"
	tests := []struct {
		name    string
		entries map[string]string
		want    client.Config
		bad     bool
	}{
		{
			name: "keys and session token",
			entries: map[string]string{
				"access_key_id":     "AKIACE16KEYS",
				"secret_access_key": credentialSecret,
				"session_token":     token,
			},
			want: client.Config{
				AccessKeyID:     "AKIACE16KEYS",
				SecretAccessKey: credentialSecret,
				SessionToken:    token,
			},
		},
		{
			name:    "role only",
			entries: map[string]string{"role_arn": credentialRole},
			want:    client.Config{RoleARN: credentialRole},
		},
		{
			name: "keys and role",
			entries: map[string]string{
				"access_key_id":     "AKIACE16ROLE",
				"secret_access_key": credentialSecret,
				"role_arn":          credentialRole,
			},
			want: client.Config{
				AccessKeyID:     "AKIACE16ROLE",
				SecretAccessKey: credentialSecret,
				RoleARN:         credentialRole,
			},
		},
		{
			name:    "access key only",
			entries: map[string]string{"access_key_id": "AKIACE16LONE"},
			bad:     true,
		},
		{
			name:    "secret only",
			entries: map[string]string{"secret_access_key": credentialSecret},
			bad:     true,
		},
		{
			name:    "session token only",
			entries: map[string]string{"session_token": token},
			bad:     true,
		},
		{
			name: "session token and role without keys",
			entries: map[string]string{
				"session_token": token,
				"role_arn":      credentialRole,
			},
			bad: true,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()

			var got []client.Config
			calc := NewCalculatorWithClient(client.NewClientWithAPI(pricedCostAPI(), "us-east-1"))
			calc.cache = nil
			calc.openClient = func(_ context.Context, cfg client.Config) (*client.Client, error) {
				got = append(got, cfg)
				return client.NewClientWithAPI(pricedCostAPI(), "us-east-1"), nil
			}

			_, err := calc.GetActualCost(credentialContext(t, tt.entries), serviceCostRequest())
			if tt.bad {
				assertInvalidCredentials(t, err, tt.entries)
				if len(got) != 0 {
					t.Fatalf("openClient called for incomplete credentials: %+v", got)
				}
				return
			}
			if err != nil {
				t.Fatalf("GetActualCost: %v", err)
			}
			if len(got) != 1 || got[0] != tt.want {
				t.Fatalf("config = %+v, want %+v", got, tt.want)
			}
		})
	}
}

func TestGetActualCost_EmptyCredentialsUseInjectedClient(t *testing.T) {
	t.Parallel()

	injected := pricedCostAPI()
	calc := NewCalculatorWithClient(client.NewClientWithAPI(injected, "us-east-1"))
	calc.cache = nil
	calc.openClient = func(context.Context, client.Config) (*client.Client, error) {
		t.Fatal("openClient called without credentials")
		return nil, nil
	}

	resp, err := calc.GetActualCost(context.Background(), serviceCostRequest())
	if err != nil {
		t.Fatalf("GetActualCost: %v", err)
	}
	if resp == nil || len(resp.GetResults()) != 1 || resp.GetResults()[0].GetCost() != 1 {
		t.Fatalf("cost result changed: %#v", resp)
	}
	if len(injected.costCalls) != 1 {
		t.Fatalf("injected client calls = %d, want 1", len(injected.costCalls))
	}
}

func TestGetActualCost_PerRequestSkipsSharedCache(t *testing.T) {
	t.Parallel()

	t.Run("cached default is not served", func(t *testing.T) {
		t.Parallel()

		injected := pricedCostAPI()
		perRequest := pricedCostAPI()
		calc := calculatorWithCache(t, injected)
		calc.openClient = func(context.Context, client.Config) (*client.Client, error) {
			return client.NewClientWithAPI(perRequest, "us-east-1"), nil
		}

		if _, err := calc.GetActualCost(context.Background(), serviceCostRequest()); err != nil {
			t.Fatalf("default call: %v", err)
		}
		if _, err := calc.GetActualCost(credentialContext(t, map[string]string{
			"access_key_id":     "AKIACE16CACHE",
			"secret_access_key": credentialSecret,
		}), serviceCostRequest()); err != nil {
			t.Fatalf("credentialed call: %v", err)
		}
		if len(perRequest.costCalls) != 1 {
			t.Fatalf("per-request client calls = %d, want 1", len(perRequest.costCalls))
		}
	})

	t.Run("per-request result is not reused", func(t *testing.T) {
		t.Parallel()

		injected := pricedCostAPI()
		perRequest := pricedCostAPI()
		calc := calculatorWithCache(t, injected)
		calc.openClient = func(context.Context, client.Config) (*client.Client, error) {
			return client.NewClientWithAPI(perRequest, "us-east-1"), nil
		}

		ctx := credentialContext(t, map[string]string{
			"access_key_id":     "AKIACE16CACHE",
			"secret_access_key": credentialSecret,
		})
		if _, err := calc.GetActualCost(ctx, serviceCostRequest()); err != nil {
			t.Fatalf("credentialed call: %v", err)
		}
		if _, err := calc.GetActualCost(context.Background(), serviceCostRequest()); err != nil {
			t.Fatalf("default call: %v", err)
		}
		if len(injected.costCalls) != 1 {
			t.Fatalf("injected client calls = %d, want 1", len(injected.costCalls))
		}
	})
}

func TestGetActualCost_MalformedCredentialHeader(t *testing.T) {
	t.Parallel()

	const leakedName = "leakedname"
	const leakedValue = "leaked-secret-value"

	var buf bytes.Buffer
	logger := zerolog.New(&buf)
	calc := NewCalculator()
	calc.cache = nil
	calc.logger = logger
	calc.openClient = func(context.Context, client.Config) (*client.Client, error) {
		t.Fatal("openClient called for malformed credentials")
		return nil, nil
	}

	lis, err := net.Listen("tcp", "127.0.0.1:0")
	if err != nil {
		t.Fatalf("listen: %v", err)
	}
	srv := grpc.NewServer(grpc.UnaryInterceptor(pluginsdk.TracingUnaryServerInterceptorWithLogger(logger)))
	pbc.RegisterCostSourceServiceServer(srv, pluginsdk.NewServer(calc))
	go func() {
		if serveErr := srv.Serve(lis); serveErr != nil {
			return
		}
	}()
	t.Cleanup(srv.Stop)

	conn, err := grpc.NewClient(lis.Addr().String(), grpc.WithTransportCredentials(insecure.NewCredentials()))
	if err != nil {
		t.Fatalf("dial: %v", err)
	}
	t.Cleanup(func() { _ = conn.Close() })

	// Two values for one name is malformed wire material. The interceptor copies
	// it onto the context. The plugin must not read the header itself.
	ctx := metadata.NewOutgoingContext(context.Background(), metadata.Pairs(
		pluginsdk.CredentialMetadataPrefix+leakedName, leakedValue,
		pluginsdk.CredentialMetadataPrefix+leakedName, leakedValue,
	))
	_, err = pbc.NewCostSourceServiceClient(conn).GetActualCost(ctx, serviceCostRequest())
	assertInvalidCredentials(t, err, map[string]string{leakedName: leakedValue})
	if status.Convert(err).Message() != pluginsdk.ErrMalformedCredentials.Error() {
		t.Fatalf("message = %q, want %q", status.Convert(err).Message(), pluginsdk.ErrMalformedCredentials.Error())
	}
	if strings.Contains(buf.String(), leakedName) || strings.Contains(buf.String(), leakedValue) {
		t.Fatalf("credential material in logs: %s", buf.String())
	}
}

func calculatorWithCache(t *testing.T, api *mockCostExplorerAPI) *Calculator {
	t.Helper()
	cache, err := NewCacheManager(t.TempDir(), time.Hour)
	if err != nil {
		t.Fatalf("cache: %v", err)
	}
	calc := NewCalculatorWithClient(client.NewClientWithAPI(api, "us-east-1"))
	calc.cache = cache
	return calc
}

func pricedCostAPI() *mockCostExplorerAPI {
	return &mockCostExplorerAPI{
		GetCostAndUsageFunc: func(context.Context, *costexplorer.GetCostAndUsageInput, ...func(*costexplorer.Options)) (*costexplorer.GetCostAndUsageOutput, error) {
			return pricedCostOutput(), nil
		},
	}
}

func credentialContext(t *testing.T, entries map[string]string) context.Context {
	t.Helper()
	creds, err := pluginsdk.NewCredentials(entries)
	if err != nil {
		t.Fatalf("NewCredentials: %v", err)
	}
	return pluginsdk.WithCredentials(context.Background(), creds)
}

func assertInvalidCredentials(t *testing.T, err error, secret map[string]string) {
	t.Helper()
	if status.Code(err) != codes.InvalidArgument {
		t.Fatalf("status.Code = %s, want %s (%v)", status.Code(err), codes.InvalidArgument, err)
	}
	if got := errorDetailCode(err); got != pbc.ErrorCode_ERROR_CODE_INVALID_CREDENTIALS {
		t.Fatalf("ErrorDetail code = %s, want %s; details=%v", got, pbc.ErrorCode_ERROR_CODE_INVALID_CREDENTIALS, status.Convert(err).Details())
	}
	text := status.Convert(err).Message()
	for name, value := range secret {
		if strings.Contains(text, name) || strings.Contains(text, value) {
			t.Fatalf("status text %q contains credential material", text)
		}
	}
}
