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
		{name: "defaults", wantMax: 100, wantWorkers: 50},
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

func TestCarbonSettingsFromEnv(t *testing.T) {
	for _, tc := range []struct {
		name, value          string
		unset, want, wantErr bool
	}{
		{name: "unset", unset: true}, {name: "false", value: "false"}, {name: "true", value: "true", want: true},
		{name: "blank", value: ""}, {name: "whitespace", value: "   "}, {name: "case", value: " TrUe ", want: true},
		{name: "invalid", value: "yes", wantErr: true}, {name: "numeric", value: "1", wantErr: true},
	} {
		t.Run(tc.name, func(t *testing.T) {
			t.Setenv("FINFOCUS_AWS_CARBON_ENABLED", tc.value)
			if tc.unset {
				if err := os.Unsetenv("FINFOCUS_AWS_CARBON_ENABLED"); err != nil {
					t.Fatal(err)
				}
			}
			got, err := carbonSettingsFromEnv()
			if (err != nil) != tc.wantErr {
				t.Fatalf("error = %v, wantErr %v", err, tc.wantErr)
			}
			if got != tc.want {
				t.Errorf("enabled = %v, want %v", got, tc.want)
			}
			if err != nil && !strings.Contains(err.Error(), "FINFOCUS_AWS_CARBON_ENABLED") {
				t.Errorf("error does not name setting: %v", err)
			}
		})
	}
}

func TestCarbonSettingsFailStartup(t *testing.T) {
	if value, _ := os.LookupEnv("FINFOCUS_CARBON_STARTUP_PROBE"); value == "true" {
		main()
		return
	}
	cmd := exec.Command(os.Args[0], "-test.run=^TestCarbonSettingsFailStartup$")
	cmd.Env = append(os.Environ(), "FINFOCUS_CARBON_STARTUP_PROBE=true", "FINFOCUS_AWS_CARBON_ENABLED=invalid", "FINFOCUS_AWS_CE_BATCH_WORKERS=1", "FINFOCUS_AWS_CE_MAX_BATCH_SIZE=1", "FINFOCUS_LOG_FILE=")
	output, err := cmd.CombinedOutput()
	if err == nil || !strings.Contains(string(output), "FINFOCUS_AWS_CARBON_ENABLED must be true or false") {
		t.Fatalf("startup error=%v output=%s", err, output)
	}
}
