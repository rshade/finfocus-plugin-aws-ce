package main

import (
	"os"
	"os/exec"
	"strings"
	"testing"
)

func TestBatchSettings(t *testing.T) {
	for _, tc := range []struct {
		name, max, workers   string
		wantMax, wantWorkers int
		bad                  string
	}{
		{name: "defaults", wantMax: 100, wantWorkers: 10},
		{name: "configured", max: "12", workers: "3", wantMax: 12, wantWorkers: 3},
		{name: "invalid size", max: "abc", bad: "FINFOCUS_AWS_CE_MAX_BATCH_SIZE"},
		{name: "negative workers", workers: "-1", bad: "FINFOCUS_AWS_CE_BATCH_WORKERS"},
		{name: "zero size", max: "0", bad: "FINFOCUS_AWS_CE_MAX_BATCH_SIZE"},
		{name: "oversized", max: "1001", bad: "FINFOCUS_AWS_CE_MAX_BATCH_SIZE"},
		{name: "too many workers", workers: "51", bad: "FINFOCUS_AWS_CE_BATCH_WORKERS"},
	} {
		t.Run(tc.name, func(t *testing.T) {
			t.Setenv("FINFOCUS_AWS_CE_MAX_BATCH_SIZE", tc.max)
			t.Setenv("FINFOCUS_AWS_CE_BATCH_WORKERS", tc.workers)
			max, workers, err := batchSettingsFromEnv()
			if tc.bad != "" {
				if err == nil || !strings.Contains(err.Error(), tc.bad) {
					t.Fatalf("error=%v want %s", err, tc.bad)
				}
				return
			}
			if err != nil || max != tc.wantMax || workers != tc.wantWorkers {
				t.Fatalf("settings=%d,%d,%v", max, workers, err)
			}
		})
	}
}

func TestBatchSettingsFailStartup(t *testing.T) {
	if value, _ := os.LookupEnv("FINFOCUS_CE_STARTUP_PROBE"); value == "true" {
		main()
		return
	}
	cmd := exec.Command(os.Args[0], "-test.run=^TestBatchSettingsFailStartup$")
	cmd.Env = append(os.Environ(), "FINFOCUS_CE_STARTUP_PROBE=true", "FINFOCUS_AWS_CE_BATCH_WORKERS=invalid", "FINFOCUS_LOG_FILE=")
	output, err := cmd.CombinedOutput()
	if err == nil {
		t.Fatalf("invalid settings did not fail startup: %s", output)
	}
	if !strings.Contains(string(output), "FINFOCUS_AWS_CE_BATCH_WORKERS") {
		t.Fatalf("missing actionable configuration error: %s", output)
	}
}
