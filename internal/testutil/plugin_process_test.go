package testutil

import (
	"testing"
	"time"
)

func TestPluginProcessConfiguredLifetime(t *testing.T) {
	proc := StartPluginWithTimeout(t, 2*time.Second, map[string]string{"AWS_EC2_METADATA_DISABLED": "true", "FINFOCUS_PLUGIN_PORT": "0"}, "--port", "0")
	select {
	case err := <-proc.done:
		// Preserve the exit result for the cleanup owner.
		proc.done <- err
		if err == nil {
			t.Fatal("expected process termination when its lifetime expires")
		}
	case <-time.After(4 * time.Second):
		t.Fatal("configured process lifetime was not enforced")
	}
}
