package client

import (
	"context"
	"os"
	"path/filepath"
	"strings"
	"testing"
)

func TestNewClientMissingRegion(t *testing.T) {
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
	ce, err := NewClient(context.Background(), Config{AccessKeyID: "test", SecretAccessKey: "test"})
	if err == nil || ce != nil || !strings.Contains(err.Error(), "AWS_REGION=us-east-1") {
		t.Fatalf("missing region must provide a setup action: client=%v error=%v", ce, err)
	}
	// Explicit region remains sufficient; no credential retrieval or CE call occurs.
	ce, err = NewClient(context.Background(), Config{Region: "us-east-1", AccessKeyID: "test", SecretAccessKey: "test"})
	if err != nil || ce.Region() != "us-east-1" {
		t.Fatalf("configured region: %v,%v", ce, err)
	}
}
