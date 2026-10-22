package client

import (
	"context"
	"errors"
	"fmt"
	"github.com/aws/smithy-go"
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"strings"
	"testing"
)

func TestCarbonResolvedCredentialsFrozen(t *testing.T) {
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
	write := func(key string) {
		t.Helper()
		if err := os.WriteFile(path, []byte("[default]\naws_access_key_id = "+key+"\naws_secret_access_key = fixture-secret\n"), 0600); err != nil {
			t.Fatal(err)
		}
	}
	write("caller-a")
	endpoint := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if !strings.Contains(r.Header.Get("Authorization"), "caller-a/") {
			t.Error("credential context changed after resolution")
		}
		w.Header().Set("Content-Type", "application/json")
		_, _ = w.Write([]byte(`{"Results":[]}`))
	}))
	defer endpoint.Close()
	cfg, err := ResolveCarbonCredentials(context.Background(), Config{BaseEndpoint: endpoint.URL})
	if err != nil {
		t.Fatal(err)
	}
	write("caller-b")
	reader, err := NewSustainabilityClient(context.Background(), cfg)
	if err != nil {
		t.Fatal(err)
	}
	if _, err = reader.ReadCarbonPage(context.Background(), carbonRequest(), ""); err != nil {
		t.Fatal(err)
	}
}

func TestCarbonCredentialErrorClassification(t *testing.T) {
	for _, code := range []string{"AccessDenied", "AccessDeniedException", "ExpiredToken", "ExpiredTokenException", "InvalidClientTokenId", "UnrecognizedClientException", "AuthFailure", "ThrottlingException", "TooManyRequestsException"} {
		t.Run(code, func(t *testing.T) {
			upstream := fmt.Errorf("private-wrapper: %w", &smithy.GenericAPIError{Code: code, Message: "private-credential-diagnostic"})
			safe := carbonCredentialError(context.Background(), upstream)
			var api smithy.APIError
			if !errors.As(safe, &api) || api.ErrorCode() != code {
				t.Fatalf("classification lost: %v", safe)
			}
			if strings.Contains(safe.Error(), "private") {
				t.Fatalf("unsafe error %v", safe)
			}
		})
	}
	for _, upstream := range []error{fmt.Errorf("private-wrapper: %w", &smithy.GenericAPIError{Code: "private-unknown-code", Message: "private-credential-diagnostic"}), errors.New("credential_process private-credential-diagnostic")} {
		safe := carbonCredentialError(context.Background(), upstream)
		if strings.Contains(safe.Error(), "private") || strings.Contains(safe.Error(), "credential_process") {
			t.Fatalf("unsafe error %v", safe)
		}
		var api smithy.APIError
		if errors.As(safe, &api) && api.ErrorCode() != "UpstreamError" {
			t.Fatalf("unsafe code %q", api.ErrorCode())
		}
	}
	for _, sentinel := range []error{context.Canceled, context.DeadlineExceeded} {
		safe := carbonCredentialError(context.Background(), fmt.Errorf("private: %w", sentinel))
		if !errors.Is(safe, sentinel) || strings.Contains(safe.Error(), "private") {
			t.Fatalf("context classification lost: %v", safe)
		}
	}
	ctx, cancel := context.WithCancel(context.Background())
	cancel()
	if !errors.Is(carbonCredentialError(ctx, errors.New("private")), context.Canceled) {
		t.Fatal("context cancellation lost")
	}
}
