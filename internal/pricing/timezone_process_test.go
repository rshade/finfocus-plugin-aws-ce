package pricing

import (
	"os"
	"os/exec"
	"regexp"
	"strings"
	"testing"
)

// timezoneProcess isolates time.Local from gRPC background goroutines.
func timezoneProcess(t *testing.T, zone, testName string) []byte {
	t.Helper()
	parts := strings.Split(testName, "/")
	for i, part := range parts {
		parts[i] = "^" + regexp.QuoteMeta(part) + "$"
	}
	cmd := exec.Command(os.Args[0], "-test.run="+strings.Join(parts, "/"), "-test.count=1")
	for _, entry := range os.Environ() {
		if !strings.HasPrefix(entry, "TZ=") && !strings.HasPrefix(entry, "FINFOCUS_CE_PERIOD_ZONE=") {
			cmd.Env = append(cmd.Env, entry)
		}
	}
	cmd.Env = append(cmd.Env, "TZ="+zone, "FINFOCUS_CE_PERIOD_ZONE="+zone)
	output, err := cmd.CombinedOutput()
	if err != nil {
		t.Fatalf("timezone %s subprocess failed: %v\n%s", zone, err, output)
	}
	return output
}
