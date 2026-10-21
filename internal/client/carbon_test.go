package client

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"github.com/aws/aws-sdk-go-v2/aws"
	"github.com/aws/aws-sdk-go-v2/service/sustainability"
	st "github.com/aws/aws-sdk-go-v2/service/sustainability/types"
	"github.com/aws/smithy-go"
	"github.com/rshade/finfocus-spec/sdk/go/pluginsdk"
	"io"
	"math"
	"net/http"
	"net/http/httptest"
	"os"
	"strings"
	"testing"
	"time"

	pbc "github.com/rshade/finfocus-spec/sdk/go/proto/finfocus/v1"
	"google.golang.org/protobuf/types/known/timestamppb"
)

func carbonRequest() *pbc.GetCarbonEmissionsRequest {
	return &pbc.GetCarbonEmissionsRequest{Start: timestamppb.New(time.Date(2026, 1, 1, 0, 0, 0, 0, time.UTC)), End: timestamppb.New(time.Date(2026, 2, 1, 0, 0, 0, 0, time.UTC)), AccountIds: []string{"123456789012"}, Regions: []string{"eu-west-1"}, Services: []string{"AmazonEC2"}}
}

func TestCarbonOfficialSDKHTTP(t *testing.T) {
	fixture, err := os.ReadFile("testdata/carbon/monthly.json")
	if err != nil {
		t.Fatal(err)
	}
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.Method != "POST" {
			t.Errorf("method %s", r.Method)
		}
		if !strings.Contains(r.Header.Get("Authorization"), "fixture-key/") || r.Header.Get("X-Amz-Security-Token") != "fixture-session" {
			t.Errorf("unexpected signing credentials")
		}
		var body map[string]any
		if err := json.NewDecoder(r.Body).Decode(&body); err != nil {
			t.Error(err)
		}
		expected := `{"TimePeriod":{"Start":"2026-01-01T00:00:00Z","End":"2026-02-01T00:00:00Z"},"Granularity":"MONTHLY","GroupBy":["USAGE_ACCOUNT_ID","REGION","SERVICE"],"EmissionsTypes":["TOTAL_MBM_CARBON_EMISSIONS"],"FilterBy":{"Dimensions":{"USAGE_ACCOUNT_ID":["123456789012"],"REGION":["eu-west-1"],"SERVICE":["AmazonEC2"]}},"MaxResults":50,"NextToken":"prior"}`
		var want map[string]any
		_ = json.Unmarshal([]byte(expected), &want)
		gotJSON, _ := json.Marshal(body)
		wantJSON, _ := json.Marshal(want)
		if string(gotJSON) != string(wantJSON) {
			t.Errorf("request got %s want %s", gotJSON, wantJSON)
		}
		w.Header().Set("Content-Type", "application/json")
		_, _ = w.Write(fixture)
	}))
	defer server.Close()
	c, err := NewSustainabilityClient(context.Background(), Config{Region: "eu-west-1", BaseEndpoint: server.URL, AccessKeyID: "fixture-key", SecretAccessKey: "fixture-secret", SessionToken: "fixture-session"})
	if err != nil {
		t.Fatal(err)
	}
	req := carbonRequest()
	page, err := c.ReadCarbonPage(context.Background(), req, "prior")
	if err != nil {
		t.Fatal(err)
	}
	if len(page.Records) != 1 || page.Records[0].GetValue() != 1250000 || page.NextToken != "aws-next" {
		t.Fatalf("bad page: %+v", page)
	}
	if req.Method != 0 || req.PageSize != 0 {
		t.Fatal("mutated caller request")
	}
}

// carbonFixtureAPI retains official SDK types while exercising malformed Go
// values that JSON cannot encode, including nonfinite quantities.
type carbonFixtureAPI struct {
	output *sustainability.GetEstimatedCarbonEmissionsOutput
	err    error
	input  *sustainability.GetEstimatedCarbonEmissionsInput
	calls  int
}

func (f *carbonFixtureAPI) GetEstimatedCarbonEmissions(_ context.Context, input *sustainability.GetEstimatedCarbonEmissionsInput, _ ...func(*sustainability.Options)) (*sustainability.GetEstimatedCarbonEmissionsOutput, error) {
	f.input = input
	f.calls++
	return f.output, f.err
}

func carbonRow() st.EstimatedCarbonEmissions {
	q := carbonRequest()
	return st.EstimatedCarbonEmissions{DimensionsValues: map[string]string{"USAGE_ACCOUNT_ID": "123456789012", "REGION": "eu-west-1", "SERVICE": "AmazonEC2"}, EmissionsValues: map[string]st.Emissions{"TOTAL_MBM_CARBON_EMISSIONS": {Value: aws.Float64(1.25), Unit: st.EmissionsUnitMtCo2e}}, ModelVersion: aws.String("2.0.0"), TimePeriod: &st.TimePeriod{Start: aws.Time(q.Start.AsTime()), End: aws.Time(q.End.AsTime())}}
}
func TestCarbonMethodScopeSelection(t *testing.T) {
	want := [][]st.EmissionsType{
		{st.EmissionsTypeTotalMbmCarbonEmissions, st.EmissionsTypeTotalScope1CarbonEmissions, st.EmissionsTypeTotalScope2MbmCarbonEmissions, st.EmissionsTypeTotalScope3MbmCarbonEmissions},
		{st.EmissionsTypeTotalLbmCarbonEmissions, st.EmissionsTypeTotalScope1CarbonEmissions, st.EmissionsTypeTotalScope2LbmCarbonEmissions, st.EmissionsTypeTotalScope3LbmCarbonEmissions},
	}
	for method := 1; method <= 2; method++ {
		for scope := 1; scope <= 4; scope++ {
			t.Run(fmt.Sprintf("%d/%d", method, scope), func(t *testing.T) {
				q := carbonRequest()
				q.Method = pbc.CarbonAccountingMethod(method)
				q.Scope = pbc.CarbonEmissionsScope(scope)
				row := carbonRow()
				row.EmissionsValues = map[string]st.Emissions{string(want[method-1][scope-1]): {Value: aws.Float64(0), Unit: st.EmissionsUnitMtCo2e}}
				api := &carbonFixtureAPI{output: &sustainability.GetEstimatedCarbonEmissionsOutput{Results: []st.EstimatedCarbonEmissions{row}}}
				page, err := NewSustainabilityClientWithAPI(api).ReadCarbonPage(context.Background(), q, "")
				if err != nil {
					t.Fatal(err)
				}
				if len(api.input.EmissionsTypes) != 1 || api.input.EmissionsTypes[0] != want[method-1][scope-1] {
					t.Fatal(api.input.EmissionsTypes)
				}
				record := page.Records[0]
				if record.Value == nil || record.GetValue() != 0 || record.Method != q.Method || record.Scope != q.Scope || record.ModelVersion != "2.0.0" {
					t.Fatal(record)
				}
			})
		}
	}
}
func TestCarbonRejectMalformedRows(t *testing.T) {
	tests := map[string]func(*st.EstimatedCarbonEmissions){
		"nil quantity": func(r *st.EstimatedCarbonEmissions) {
			r.EmissionsValues["TOTAL_MBM_CARBON_EMISSIONS"] = st.Emissions{Unit: st.EmissionsUnitMtCo2e}
		},
		"missing selected type": func(r *st.EstimatedCarbonEmissions) { r.EmissionsValues = nil },
		"unknown unit": func(r *st.EstimatedCarbonEmissions) {
			r.EmissionsValues["TOTAL_MBM_CARBON_EMISSIONS"] = st.Emissions{Value: aws.Float64(1), Unit: "kg"}
		},
		"negative": func(r *st.EstimatedCarbonEmissions) {
			r.EmissionsValues["TOTAL_MBM_CARBON_EMISSIONS"] = st.Emissions{Value: aws.Float64(-1), Unit: st.EmissionsUnitMtCo2e}
		},
		"nan": func(r *st.EstimatedCarbonEmissions) {
			r.EmissionsValues["TOTAL_MBM_CARBON_EMISSIONS"] = st.Emissions{Value: aws.Float64(math.NaN()), Unit: st.EmissionsUnitMtCo2e}
		},
		"infinite": func(r *st.EstimatedCarbonEmissions) {
			r.EmissionsValues["TOTAL_MBM_CARBON_EMISSIONS"] = st.Emissions{Value: aws.Float64(math.Inf(1)), Unit: st.EmissionsUnitMtCo2e}
		},
		"overflow": func(r *st.EstimatedCarbonEmissions) {
			r.EmissionsValues["TOTAL_MBM_CARBON_EMISSIONS"] = st.Emissions{Value: aws.Float64(math.MaxFloat64), Unit: st.EmissionsUnitMtCo2e}
		},
		"period":        func(r *st.EstimatedCarbonEmissions) { r.TimePeriod = nil },
		"missing start": func(r *st.EstimatedCarbonEmissions) { r.TimePeriod.Start = nil },
		"nonmonthly":    func(r *st.EstimatedCarbonEmissions) { r.TimePeriod.End = aws.Time(r.TimePeriod.End.AddDate(0, 1, 0)) },
		"out of query": func(r *st.EstimatedCarbonEmissions) {
			r.TimePeriod.Start = aws.Time(r.TimePeriod.Start.AddDate(0, -1, 0))
			r.TimePeriod.End = aws.Time(r.TimePeriod.End.AddDate(0, -1, 0))
		},
		"midmonth": func(r *st.EstimatedCarbonEmissions) {
			r.TimePeriod.Start = aws.Time(r.TimePeriod.Start.AddDate(0, 0, 1))
		},
		"model":           func(r *st.EstimatedCarbonEmissions) { r.ModelVersion = nil },
		"dimensions":      func(r *st.EstimatedCarbonEmissions) { delete(r.DimensionsValues, "REGION") },
		"filter mismatch": func(r *st.EstimatedCarbonEmissions) { r.DimensionsValues["SERVICE"] = "AmazonS3" },
	}
	for name, mutate := range tests {
		t.Run(name, func(t *testing.T) {
			row := carbonRow()
			mutate(&row)
			api := &carbonFixtureAPI{output: &sustainability.GetEstimatedCarbonEmissionsOutput{Results: []st.EstimatedCarbonEmissions{row}}}
			page, err := NewSustainabilityClientWithAPI(api).ReadCarbonPage(context.Background(), carbonRequest(), "")
			if !errors.Is(err, ErrCarbonDataInvalid) || len(page.Records) > 0 {
				t.Fatalf("page %+v err %v", page, err)
			}
		})
	}
}
func TestCarbonIdentityModelRevision(t *testing.T) {
	q := carbonRequest()
	pluginsdk.ApplyGetCarbonEmissionsDefaults(q)
	row := carbonRow()
	a, err := normalizeCarbon(row, q, timestamppb.Now())
	if err != nil {
		t.Fatal(err)
	}
	row.ModelVersion = aws.String("3.0.0")
	b, err := normalizeCarbon(row, q, timestamppb.Now())
	if err != nil {
		t.Fatal(err)
	}
	if a.Id != b.Id {
		t.Fatal("model revision changed additive identity")
	}
}
func TestCarbonEmptyDeniedCancellationAndBudget(t *testing.T) {
	t.Run("empty report", func(t *testing.T) {
		api := &carbonFixtureAPI{output: &sustainability.GetEstimatedCarbonEmissionsOutput{}}
		page, err := NewSustainabilityClientWithAPI(api).ReadCarbonPage(context.Background(), carbonRequest(), "")
		if err != nil || len(page.Records) != 0 {
			t.Fatalf("%+v %v", page, err)
		}
	})
	t.Run("nil response", func(t *testing.T) {
		_, err := NewSustainabilityClientWithAPI(&carbonFixtureAPI{}).ReadCarbonPage(context.Background(), carbonRequest(), "")
		if !errors.Is(err, ErrCarbonResponseEmpty) {
			t.Fatal(err)
		}
	})
	t.Run("denied", func(t *testing.T) {
		api := &carbonFixtureAPI{err: &smithy.GenericAPIError{Code: "AccessDeniedException", Message: "SECRET"}}
		_, err := NewSustainabilityClientWithAPI(api).ReadCarbonPage(context.Background(), carbonRequest(), "")
		if err == nil || strings.Contains(err.Error(), "SECRET") || api.calls != 1 {
			t.Fatal(err)
		}
		var e smithy.APIError
		if !errors.As(err, &e) || e.ErrorCode() != "AccessDeniedException" {
			t.Fatal(err)
		}
	})
	t.Run("cancelled", func(t *testing.T) {
		ctx, cancel := context.WithCancel(context.Background())
		cancel()
		api := &carbonFixtureAPI{}
		_, err := NewSustainabilityClientWithAPI(api).ReadCarbonPage(ctx, carbonRequest(), "")
		if !errors.Is(err, context.Canceled) || api.calls != 0 {
			t.Fatal(err)
		}
	})
	t.Run("retry budget", func(t *testing.T) {
		api := &carbonFixtureAPI{err: &smithy.GenericAPIError{Code: "ThrottlingException", Message: "SECRET"}}
		budget := errors.New("attempt budget exhausted")
		attempts := 0
		ctx := WithPageHook(context.Background(), func() error {
			attempts++
			if attempts > 2 {
				return budget
			}
			return nil
		})
		_, err := NewSustainabilityClientWithAPI(api).ReadCarbonPage(ctx, carbonRequest(), "")
		if !errors.Is(err, budget) || api.calls != 2 {
			t.Fatalf("calls %d err %v", api.calls, err)
		}
	})
	t.Run("invalid request", func(t *testing.T) {
		api := &carbonFixtureAPI{}
		_, err := NewSustainabilityClientWithAPI(api).ReadCarbonPage(context.Background(), nil, "")
		if err == nil || api.calls != 0 {
			t.Fatal(err)
		}
	})
}

type carbonHTTPFunc func(*http.Request) (*http.Response, error)

func (f carbonHTTPFunc) Do(r *http.Request) (*http.Response, error) { return f(r) }

func TestCarbonEndpointAndDefaultCredentialChain(t *testing.T) {
	t.Setenv("AWS_ACCESS_KEY_ID", "chain-key")
	t.Setenv("AWS_SECRET_ACCESS_KEY", "chain-secret")
	t.Setenv("AWS_SESSION_TOKEN", "chain-session")
	t.Setenv("AWS_REGION", "ap-southeast-2")
	transport := carbonHTTPFunc(func(r *http.Request) (*http.Response, error) {
		if !strings.Contains(r.URL.Host, "us-east-1") {
			t.Errorf("endpoint host %s", r.URL.Host)
		}
		if !strings.Contains(r.Header.Get("Authorization"), "chain-key/") || r.Header.Get("X-Amz-Security-Token") != "chain-session" {
			t.Error("default credential chain was not used")
		}
		return &http.Response{StatusCode: 200, Header: http.Header{"Content-Type": []string{"application/json"}}, Body: io.NopCloser(strings.NewReader(`{"Results":[]}`))}, nil
	})
	c, err := NewSustainabilityClient(context.Background(), Config{httpClient: transport})
	if err != nil {
		t.Fatal(err)
	}
	_, err = c.ReadCarbonPage(context.Background(), carbonRequest(), "")
	if err != nil {
		t.Fatal(err)
	}
}
func TestCarbonAssumeRoleHTTP(t *testing.T) {
	stsCalls, carbonCalls := 0, 0
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if strings.Contains(r.Header.Get("Content-Type"), "application/x-www-form-urlencoded") {
			stsCalls++
			if err := r.ParseForm(); err != nil {
				t.Error(err)
			}
			if r.Form.Get("Action") != "AssumeRole" || r.Form.Get("RoleArn") != "arn:aws:iam::123456789012:role/carbon" || !strings.Contains(r.Header.Get("Authorization"), "base-key/") {
				t.Error("unexpected assume role input or base credentials")
			}
			w.Header().Set("Content-Type", "text/xml")
			_, _ = io.WriteString(w, `<AssumeRoleResponse xmlns="https://sts.amazonaws.com/doc/2011-06-15/"><AssumeRoleResult><Credentials><AccessKeyId>assumed-key</AccessKeyId><SecretAccessKey>assumed-secret</SecretAccessKey><SessionToken>assumed-token</SessionToken><Expiration>2030-01-01T00:00:00Z</Expiration></Credentials><AssumedRoleUser><Arn>arn:aws:sts::123456789012:assumed-role/carbon/test</Arn><AssumedRoleId>test</AssumedRoleId></AssumedRoleUser></AssumeRoleResult></AssumeRoleResponse>`)
			return
		}
		carbonCalls++
		if !strings.Contains(r.Header.Get("Authorization"), "assumed-key/") || r.Header.Get("X-Amz-Security-Token") != "assumed-token" {
			t.Error("assumed credentials missing")
		}
		w.Header().Set("Content-Type", "application/json")
		_, _ = io.WriteString(w, `{"Results":[]}`)
	}))
	defer server.Close()
	c, err := NewSustainabilityClient(context.Background(), Config{BaseEndpoint: server.URL, AccessKeyID: "base-key", SecretAccessKey: "base-secret", RoleARN: "arn:aws:iam::123456789012:role/carbon"})
	if err != nil {
		t.Fatal(err)
	}
	_, err = c.ReadCarbonPage(context.Background(), carbonRequest(), "")
	if err != nil {
		t.Fatal(err)
	}
	if stsCalls != 1 || carbonCalls != 1 {
		t.Fatalf("STS %d carbon %d", stsCalls, carbonCalls)
	}
}
func TestCarbonHTTPRetriesAllBudgeted(t *testing.T) {
	t.Setenv("AWS_MAX_ATTEMPTS", "10")
	calls, hooks := 0, 0
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		calls++
		w.Header().Set("Content-Type", "application/json")
		if calls < 3 {
			w.WriteHeader(500)
			_, _ = io.WriteString(w, `{"__type":"InternalServerException","message":"PRIVATE"}`)
			return
		}
		_, _ = io.WriteString(w, `{"Results":[]}`)
	}))
	defer server.Close()
	c, err := NewSustainabilityClient(context.Background(), Config{BaseEndpoint: server.URL, AccessKeyID: "test-key", SecretAccessKey: "test-secret"})
	if err != nil {
		t.Fatal(err)
	}
	ctx := WithPageHook(context.Background(), func() error { hooks++; return nil })
	_, err = c.ReadCarbonPage(ctx, carbonRequest(), "")
	if err != nil {
		t.Fatal(err)
	}
	if calls != 3 || hooks != 3 {
		t.Fatalf("HTTP %d hooks %d", calls, hooks)
	}
}

func TestCarbonBoundedRetriesAndBackoffCancellation(t *testing.T) {
	t.Run("bounded throttle", func(t *testing.T) {
		api := &carbonFixtureAPI{err: &smithy.GenericAPIError{Code: "ThrottlingException", Message: "PRIVATE"}}
		hooks := 0
		ctx := WithPageHook(context.Background(), func() error { hooks++; return nil })
		_, err := NewSustainabilityClientWithAPI(api).ReadCarbonPage(ctx, carbonRequest(), "")
		if err == nil || strings.Contains(err.Error(), "PRIVATE") || api.calls != 6 || hooks != 6 {
			t.Fatalf("attempts %d hooks %d error %v", api.calls, hooks, err)
		}
	})
	t.Run("cancel backoff", func(t *testing.T) {
		ctx, cancel := context.WithCancel(context.Background())
		defer cancel()
		calls := 0
		server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
			calls++
			w.Header().Set("Content-Type", "application/json")
			w.WriteHeader(429)
			_, _ = io.WriteString(w, `{"__type":"ThrottlingException","message":"PRIVATE"}`)
			cancel()
		}))
		defer server.Close()
		c, err := NewSustainabilityClient(context.Background(), Config{BaseEndpoint: server.URL, AccessKeyID: "fixture", SecretAccessKey: "fixture"})
		if err != nil {
			t.Fatal(err)
		}
		_, err = c.ReadCarbonPage(ctx, carbonRequest(), "")
		if !errors.Is(err, context.Canceled) || calls != 1 {
			t.Fatalf("calls %d error %v", calls, err)
		}
	})
}

func TestCarbonCredentialErrorsPreserveSafeCodes(t *testing.T) {
	for _, code := range []string{"ExpiredToken", "ExpiredTokenException", "InvalidClientTokenId", "UnrecognizedClientException", "AuthFailure"} {
		t.Run(code, func(t *testing.T) {
			private := "session-token-secret arn:aws:iam::123456789012:role/private"
			api := &carbonFixtureAPI{err: &smithy.GenericAPIError{Code: code, Message: private}}
			_, err := NewSustainabilityClientWithAPI(api).ReadCarbonPage(context.Background(), carbonRequest(), "")
			var awsErr smithy.APIError
			if !errors.As(err, &awsErr) || awsErr.ErrorCode() != code {
				t.Fatalf("credential code got %v want %s", err, code)
			}
			if awsErr.ErrorMessage() != "AWS sustainability request failed" || strings.Contains(err.Error(), private) || strings.Contains(err.Error(), "session-token-secret") || strings.Contains(err.Error(), "arn:aws:") {
				t.Fatalf("unsafe diagnostic: %v", err)
			}
			if api.calls != 1 {
				t.Fatalf("credential failure retried %d times", api.calls)
			}
		})
	}
}
