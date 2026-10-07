package client

import (
	"context"
	"encoding/json"
	"errors"
	"github.com/aws/aws-sdk-go-v2/service/costexplorer/types"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
	"time"
)

func TestAttributedCostPartitionsAndAmortizes(t *testing.T) {
	var discovery, queries, hooks int
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "application/x-amz-json-1.1")
		var request struct {
			Dimension     string
			NextPageToken *string
			GroupBy       []struct{ Key string }
			Metrics       []string
			Filter        json.RawMessage
		}
		if err := json.NewDecoder(r.Body).Decode(&request); err != nil {
			t.Error(err)
			return
		}
		if strings.HasSuffix(r.Header.Get("X-Amz-Target"), ".GetDimensionValues") {
			discovery++
			if request.Dimension == "RESERVATION_ID" {
				if request.NextPageToken == nil {
					_, _ = w.Write([]byte(`{"DimensionValues":[{"Value":"ri-1"}],"NextPageToken":"page2"}`))
				} else {
					_, _ = w.Write([]byte(`{"DimensionValues":[{"Value":"ri-1"},{"Value":""}]}`))
				}
			} else {
				_, _ = w.Write([]byte(`{"DimensionValues":[{"Value":"sp-1"}]}`))
			}
			return
		}
		queries++
		if len(request.GroupBy) != 1 || request.GroupBy[0].Key != "SERVICE" {
			t.Errorf("unsupported grouping: %+v", request.GroupBy)
		}
		filter := string(request.Filter)
		if !strings.Contains(filter, "LINKED_ACCOUNT") {
			t.Errorf("caller filter lost: %s", filter)
		}
		amount := "1"
		amortized := ""
		switch queries {
		case 1:
			if !strings.Contains(filter, "ri-1") || strings.Contains(filter, "\"Not\"") {
				t.Errorf("RI filter: %s", filter)
			}
			amortized = `,"AmortizedCost":{"Amount":"2.125","Unit":"USD"}`
		case 2:
			if !strings.Contains(filter, "sp-1") || !strings.Contains(filter, "\"Not\"") || !strings.Contains(filter, "ri-1") {
				t.Errorf("SP overlap exclusion: %s", filter)
			}
			amortized = `,"AmortizedCost":{"Amount":"3.25","Unit":"USD"}`
		case 3:
			if !strings.Contains(filter, "\"Not\"") || !strings.Contains(filter, "ri-1") || !strings.Contains(filter, "sp-1") {
				t.Errorf("residual filter: %s", filter)
			}
		default:
			t.Errorf("unexpected query %d", queries)
		}
		_, _ = w.Write([]byte(`{"ResultsByTime":[{"TimePeriod":{"Start":"2026-10-01","End":"2026-10-02"},"Groups":[{"Keys":["S3"],"Metrics":{"UnblendedCost":{"Amount":"` + amount + `","Unit":"USD"}` + amortized + `}}]}]}`))
	}))
	defer server.Close()
	ce, err := NewClient(context.Background(), Config{Region: "us-east-1", BaseEndpoint: server.URL, AccessKeyID: "test", SecretAccessKey: "test"})
	if err != nil {
		t.Fatal(err)
	}
	ctx := WithPageHook(context.Background(), func() error { hooks++; return nil })
	start := time.Date(2026, 10, 1, 0, 0, 0, 0, time.UTC)
	rows, err := ce.GetAttributedCost(ctx, &types.Expression{Dimensions: &types.DimensionValues{Key: types.DimensionLinkedAccount, Values: []string{"123"}}}, start, start.Add(24*time.Hour), "DAILY")
	if err != nil {
		t.Fatal(err)
	}
	if discovery != 3 || queries != 3 || hooks != 6 || len(rows) != 3 {
		t.Fatalf("discovery=%d queries=%d hooks=%d rows=%d", discovery, queries, hooks, len(rows))
	}
	if rows[0].ReservationARN != "ri-1" || rows[0].AmountExact.RatString() != "17/8" || rows[0].Metric != "AmortizedCost" {
		t.Fatalf("RI row: %+v", rows[0])
	}
	if rows[1].SavingsPlanARN != "sp-1" || rows[1].AmountExact.RatString() != "13/4" {
		t.Fatalf("SP row: %+v", rows[1])
	}
	if rows[2].AmountExact.RatString() != "1" || rows[2].Metric != "UnblendedCost" {
		t.Fatalf("residual: %+v", rows[2])
	}
}

func TestAttributedCostDiscoveryRespectsBudget(t *testing.T) {
	t.Setenv("AWS_MAX_ATTEMPTS", "3")
	var calls, hooks int
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		calls++
		w.Header().Set("Content-Type", "application/x-amz-json-1.1")
		w.WriteHeader(http.StatusBadRequest)
		_, _ = w.Write([]byte(`{"__type":"LimitExceededException","Message":"retry"}`))
	}))
	defer server.Close()
	ce, err := NewClient(context.Background(), Config{Region: "us-east-1", BaseEndpoint: server.URL, AccessKeyID: "test", SecretAccessKey: "test"})
	if err != nil {
		t.Fatal(err)
	}
	ctx := WithPageHook(context.Background(), func() error {
		hooks++
		if hooks > 1 {
			return ErrRateLimited
		}
		return nil
	})
	start := time.Date(2026, 10, 1, 0, 0, 0, 0, time.UTC)
	rows, err := ce.GetAttributedCost(ctx, nil, start, start.Add(24*time.Hour), "DAILY")
	if !errors.Is(err, ErrRateLimited) || rows != nil || calls != 1 || hooks != 2 {
		t.Fatalf("rows=%v err=%v calls=%d hooks=%d", rows, err, calls, hooks)
	}
}
func TestAttributedResourceCostPartitionsAndAmortizes(t *testing.T) {
	var discovery, queries, hooks int
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "application/x-amz-json-1.1")
		var request struct {
			Dimension     string
			NextPageToken *string
			GroupBy       []struct{ Key string }
			Metrics       []string
			Filter        json.RawMessage
		}
		if err := json.NewDecoder(r.Body).Decode(&request); err != nil {
			t.Error(err)
			return
		}
		if strings.HasSuffix(r.Header.Get("X-Amz-Target"), ".GetDimensionValues") {
			discovery++
			if request.Dimension == "RESERVATION_ID" {
				if request.NextPageToken == nil {
					_, _ = w.Write([]byte(`{"DimensionValues":[{"Value":"ri-1"}],"NextPageToken":"page2"}`))
				} else {
					_, _ = w.Write([]byte(`{"DimensionValues":[{"Value":"ri-1"},{"Value":""}]}`))
				}
			} else {
				_, _ = w.Write([]byte(`{"DimensionValues":[{"Value":"sp-1"}]}`))
			}
			return
		}
		queries++
		if len(request.GroupBy) != 1 || request.GroupBy[0].Key != "RESOURCE_ID" {
			t.Errorf("unsupported grouping: %+v", request.GroupBy)
		}
		filter := string(request.Filter)
		if !strings.Contains(filter, "LINKED_ACCOUNT") {
			t.Errorf("caller filter lost: %s", filter)
		}
		amount := "1"
		amortized := ""
		switch queries {
		case 1:
			if !strings.Contains(filter, "ri-1") || strings.Contains(filter, "\"Not\"") {
				t.Errorf("RI filter: %s", filter)
			}
			amortized = `,"AmortizedCost":{"Amount":"2.125","Unit":"USD"}`
		case 2:
			if !strings.Contains(filter, "sp-1") || !strings.Contains(filter, "\"Not\"") || !strings.Contains(filter, "ri-1") {
				t.Errorf("SP overlap exclusion: %s", filter)
			}
			amortized = `,"AmortizedCost":{"Amount":"3.25","Unit":"USD"}`
		case 3:
			if !strings.Contains(filter, "\"Not\"") || !strings.Contains(filter, "ri-1") || !strings.Contains(filter, "sp-1") {
				t.Errorf("residual filter: %s", filter)
			}
		default:
			t.Errorf("unexpected query %d", queries)
		}
		_, _ = w.Write([]byte(`{"ResultsByTime":[{"TimePeriod":{"Start":"2026-10-01","End":"2026-10-02"},"Groups":[{"Keys":["S3"],"Metrics":{"UnblendedCost":{"Amount":"` + amount + `","Unit":"USD"}` + amortized + `}}]}]}`))
	}))
	defer server.Close()
	ce, err := NewClient(context.Background(), Config{Region: "us-east-1", BaseEndpoint: server.URL, AccessKeyID: "test", SecretAccessKey: "test"})
	if err != nil {
		t.Fatal(err)
	}
	ctx := WithPageHook(context.Background(), func() error { hooks++; return nil })
	start := time.Date(2026, 10, 1, 0, 0, 0, 0, time.UTC)
	rows, err := ce.GetAttributedCostWithResources(ctx, "i-1234567890abcdef0", "123", start, start.Add(24*time.Hour))
	if err != nil {
		t.Fatal(err)
	}
	if discovery != 3 || queries != 3 || hooks != 6 || len(rows) != 3 {
		t.Fatalf("discovery=%d queries=%d hooks=%d rows=%d", discovery, queries, hooks, len(rows))
	}
	if rows[0].ReservationARN != "ri-1" || rows[0].AmountExact.RatString() != "17/8" || rows[0].Metric != "AmortizedCost" {
		t.Fatalf("RI row: %+v", rows[0])
	}
	if rows[1].SavingsPlanARN != "sp-1" || rows[1].AmountExact.RatString() != "13/4" {
		t.Fatalf("SP row: %+v", rows[1])
	}
	if rows[2].AmountExact.RatString() != "1" || rows[2].Metric != "UnblendedCost" {
		t.Fatalf("residual: %+v", rows[2])
	}
}

func TestAttributedCostAllowsEmptyPartitions(t *testing.T) {
	var costCalls int
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "application/x-amz-json-1.1")
		if strings.HasSuffix(r.Header.Get("X-Amz-Target"), ".GetDimensionValues") {
			var req struct{ Dimension string }
			_ = json.NewDecoder(r.Body).Decode(&req)
			if req.Dimension == "RESERVATION_ID" {
				_, _ = w.Write([]byte(`{"DimensionValues":[{"Value":"unused"},{"Value":"ri-1"}]}`))
			} else {
				_, _ = w.Write([]byte(`{}`))
			}
			return
		}
		costCalls++
		if costCalls != 2 {
			_, _ = w.Write([]byte(`{"ResultsByTime":[]}`))
			return
		}
		_, _ = w.Write([]byte(`{"ResultsByTime":[{"TimePeriod":{"Start":"2026-10-01","End":"2026-10-02"},"Groups":[{"Keys":["S3"],"Metrics":{"UnblendedCost":{"Amount":"1","Unit":"USD"},"AmortizedCost":{"Amount":"2","Unit":"USD"}}}]}]}`))
	}))
	defer server.Close()
	ce, err := NewClient(context.Background(), Config{Region: "us-east-1", BaseEndpoint: server.URL, AccessKeyID: "test", SecretAccessKey: "test"})
	if err != nil {
		t.Fatal(err)
	}
	start := time.Date(2026, 10, 1, 0, 0, 0, 0, time.UTC)
	rows, err := ce.GetAttributedCost(context.Background(), nil, start, start.Add(24*time.Hour), "DAILY")
	if err != nil || len(rows) != 1 || costCalls != 3 {
		t.Fatalf("rows=%v err=%v calls=%d", rows, err, costCalls)
	}
}

func TestAttributedCostLateFailureReturnsNoRows(t *testing.T) {
	var costCalls int
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "application/x-amz-json-1.1")
		if strings.HasSuffix(r.Header.Get("X-Amz-Target"), ".GetDimensionValues") {
			var req struct{ Dimension string }
			_ = json.NewDecoder(r.Body).Decode(&req)
			if req.Dimension == "RESERVATION_ID" {
				_, _ = w.Write([]byte(`{"DimensionValues":[{"Value":"ri-1"}]}`))
			} else {
				_, _ = w.Write([]byte(`{}`))
			}
			return
		}
		costCalls++
		if costCalls == 2 {
			w.WriteHeader(http.StatusBadRequest)
			_, _ = w.Write([]byte(`{"__type":"AccessDeniedException","Message":"denied"}`))
			return
		}
		_, _ = w.Write([]byte(`{"ResultsByTime":[{"TimePeriod":{"Start":"2026-10-01","End":"2026-10-02"},"Groups":[{"Keys":["S3"],"Metrics":{"UnblendedCost":{"Amount":"1","Unit":"USD"}}}]}]}`))
	}))
	defer server.Close()
	ce, err := NewClient(context.Background(), Config{Region: "us-east-1", BaseEndpoint: server.URL, AccessKeyID: "test", SecretAccessKey: "test"})
	if err != nil {
		t.Fatal(err)
	}
	start := time.Date(2026, 10, 1, 0, 0, 0, 0, time.UTC)
	rows, err := ce.GetAttributedCost(context.Background(), nil, start, start.Add(24*time.Hour), "DAILY")
	if err == nil || rows != nil || costCalls != 2 {
		t.Fatalf("rows=%v err=%v calls=%d", rows, err, costCalls)
	}
}
