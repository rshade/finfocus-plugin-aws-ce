package pricing

import (
	"context"
	"encoding/json"
	"fmt"
	"io"
	"math"
	"math/big"
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"runtime"
	"strings"
	"sync"
	"testing"
	"time"

	_ "time/tzdata"

	"github.com/rshade/finfocus-plugin-aws-ce/internal/client"
	"github.com/rshade/finfocus-spec/sdk/go/pluginsdk"
	pbc "github.com/rshade/finfocus-spec/sdk/go/proto/finfocus/v1"
	"google.golang.org/grpc/codes"
	"google.golang.org/grpc/status"
	"google.golang.org/protobuf/types/known/timestamppb"
)

const (
	contractCurrencyColumn  = "currency"
	contractEstimatedColumn = "estimated"
	contractGroupKeyColumn  = "group_key"
	contractAmountColumn    = "amount_decimal"
)

// okPage is a valid Cost Explorer page so request-shape cases can finish the RPC
// after the plugin has sent the call we need to inspect.
const okPage = `{"ResultsByTime":[{"Estimated":false,"Groups":[{"Keys":["Amazon Simple Storage Service"],"Metrics":{"UnblendedCost":{"Amount":"1.00","Unit":"USD"}}}],"TimePeriod":{"Start":"2026-09-20","End":"2026-09-21"}}]}`

type contractDoc struct {
	Cases []contractCase `json:"cases"`
}

type contractCase struct {
	ID             string            `json:"id"`
	Kind           string            `json:"kind"`
	Why            string            `json:"why"`
	Input          string            `json:"input"`
	StartUnix      int64             `json:"start_unix"`
	EndUnix        int64             `json:"end_unix"`
	RequestAgeDays int               `json:"request_age_days"`
	Pages          []json.RawMessage `json:"pages"`
	Body           map[string]any    `json:"body"`
	Expect         contractExpect    `json:"expect"`
}

type contractExpect struct {
	Outcome    string            `json:"outcome"`
	Currency   string            `json:"currency"`
	Estimated  *bool             `json:"estimated"`
	PageCount  *int              `json:"page_count"`
	Total      string            `json:"total"`
	PerService map[string]string `json:"per_service"`
	GRPCCode   string            `json:"grpc_code"`
	Never      string            `json:"never"`
	Reason     string            `json:"reason"`
	Start      string            `json:"Start"`
	End        string            `json:"End"`
	Operation  string            `json:"operation"`
	Dimension  string            `json:"dimension"`
	Value      string            `json:"value"`
	Note       string            `json:"note"`
}

type contractRow struct {
	Case     string
	Expected string
	Actual   string
	Verdict  string
}

type capturedCall struct {
	Target string
	Body   []byte
}

type fakeCE struct {
	srv   *httptest.Server
	mu    sync.Mutex
	calls []capturedCall
}

func newFakeCE(t *testing.T, pages []json.RawMessage) *fakeCE {
	t.Helper()
	f := &fakeCE{}
	f.srv = httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		body, _ := io.ReadAll(r.Body)
		f.mu.Lock()
		f.calls = append(f.calls, capturedCall{Target: r.Header.Get("X-Amz-Target"), Body: append([]byte(nil), body...)})
		idx := len(f.calls) - 1
		f.mu.Unlock()
		w.Header().Set("Content-Type", "application/x-amz-json-1.1")
		payload := []byte(`{"ResultsByTime":[]}`)
		if len(pages) > 0 {
			if idx >= len(pages) {
				idx = len(pages) - 1
			}
			payload = pages[idx]
		}
		_, _ = w.Write(payload)
	}))
	t.Cleanup(f.srv.Close)
	return f
}

func (f *fakeCE) snapshot() []capturedCall {
	f.mu.Lock()
	defer f.mu.Unlock()
	out := make([]capturedCall, len(f.calls))
	copy(out, f.calls)
	return out
}

func TestCEContractFixtures(t *testing.T) {
	doc := loadContract(t)
	if len(doc.Cases) != 26 {
		t.Fatalf("contract cases = %d, want 26", len(doc.Cases))
	}

	var (
		mu   sync.Mutex
		rows []contractRow
	)
	add := func(row contractRow) {
		mu.Lock()
		rows = append(rows, row)
		mu.Unlock()
		if row.Verdict == "FAIL" {
			t.Errorf("%s: %s", row.Case, row.Actual)
		}
	}
	t.Cleanup(func() {
		if err := writeContractResults(rows); err != nil {
			t.Errorf("writing contract results: %v", err)
		}
	})

	for _, tc := range doc.Cases {
		tc := tc
		t.Run(tc.ID, func(t *testing.T) {
			switch tc.Kind {
			case "response":
				add(runResponseCase(t, tc))
			case "request_period":
				add(runPeriodCase(t, tc))
			case "resource_id":
				add(runResourceCase(t, tc))
			case "official_request":
				for _, row := range runOfficialCase(t, tc) {
					add(row)
				}
			default:
				add(contractRow{Case: tc.ID, Expected: tc.Kind, Actual: "unknown kind", Verdict: "FAIL"})
			}
		})
	}
}

// TestCEContractLiveOptIn is the credentialed Cost Explorer check.
// It is skipped unless FINFOCUS_AWS_CE_LIVE=1 and must not be set in CI.
func TestCEContractLiveOptIn(t *testing.T) {
	if os.Getenv("FINFOCUS_AWS_CE_LIVE") != "1" {
		t.Skip("BLOCKED-ON-CREDENTIALS")
	}
	ctx, cancel := context.WithTimeout(context.Background(), 30*time.Second)
	defer cancel()
	ce, err := client.NewClient(ctx, client.Config{Region: "us-east-1"})
	if err != nil {
		t.Fatalf("BLOCKED-ON-CREDENTIALS: client: %v", err)
	}
	end := time.Now().UTC().Truncate(24 * time.Hour)
	start := end.Add(-48 * time.Hour)
	if _, err := ce.GetCost(ctx, nil, []string{"SERVICE"}, start, end, "DAILY"); err != nil {
		t.Fatalf("BLOCKED-ON-CREDENTIALS: live GetCost: %v", err)
	}
}

func runResponseCase(t *testing.T, tc contractCase) contractRow {
	t.Helper()
	f := newFakeCE(t, tc.Pages)
	start, end := recentWindow()
	resp, err := callActual(t, f, tc.ID, "", start, end)
	calls := f.snapshot()
	row := contractRow{Case: tc.ID, Expected: expectSummary(tc.Expect)}

	if tc.Expect.Outcome == "error" {
		return errorRow(row, tc, resp, err)
	}

	if err != nil {
		row.Actual = fmt.Sprintf("error %v; calls=%d", err, len(calls))
		row.Verdict = "FAIL"
		return row
	}
	return okRow(row, tc, resp, calls)
}

func runPeriodCase(t *testing.T, tc contractCase) contractRow {
	t.Helper()
	zoneProcess, _ := os.LookupEnv("FINFOCUS_CE_PERIOD_ZONE")
	if zoneProcess == "" {
		var notes []string
		for _, zone := range []string{"UTC", "America/Los_Angeles", "Pacific/Auckland"} {
			output := timezoneProcess(t, zone, "TestCEContractFixtures/"+tc.ID)
			found := false
			for _, line := range strings.Split(string(output), "\n") {
				if strings.HasPrefix(line, "CONTRACT_PERIOD_ROW=") {
					var row contractRow
					if err := json.Unmarshal([]byte(strings.TrimPrefix(line, "CONTRACT_PERIOD_ROW=")), &row); err != nil {
						t.Fatal(err)
					}
					if row.Verdict != "PASS" {
						t.Fatalf("child contract row: %#v", row)
					}
					notes = append(notes, row.Actual)
					found = true
				}
			}
			if !found {
				t.Fatalf("missing child period result: %s", output)
			}
		}
		return contractRow{Case: tc.ID, Expected: fmt.Sprintf("%s %s..%s", tc.Expect.Outcome, tc.Expect.Start, tc.Expect.End), Actual: strings.Join(notes, "; "), Verdict: "PASS"}
	}
	zones := []string{zoneProcess}
	var notes []string
	verdict := "PASS"
	for _, zone := range zones {
		var got string
		func() {
			_, err := time.LoadLocation(zone)
			if err != nil {
				notes = append(notes, zone+": "+err.Error())
				verdict = "FAIL"
				return
			}

			f := newFakeCE(t, []json.RawMessage{json.RawMessage(okPage)})
			start := time.Unix(tc.StartUnix, 0)
			end := time.Unix(tc.EndUnix, 0)
			resp, err := callActual(t, f, tc.ID, "", start, end)
			calls := f.snapshot()
			if tc.Expect.Outcome == "error" {
				if err == nil {
					got = "success"
					verdict = "FAIL"
					return
				}
				if status.Code(err) != codes.InvalidArgument {
					got = status.Code(err).String()
					verdict = "FAIL"
					return
				}
				if len(calls) != 0 {
					got = fmt.Sprintf("aws calls=%d", len(calls))
					verdict = "FAIL"
					return
				}
				if resp != nil && len(resp.GetResults()) > 0 {
					got = "returned cost rows"
					verdict = "FAIL"
					return
				}
				got = "InvalidArgument, calls=0"
				return
			}
			if err != nil {
				got = err.Error()
				verdict = "FAIL"
				return
			}
			if len(calls) != 1 {
				got = fmt.Sprintf("calls=%d", len(calls))
				verdict = "FAIL"
				return
			}
			period := timePeriodOf(calls[0].Body)
			if period["Start"] != tc.Expect.Start || period["End"] != tc.Expect.End {
				got = fmt.Sprintf("TimePeriod %s..%s", period["Start"], period["End"])
				verdict = "FAIL"
				return
			}
			if op := operationName(calls[0].Target); op != "GetCostAndUsage" {
				got = "operation " + op
				verdict = "FAIL"
				return
			}
			got = period["Start"] + ".." + period["End"]
		}()
		notes = append(notes, zone+"="+got)
	}
	row := contractRow{
		Case:     tc.ID,
		Expected: fmt.Sprintf("%s %s..%s", tc.Expect.Outcome, tc.Expect.Start, tc.Expect.End),
		Actual:   strings.Join(notes, "; "),
		Verdict:  verdict,
	}
	encoded, err := json.Marshal(row)
	if err != nil {
		t.Fatal(err)
	}
	fmt.Printf("CONTRACT_PERIOD_ROW=%s\n", encoded)
	return row
}

func runResourceCase(t *testing.T, tc contractCase) contractRow {
	t.Helper()
	row := contractRow{Case: tc.ID}
	f := newFakeCE(t, []json.RawMessage{json.RawMessage(okPage)})
	start, end := recentWindow()
	if tc.RequestAgeDays > 0 {
		end = time.Now().UTC().Add(-time.Duration(tc.RequestAgeDays) * 24 * time.Hour)
		start = end.Add(-24 * time.Hour)
		row.Expected = "error mentioning 14 days (" + tc.Expect.GRPCCode + ")"
	}
	resp, err := callActual(t, f, tc.ID, tc.Input, start, end)
	calls := f.snapshot()

	switch tc.ID {
	case "resource_id:ec2":
		row.Expected = "GetCostAndUsageWithResources RESOURCE_ID=" + tc.Expect.Value
		if err != nil {
			row.Actual = err.Error()
			row.Verdict = "FAIL"
			return row
		}
		if len(calls) == 0 {
			row.Actual = "no aws call"
			row.Verdict = "FAIL"
			return row
		}
		op := operationName(calls[0].Target)
		values := dimensionValues(calls[0].Body, "RESOURCE_ID")
		arnSent := strings.Contains(string(calls[0].Body), tc.Input)
		if op != tc.Expect.Operation || len(values) == 0 || values[0] != tc.Expect.Value || arnSent {
			row.Actual = fmt.Sprintf("op=%s values=%v arn_in_body=%v", op, values, arnSent)
			row.Verdict = "FAIL"
			return row
		}
		row.Actual = fmt.Sprintf("op=%s RESOURCE_ID=%s", op, values[0])
		row.Verdict = "PASS"
		return row
	case "resource_id:s3", "resource_id:rds":
		// Documented choice: non-EC2 resource ARNs have no verified Cost Explorer
		// RESOURCE_ID form. The plugin returns an explicit error and does not send
		// the bucket name, the db name, or the full ARN.
		row.Expected = "explicit error: resource-level cost is not available"
		if err == nil {
			row.Actual = fmt.Sprintf("success results=%d calls=%d", len(resp.GetResults()), len(calls))
			row.Verdict = "FAIL"
			return row
		}
		msg := err.Error()
		if !strings.Contains(msg, "resource-level cost is not available") || len(calls) != 0 || strings.Contains(joinedBodies(calls), tc.Input) {
			row.Actual = fmt.Sprintf("%s; calls=%d", msg, len(calls))
			row.Verdict = "FAIL"
			return row
		}
		row.Actual = "choice=explicit error, no id form sent; " + status.Code(err).String() + ": " + msg
		row.Verdict = "PASS"
		return row
	default:
		if tc.RequestAgeDays > 0 {
			if err == nil {
				row.Actual = "success"
				row.Verdict = "FAIL"
				return row
			}
			code := status.Code(err)
			msg := err.Error()
			if (code != codes.InvalidArgument && code != codes.OutOfRange) || !strings.Contains(msg, "14 days") || len(calls) != 0 {
				row.Actual = fmt.Sprintf("%s: %s; calls=%d", code, msg, len(calls))
				row.Verdict = "FAIL"
				return row
			}
			if resp != nil && len(resp.GetResults()) > 0 {
				row.Actual = "returned rows with the error"
				row.Verdict = "FAIL"
				return row
			}
			row.Actual = code.String() + ": " + msg
			row.Verdict = "PASS"
			return row
		}
	}
	row.Actual = "unhandled resource case"
	row.Verdict = "FAIL"
	return row
}

func runOfficialCase(t *testing.T, tc contractCase) []contractRow {
	t.Helper()
	f := newFakeCE(t, []json.RawMessage{json.RawMessage(okPage)})
	start, end := recentWindow()
	wantStart := start.UTC().Format("2006-01-02")
	wantEnd := end.UTC().Format("2006-01-02")
	arn := ""
	wantOp := "GetCostAndUsage"
	if strings.Contains(tc.ID, "gcuwr") {
		arn = "arn:aws:ec2:us-east-1:123456789012:instance/i-0abc123def4567890"
		wantOp = "GetCostAndUsageWithResources"
	}
	_, err := callActual(t, f, tc.ID, arn, start, end)
	calls := f.snapshot()
	rows := []contractRow{}
	main := contractRow{
		Case:     tc.ID,
		Expected: "operation " + wantOp + " and UTC exclusive TimePeriod " + wantStart + ".." + wantEnd,
	}
	if err != nil || len(calls) == 0 {
		main.Actual = fmt.Sprintf("err=%v calls=%d", err, len(calls))
		main.Verdict = "FAIL"
		return append(rows, main)
	}
	op := operationName(calls[0].Target)
	period := timePeriodOf(calls[0].Body)
	if op != wantOp || period["Start"] != wantStart || period["End"] != wantEnd {
		main.Actual = fmt.Sprintf("op=%s period=%s..%s", op, period["Start"], period["End"])
		main.Verdict = "FAIL"
	} else {
		main.Actual = fmt.Sprintf("op=%s period=%s..%s", op, period["Start"], period["End"])
		main.Verdict = "PASS"
	}
	rows = append(rows, main)

	// GetActualCostRequest cannot carry Granularity, GroupBy, Metrics, or a service Filter.
	// The 2017/2018 fixture TimePeriod is not sent. recentWindow stays inside the 14-day resource lookback.
	for _, field := range []string{"Granularity", "GroupBy", "Metrics", "Filter"} {
		raw, _ := json.Marshal(tc.Body[field])
		rows = append(rows, contractRow{
			Case:     tc.ID + "." + field,
			Expected: "proto field does not exist; fixture wants " + string(raw),
			Actual:   "FINDING: GetActualCostRequest cannot express " + field,
			Verdict:  "FINDING",
		})
	}
	return rows
}

func callActual(t *testing.T, f *fakeCE, id, arn string, start, end time.Time) (*pbc.GetActualCostResponse, error) {
	t.Helper()
	t.Setenv("AWS_EC2_METADATA_DISABLED", "true")
	t.Setenv("AWS_ACCESS_KEY_ID", "test")
	t.Setenv("AWS_SECRET_ACCESS_KEY", "test")
	t.Setenv("AWS_REGION", "us-east-1")

	ctx, cancel := context.WithTimeout(context.Background(), 20*time.Second)
	defer cancel()
	ce, err := client.NewClient(ctx, client.Config{
		Region:          "us-east-1",
		BaseEndpoint:    f.srv.URL,
		AccessKeyID:     "test",
		SecretAccessKey: "test",
	})
	if err != nil {
		t.Fatalf("new client: %v", err)
	}
	calc := NewCalculatorWithClient(ce)
	calc.cache = nil
	ts := pluginsdk.NewTestServer(t, calc)
	t.Cleanup(ts.Close)

	resourceID := "contract-" + id
	if arn != "" {
		if i := strings.LastIndex(arn, "/"); i >= 0 && strings.Contains(arn, ":instance/") {
			resourceID = arn[i+1:]
		}
	}
	return ts.Client().GetActualCost(ctx, &pbc.GetActualCostRequest{
		ResourceId:       resourceID,
		BillingAccountId: "contract-billing-account",
		Arn:              arn,
		Start:            timestamppb.New(start),
		End:              timestamppb.New(end),
	})
}

func okRow(row contractRow, tc contractCase, resp *pbc.GetActualCostResponse, calls []capturedCall) contractRow {
	if resp == nil {
		row.Actual = "nil response"
		row.Verdict = "FAIL"
		return row
	}
	got := map[string]*pbc.ActualCostResult{}
	var notes []string
	verdict := "PASS"
	for _, res := range resp.GetResults() {
		key := resultGroupKey(res)
		if key == "" {
			notes = append(notes, "missing group key")
			verdict = "FAIL"
			continue
		}
		if _, dup := got[key]; dup {
			notes = append(notes, "duplicate "+key)
			verdict = "FAIL"
		}
		got[key] = res
	}
	if len(got) != len(tc.Expect.PerService) {
		notes = append(notes, fmt.Sprintf("groups=%d want %d", len(got), len(tc.Expect.PerService)))
		verdict = "FAIL"
	}

	sumGot := new(big.Rat)
	for key, want := range tc.Expect.PerService {
		res := got[key]
		if res == nil {
			notes = append(notes, "missing "+key)
			verdict = "FAIL"
			continue
		}
		gotText := resultAmountDecimal(res)
		gotRat, gok := new(big.Rat).SetString(gotText)
		wantRat, wok := new(big.Rat).SetString(want)
		if !gok || !wok || gotRat.Cmp(wantRat) != 0 {
			notes = append(notes, fmt.Sprintf("%s amount_decimal %q != %s", key, gotText, want))
			verdict = "FAIL"
		} else {
			sumGot.Add(sumGot, gotRat)
			wire, _ := gotRat.Float64()
			if math.Float64bits(res.GetCost()) != math.Float64bits(wire) {
				notes = append(notes, fmt.Sprintf("%s cost bits %#x != %#x", key, math.Float64bits(res.GetCost()), math.Float64bits(wire)))
				verdict = "FAIL"
			}
		}
		cur := resultCurrency(res)
		if cur != tc.Expect.Currency {
			notes = append(notes, fmt.Sprintf("%s currency %q", key, cur))
			verdict = "FAIL"
		}
		if res.GetUsageUnit() == tc.Expect.Currency && tc.Expect.Currency != "" {
			notes = append(notes, key+" usage unit is the currency")
			verdict = "FAIL"
		}
	}

	if tc.Expect.Total != "" {
		want, ok := new(big.Rat).SetString(tc.Expect.Total)
		if !ok || sumGot.Cmp(want) != 0 {
			notes = append(notes, fmt.Sprintf("total got %s want %s", sumGot.FloatString(10), tc.Expect.Total))
			verdict = "FAIL"
		}
	}

	if tc.Expect.Estimated != nil {
		est := false
		for _, res := range got {
			if resultEstimated(res) {
				est = true
			}
		}
		if est != *tc.Expect.Estimated {
			notes = append(notes, fmt.Sprintf("estimated=%v", est))
			verdict = "FAIL"
		}
	}
	if tc.Expect.PageCount != nil && len(calls) != *tc.Expect.PageCount {
		notes = append(notes, fmt.Sprintf("page_count=%d", len(calls)))
		verdict = "FAIL"
	}
	if tc.Expect.PageCount != nil && *tc.Expect.PageCount > 1 {
		tokens := pageTokens(tc.Pages)
		for i := 1; i < len(calls) && i-1 < len(tokens); i++ {
			if !strings.Contains(string(calls[i].Body), tokens[i-1]) {
				notes = append(notes, fmt.Sprintf("call %d missing token %s", i+1, tokens[i-1]))
				verdict = "FAIL"
			}
		}
	}
	if usageNote, bad := usageMatches(tc, got); bad {
		notes = append(notes, usageNote)
		verdict = "FAIL"
	} else if usageNote != "" {
		notes = append(notes, usageNote)
	}
	if len(notes) == 0 {
		notes = append(notes, "total "+tc.Expect.Total+" currency "+tc.Expect.Currency)
	}
	row.Actual = strings.Join(notes, "; ")
	row.Verdict = verdict
	return row
}

func errorRow(row contractRow, tc contractCase, resp *pbc.GetActualCostResponse, err error) contractRow {
	if err == nil {
		row.Actual = fmt.Sprintf("success with %d rows", len(resp.GetResults()))
		row.Verdict = "FAIL"
		return row
	}
	if resp != nil && len(resp.GetResults()) > 0 {
		row.Actual = "error plus cost rows"
		row.Verdict = "FAIL"
		return row
	}
	code := status.Code(err)
	switch {
	case strings.Contains(tc.Expect.GRPCCode, "NotFound"):
		if code != codes.NotFound {
			row.Actual = code.String() + ": " + err.Error()
			row.Verdict = "FAIL"
			return row
		}
	case strings.Contains(tc.Expect.GRPCCode, "FailedPrecondition"):
		if code != codes.FailedPrecondition {
			row.Actual = code.String() + ": " + err.Error()
			row.Verdict = "FAIL"
			return row
		}
	case strings.Contains(tc.Expect.GRPCCode, "Internal"):
		if code != codes.Internal && code != codes.DataLoss {
			row.Actual = code.String() + ": " + err.Error()
			row.Verdict = "FAIL"
			return row
		}
	case strings.Contains(tc.Expect.GRPCCode, "InvalidArgument"):
		if code != codes.InvalidArgument {
			row.Actual = code.String() + ": " + err.Error()
			row.Verdict = "FAIL"
			return row
		}
	}
	row.Actual = code.String() + ": " + err.Error()
	row.Verdict = "PASS"
	return row
}

func usageMatches(tc contractCase, got map[string]*pbc.ActualCostResult) (string, bool) {
	type agg struct {
		sum     *big.Rat
		unit    string
		present bool
	}
	want := map[string]*agg{}
	for _, page := range tc.Pages {
		var body struct {
			ResultsByTime []struct {
				Groups []struct {
					Keys    []string
					Metrics map[string]struct {
						Amount *string
						Unit   *string
					}
				}
			}
		}
		if err := json.Unmarshal(page, &body); err != nil {
			return "usage fixture: " + err.Error(), true
		}
		for _, period := range body.ResultsByTime {
			for _, g := range period.Groups {
				if len(g.Keys) == 0 {
					continue
				}
				key := g.Keys[0]
				a := want[key]
				if a == nil {
					a = &agg{}
					want[key] = a
				}
				u := g.Metrics["UsageQuantity"]
				if u.Amount == nil {
					continue
				}
				r, ok := new(big.Rat).SetString(*u.Amount)
				if !ok {
					continue
				}
				if a.sum == nil {
					a.sum = r
				} else {
					a.sum.Add(a.sum, r)
				}
				a.present = true
				if u.Unit != nil {
					a.unit = *u.Unit
				}
			}
		}
	}
	for key, res := range got {
		a := want[key]
		if a == nil || !a.present {
			if res.GetUsageAmount() != 0 || res.GetUsageUnit() != "" {
				return fmt.Sprintf("%s usage %v %q, want 0 and empty", key, res.GetUsageAmount(), res.GetUsageUnit()), true
			}
			continue
		}
		f, _ := a.sum.Float64()
		if res.GetUsageAmount() != f || res.GetUsageUnit() != a.unit {
			return fmt.Sprintf("%s usage %v %q, want %v %q", key, res.GetUsageAmount(), res.GetUsageUnit(), f, a.unit), true
		}
	}
	return "", false
}

func resultAmountDecimal(res *pbc.ActualCostResult) string {
	fr := res.GetFocusRecord()
	if fr == nil {
		return ""
	}
	return fr.GetExtendedColumns()[contractAmountColumn]
}

func resultGroupKey(res *pbc.ActualCostResult) string {
	fr := res.GetFocusRecord()
	if fr == nil {
		return ""
	}
	if fr.GetServiceName() != "" {
		return fr.GetServiceName()
	}
	return fr.GetExtendedColumns()[contractGroupKeyColumn]
}

func resultCurrency(res *pbc.ActualCostResult) string {
	fr := res.GetFocusRecord()
	if fr == nil {
		return ""
	}
	if fr.GetBillingCurrency() != "" {
		return fr.GetBillingCurrency()
	}
	return fr.GetExtendedColumns()[contractCurrencyColumn]
}

func resultEstimated(res *pbc.ActualCostResult) bool {
	fr := res.GetFocusRecord()
	if fr == nil {
		return false
	}
	return fr.GetExtendedColumns()[contractEstimatedColumn] == "true"
}

func recentWindow() (time.Time, time.Time) {
	end := time.Now().UTC().Truncate(24 * time.Hour)
	start := end.Add(-48 * time.Hour)
	return start, end
}

func operationName(target string) string {
	if strings.Contains(target, "GetCostAndUsageWithResources") {
		return "GetCostAndUsageWithResources"
	}
	if strings.Contains(target, "GetCostAndUsage") {
		return "GetCostAndUsage"
	}
	return target
}

func timePeriodOf(body []byte) map[string]string {
	var m map[string]any
	if err := json.Unmarshal(body, &m); err != nil {
		return map[string]string{}
	}
	raw, _ := m["TimePeriod"].(map[string]any)
	out := map[string]string{}
	for _, k := range []string{"Start", "End"} {
		if s, ok := raw[k].(string); ok {
			out[k] = s
		}
	}
	return out
}

func dimensionValues(body []byte, key string) []string {
	var m any
	if err := json.Unmarshal(body, &m); err != nil {
		return nil
	}
	var out []string
	var walk func(any)
	walk = func(n any) {
		switch t := n.(type) {
		case map[string]any:
			if ks, _ := t["Key"].(string); ks == key {
				if vals, ok := t["Values"].([]any); ok {
					for _, v := range vals {
						if s, ok := v.(string); ok {
							out = append(out, s)
						}
					}
				}
			}
			for _, child := range t {
				walk(child)
			}
		case []any:
			for _, child := range t {
				walk(child)
			}
		}
	}
	walk(m)
	return out
}

func pageTokens(pages []json.RawMessage) []string {
	var tokens []string
	for _, page := range pages {
		var m struct {
			NextPageToken string `json:"NextPageToken"`
		}
		_ = json.Unmarshal(page, &m)
		if m.NextPageToken != "" {
			tokens = append(tokens, m.NextPageToken)
		}
	}
	return tokens
}

func joinedBodies(calls []capturedCall) string {
	var b strings.Builder
	for _, c := range calls {
		b.Write(c.Body)
	}
	return b.String()
}

func expectSummary(e contractExpect) string {
	if e.Outcome == "error" {
		return e.Outcome + " " + e.GRPCCode
	}
	est := ""
	if e.Estimated != nil {
		est = fmt.Sprintf(" estimated=%v", *e.Estimated)
	}
	pages := ""
	if e.PageCount != nil {
		pages = fmt.Sprintf(" pages=%d", *e.PageCount)
	}
	return fmt.Sprintf("ok total=%s %s%s%s", e.Total, e.Currency, est, pages)
}

func loadContract(t *testing.T) contractDoc {
	t.Helper()
	_, file, _, ok := runtime.Caller(0)
	if !ok {
		t.Fatal("runtime.Caller failed")
	}
	path := filepath.Join(filepath.Dir(file), "..", "client", "testdata", "ce-contract", "ce-contract.json")
	data, err := os.ReadFile(path)
	if err != nil {
		t.Fatalf("read contract: %v", err)
	}
	var doc contractDoc
	if err := json.Unmarshal(data, &doc); err != nil {
		t.Fatalf("parse contract: %v", err)
	}
	return doc
}

func writeContractResults(rows []contractRow) error {
	_, file, _, ok := runtime.Caller(0)
	if !ok {
		return fmt.Errorf("runtime.Caller failed")
	}
	path := filepath.Join(filepath.Dir(file), "..", "..", ".superpowers", "contract-results.md")
	if err := os.MkdirAll(filepath.Dir(path), 0o755); err != nil {
		return err
	}
	var b strings.Builder
	b.WriteString("# Cost Explorer contract results\n\n")
	b.WriteString("| Case | Expected | Actual | Verdict |\n")
	b.WriteString("| --- | --- | --- | --- |\n")
	for _, row := range rows {
		fmt.Fprintf(&b, "| %s | %s | %s | %s |\n", mdCell(row.Case), mdCell(row.Expected), mdCell(row.Actual), mdCell(row.Verdict))
	}
	b.WriteString("\n")
	b.WriteString("ActualCostResult.Cost is float64. The exact decimal is extended_columns[\"amount_decimal\"].\n")
	return os.WriteFile(path, []byte(b.String()), 0o644)
}

func mdCell(s string) string {
	s = strings.ReplaceAll(s, "\n", " ")
	s = strings.ReplaceAll(s, "|", "\\|")
	return s
}
