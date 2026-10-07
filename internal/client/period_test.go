package client

import (
	"errors"
	"testing"
	"time"
)

func TestCostExplorerPeriod(t *testing.T) {
	start := time.Date(2026, 10, 1, 12, 0, 0, 0, time.UTC)
	midnight := time.Date(2026, 10, 4, 0, 0, 0, 0, time.UTC)
	for _, tc := range []struct {
		name    string
		end     time.Time
		wantEnd string
	}{
		{"midnight", midnight, "2026-10-04"},
		{"one nanosecond after midnight", midnight.Add(time.Nanosecond), "2026-10-05"},
		{"same instant in another zone", midnight.In(time.FixedZone("offset", -7*60*60)), "2026-10-04"},
	} {
		t.Run(tc.name, func(t *testing.T) {
			gotStart, gotEnd, err := CostExplorerPeriod(start, tc.end)
			if err != nil || gotStart != "2026-10-01" || gotEnd != tc.wantEnd {
				t.Fatalf("period = %q..%q, %v; want 2026-10-01..%s", gotStart, gotEnd, err, tc.wantEnd)
			}
		})
	}
	_, _, err := CostExplorerPeriod(start, start)
	if !errors.Is(err, ErrInvalidTimeRange) {
		t.Fatalf("equal instants: got %v, want ErrInvalidTimeRange", err)
	}
}
