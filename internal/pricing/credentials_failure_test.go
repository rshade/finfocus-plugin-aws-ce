package pricing

import (
	"bytes"
	"context"
	"net"
	"net/http"
	"net/http/httptest"
	"net/url"
	"strings"
	"testing"

	"github.com/rs/zerolog"
	"github.com/rshade/finfocus-plugin-aws-ce/internal/client"
	"google.golang.org/grpc/codes"
	"google.golang.org/grpc/status"
)

func TestGetActualCost_AssumeRoleDeniedOmitsRoleARN(t *testing.T) {
	t.Setenv("AWS_EC2_METADATA_DISABLED", "true")

	const roleARN = "arn:aws:iam::123456789012:role/finfocus-denied"
	const secret = "ce16-denied-secret"
	const denied = "not authorized to perform: sts:AssumeRole on resource: " + roleARN

	lis, err := net.Listen("tcp", "127.0.0.1:0")
	if err != nil {
		t.Fatalf("listen: %v", err)
	}
	srv := httptest.NewUnstartedServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "text/xml")
		w.WriteHeader(http.StatusForbidden)
		_, _ = w.Write([]byte(`<ErrorResponse><Error><Code>AccessDenied</Code><Message>` + denied + `</Message></Error><RequestId>ce16-denied</RequestId></ErrorResponse>`))
	}))
	srv.Listener = lis
	srv.Start()
	t.Cleanup(srv.Close)
	endpoint, err := url.Parse(srv.URL)
	if err != nil {
		t.Fatalf("parse endpoint: %v", err)
	}
	if endpoint.Hostname() != "127.0.0.1" {
		t.Fatalf("dial target %s is not 127.0.0.1", endpoint.Host)
	}

	var buf bytes.Buffer
	calc := NewCalculator()
	calc.cache = nil
	calc.logger = zerolog.New(&buf)
	calc.openClient = func(ctx context.Context, cfg client.Config) (*client.Client, error) {
		cfg.Region = "us-east-1"
		cfg.BaseEndpoint = srv.URL
		return client.NewClient(ctx, cfg)
	}

	_, err = calc.GetActualCost(credentialContext(t, map[string]string{
		"access_key_id":     "AKIACE16DENIED",
		"secret_access_key": secret,
		"role_arn":          roleARN,
	}), serviceCostRequest())
	if status.Code(err) != codes.Internal {
		t.Fatalf("status.Code = %s, want %s (%v)", status.Code(err), codes.Internal, err)
	}
	msg := status.Convert(err).Message()
	if msg != "retrieving costs failed: AccessDenied" {
		t.Fatalf("message = %q", msg)
	}
	if strings.Contains(msg, roleARN) || strings.Contains(msg, secret) || strings.Contains(err.Error(), roleARN) || strings.Contains(err.Error(), secret) {
		t.Fatalf("status text contains credential material: %s", err)
	}
	logs := buf.String()
	if strings.Contains(logs, roleARN) || strings.Contains(logs, secret) || strings.Contains(logs, denied) {
		t.Fatalf("log contains credential material: %s", logs)
	}
}
