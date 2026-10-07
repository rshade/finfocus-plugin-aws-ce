package pricing

import (
	"context"
	"fmt"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/rshade/finfocus-plugin-aws-ce/internal/client"
	"google.golang.org/grpc/codes"
	"google.golang.org/grpc/status"
)

func TestPerRequestMissingRegionError(t *testing.T) {
	for _, name := range []string{"AWS_REGION", "AWS_DEFAULT_REGION", "AWS_PROFILE", "AWS_DEFAULT_PROFILE"} {
		t.Setenv(name, "")
	}
	empty := filepath.Join(t.TempDir(), "aws-config")
	if err := os.WriteFile(empty, nil, 0600); err != nil {
		t.Fatal(err)
	}
	t.Setenv("AWS_CONFIG_FILE", empty)
	t.Setenv("AWS_SHARED_CREDENTIALS_FILE", empty)
	t.Setenv("AWS_EC2_METADATA_DISABLED", "true")
	entries := map[string]string{"access_key_id": "region-test-key", "secret_access_key": "region-test-secret"}
	calc := NewCalculator()
	calc.cache = nil
	_, err := calc.GetActualCost(credentialContext(t, entries), serviceCostRequest())
	if status.Code(err) != codes.FailedPrecondition || !strings.Contains(status.Convert(err).Message(), "AWS_REGION=us-east-1") {
		t.Fatalf("region setup action lost: %v", err)
	}
	// Wrapped factory errors preserve the safe sentinel action but discard diagnostics.
	calc.openClient = func(context.Context, client.Config) (*client.Client, error) {
		return nil, fmt.Errorf("%w: %s", client.ErrRegionMissing, entries["secret_access_key"])
	}
	_, err = calc.GetActualCost(credentialContext(t, entries), serviceCostRequest())
	if status.Code(err) != codes.FailedPrecondition || !strings.Contains(status.Convert(err).Message(), "AWS_REGION=us-east-1") {
		t.Fatalf("wrapped safe error: %v", err)
	}
	for _, value := range entries {
		if strings.Contains(err.Error(), value) {
			t.Fatal("credential exposed in configuration error")
		}
	}
}
