package pricing

import (
	"bytes"
	"context"
	"fmt"
	"github.com/rs/zerolog"
	"github.com/rshade/finfocus-spec/sdk/go/pluginsdk"
	"google.golang.org/protobuf/proto"
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"

	"github.com/rshade/finfocus-plugin-aws-ce/internal/client"
	pbc "github.com/rshade/finfocus-spec/sdk/go/proto/finfocus/v1"
	"google.golang.org/grpc/codes"
	"google.golang.org/grpc/status"
)

func carbonSharedCredentials(t *testing.T) func(string) {
	t.Helper()
	dir := t.TempDir()
	path := filepath.Join(dir, "credentials")
	config := filepath.Join(dir, "config")
	if err := os.WriteFile(config, nil, 0600); err != nil {
		t.Fatal(err)
	}
	for _, name := range []string{"AWS_ACCESS_KEY_ID", "AWS_SECRET_ACCESS_KEY", "AWS_SESSION_TOKEN", "AWS_PROFILE", "AWS_DEFAULT_PROFILE", "AWS_ROLE_ARN", "AWS_WEB_IDENTITY_TOKEN_FILE", "AWS_CONTAINER_CREDENTIALS_RELATIVE_URI", "AWS_CONTAINER_CREDENTIALS_FULL_URI"} {
		t.Setenv(name, "")
	}
	t.Setenv("AWS_SHARED_CREDENTIALS_FILE", path)
	t.Setenv("AWS_CONFIG_FILE", config)
	t.Setenv("AWS_EC2_METADATA_DISABLED", "true")
	return func(key string) {
		t.Helper()
		if err := os.WriteFile(path, []byte(fmt.Sprintf("[default]\naws_access_key_id = %s\naws_secret_access_key = fixture-secret\naws_session_token = fixture-session\n", key)), 0600); err != nil {
			t.Fatal(err)
		}
	}
}
func TestCarbonAmbientCredentialRotation(t *testing.T) {
	for _, role := range []string{"", "arn:aws:iam::123456789012:role/carbon"} {
		t.Run(role, func(t *testing.T) {
			write := carbonSharedCredentials(t)
			write("caller-a")
			c := NewCalculator(WithCarbonEnabled(true))
			calls := 0
			c.openCarbon = func(_ context.Context, cfg client.Config) (carbonReader, error) {
				calls++
				if cfg.AccessKeyID != "caller-a" || cfg.RoleARN != role {
					t.Errorf("client did not get resolved base credentials and role")
				}
				return carbonReaderFunc(func(context.Context, *pbc.GetCarbonEmissionsRequest, string) (client.CarbonPage, error) {
					return client.CarbonPage{NextToken: "private-token"}, nil
				}), nil
			}
			ctx := context.Background()
			if role != "" {
				ctx = credentialContext(t, map[string]string{"role_arn": role})
			}
			q := carbonQuery()
			page, err := c.GetCarbonEmissions(ctx, q)
			if err != nil {
				t.Fatal(err)
			}
			write("caller-b")
			q.PageToken = page.NextPageToken
			if _, err = c.GetCarbonEmissions(ctx, q); status.Code(err) != codes.InvalidArgument {
				t.Fatalf("rotated credentials accepted: %v", err)
			}
			if calls != 1 {
				t.Fatalf("factory calls %d", calls)
			}
		})
	}
}

func TestCarbonInvalidCursorNeverAcquiresCredentials(t *testing.T) {
	write := carbonSharedCredentials(t)
	write("unused")
	if err := os.WriteFile(os.Getenv("AWS_SHARED_CREDENTIALS_FILE"), nil, 0600); err != nil {
		t.Fatal(err)
	}
	marker := filepath.Join(t.TempDir(), "credential-access")
	script := filepath.Join(t.TempDir(), "credential-process.sh")
	if err := os.WriteFile(script, []byte("#!/bin/sh\n: > '"+marker+"'\nprintf '%s' '{\"Version\":1,\"AccessKeyId\":\"process-key\",\"SecretAccessKey\":\"process-secret\"}'\n"), 0700); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(os.Getenv("AWS_CONFIG_FILE"), []byte("[default]\ncredential_process = sh "+script+"\n"), 0600); err != nil {
		t.Fatal(err)
	}
	c := NewCalculator(WithCarbonEnabled(true))
	c.openCarbon = func(context.Context, client.Config) (carbonReader, error) {
		t.Fatal("invalid cursor opened client")
		return nil, nil
	}
	q := carbonQuery()
	normalized := proto.CloneOf(q)
	pluginsdk.ApplyGetCarbonEmissionsDefaults(normalized)
	query, _ := proto.MarshalOptions{Deterministic: true}.Marshal(normalized)
	state := carbonCursor{Query: carbonDigest(query), Caller: "unused", Token: "aws-next", Pages: 1, Expires: c.currentTime().Add(time.Minute).Unix(), Seen: []string{carbonDigest([]byte("aws-next"))}}
	valid, err := c.signCarbon(state)
	if err != nil {
		t.Fatal(err)
	}
	state.Expires = c.currentTime().Add(-time.Minute).Unix()
	expired, err := c.signCarbon(state)
	if err != nil {
		t.Fatal(err)
	}
	for _, tc := range []struct {
		name, token string
		method      pbc.CarbonAccountingMethod
	}{{"bad-mac", valid + "x", 0}, {"wrong-query", valid, pbc.CarbonAccountingMethod_CARBON_ACCOUNTING_METHOD_LOCATION_BASED}, {"expired", expired, 0}} {
		t.Run(tc.name, func(t *testing.T) {
			q := carbonQuery()
			q.PageToken = tc.token
			q.Method = tc.method
			if _, err := c.GetCarbonEmissions(context.Background(), q); status.Code(err) != codes.InvalidArgument {
				t.Fatal(err)
			}
		})
	}
	disabled := NewCalculator()
	if _, err = disabled.GetCarbonEmissions(context.Background(), nil); status.Code(err) != codes.FailedPrecondition {
		t.Fatal(err)
	}
	if _, err = os.Stat(marker); !os.IsNotExist(err) {
		t.Fatalf("credential process accessed: %v", err)
	}
}
func TestCarbonCredentialAcquisitionErrorsSanitized(t *testing.T) {
	write := carbonSharedCredentials(t)
	write("unused")
	if err := os.WriteFile(os.Getenv("AWS_SHARED_CREDENTIALS_FILE"), nil, 0600); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(os.Getenv("AWS_CONFIG_FILE"), []byte("[default]\ncredential_process = sh -c 'echo private-credential-sentinel >&2; exit 1'\n"), 0600); err != nil {
		t.Fatal(err)
	}
	c := NewCalculator(WithCarbonEnabled(true))
	var logs bytes.Buffer
	c.logger = zerolog.New(&logs)
	c.openCarbon = func(context.Context, client.Config) (carbonReader, error) {
		t.Fatal("failed credential resolution opened client")
		return nil, nil
	}
	_, err := c.GetCarbonEmissions(context.Background(), carbonQuery())
	if status.Code(err) != codes.Internal || strings.Contains(err.Error(), "private-credential-sentinel") || strings.Contains(logs.String(), "private-credential-sentinel") {
		t.Fatalf("unsafe resolution failure %v, logs %q", err, logs.String())
	}
}
