package pricing

import (
	"encoding/json"
	"testing"
	"time"

	pbc "github.com/rshade/finfocus-spec/sdk/go/proto/finfocus/v1"
	"google.golang.org/grpc/codes"
	"google.golang.org/grpc/status"
)

// TestRequestPeriodStaysUTCAcrossHostZones runs every request_period fixture
// under UTC, America/Los_Angeles, and Pacific/Auckland. time.Unix keeps the
// host zone on the instant, so a Cost Explorer start formatted with time.Local
// changes the date. Pacific/Auckland moves 2026-09-01T23:30Z onto 2026-09-02.
func TestRequestPeriodStaysUTCAcrossHostZones(t *testing.T) {
	doc := loadContract(t)
	var cases []contractCase
	for _, tc := range doc.Cases {
		if tc.Kind == "request_period" {
			cases = append(cases, tc)
		}
	}
	if len(cases) != 8 {
		t.Fatalf("request_period cases = %d, want 8", len(cases))
	}

	zones := []string{"UTC", "America/Los_Angeles", "Pacific/Auckland"}
	for _, tc := range cases {
		t.Run(tc.ID, func(t *testing.T) {
			for _, zone := range zones {
				t.Run(zone, func(t *testing.T) {
					assertRequestPeriodInZone(t, tc, zone)
				})
			}
		})
	}
}

func assertRequestPeriodInZone(t *testing.T, tc contractCase, zone string) {
	t.Helper()
	loc, err := time.LoadLocation(zone)
	if err != nil {
		t.Fatalf("load zone: %v", err)
	}
	prev := time.Local
	time.Local = loc
	t.Cleanup(func() { time.Local = prev })
	t.Setenv("TZ", zone)

	// This instant is the one Auckland shifts forward. If the wall date ever
	// matches the UTC start, the case no longer catches a local format.
	if zone == "Pacific/Auckland" && tc.ID == "period:2026-09-01..2026-09-02:2" {
		localStart := time.Unix(tc.StartUnix, 0).Format("2006-01-02")
		if localStart == tc.Expect.Start {
			t.Fatalf("Auckland wall date %s equals UTC start %s", localStart, tc.Expect.Start)
		}
	}

	f := newFakeCE(t, []json.RawMessage{json.RawMessage(okPage)})
	start := time.Unix(tc.StartUnix, 0)
	end := time.Unix(tc.EndUnix, 0)
	resp, err := callActual(t, f, tc.ID, "", start, end)
	calls := f.snapshot()

	if tc.Expect.Outcome == "error" {
		if err == nil {
			t.Fatalf("got success, want InvalidArgument")
		}
		if status.Code(err) != codes.InvalidArgument {
			t.Fatalf("code %s, want InvalidArgument: %v", status.Code(err), err)
		}
		if got := errorDetailCode(err); got != pbc.ErrorCode_ERROR_CODE_INVALID_TIME_RANGE {
			t.Fatalf("detail %s, want ERROR_CODE_INVALID_TIME_RANGE", got)
		}
		if len(calls) != 0 {
			t.Fatalf("aws calls=%d, want 0", len(calls))
		}
		if resp != nil && len(resp.GetResults()) > 0 {
			t.Fatalf("returned %d cost rows", len(resp.GetResults()))
		}
		return
	}
	if err != nil {
		t.Fatalf("GetActualCost: %v", err)
	}
	if len(calls) != 1 {
		t.Fatalf("aws calls=%d, want 1", len(calls))
	}
	period := timePeriodOf(calls[0].Body)
	if period["Start"] != tc.Expect.Start || period["End"] != tc.Expect.End {
		t.Fatalf("TimePeriod %s..%s, want %s..%s", period["Start"], period["End"], tc.Expect.Start, tc.Expect.End)
	}
	if op := operationName(calls[0].Target); op != "GetCostAndUsage" {
		t.Fatalf("operation %s, want GetCostAndUsage", op)
	}
}
