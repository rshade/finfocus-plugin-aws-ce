package client

import (
	"context"
	"errors"
	"net/http"
	"net/http/httptest"
	"sync/atomic"
	"testing"
	"time"
)

func TestCostRetryRespectsRequestBudget(t *testing.T) {
	for _, resource := range []bool{false, true} {
		name := "service"
		if resource {
			name = "resource"
		}
		t.Run(name, func(t *testing.T) {
			t.Setenv("AWS_MAX_ATTEMPTS", "3")
			var calls, hooks atomic.Int32
			server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
				w.Header().Set("Content-Type", "application/x-amz-json-1.1")
				if calls.Add(1) == 1 {
					w.WriteHeader(http.StatusBadRequest)
					_, _ = w.Write([]byte(`{"__type":"LimitExceededException","Message":"retry once"}`))
					return
				}
				_, _ = w.Write([]byte(`{"ResultsByTime":[{"TimePeriod":{"Start":"2026-10-01","End":"2026-10-02"},"Groups":[{"Keys":["Amazon Simple Storage Service"],"Metrics":{"UnblendedCost":{"Amount":"1","Unit":"USD"}}}]}]}`))
			}))
			defer server.Close()
			ce, err := NewClient(context.Background(), Config{Region: "us-east-1", BaseEndpoint: server.URL, AccessKeyID: "test", SecretAccessKey: "test"})
			if err != nil {
				t.Fatal(err)
			}
			ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
			defer cancel()
			ctx = WithPageHook(ctx, func() error {
				if hooks.Add(1) > 1 {
					return ErrRateLimited
				}
				return nil
			})
			start := time.Date(2026, 10, 1, 0, 0, 0, 0, time.UTC)
			end := start.Add(24 * time.Hour)
			if resource {
				_, err = ce.GetCostWithResources(ctx, "i-1234567890abcdef0", "", start, end)
			} else {
				_, err = ce.GetCost(ctx, nil, []string{"SERVICE"}, start, end, "DAILY")
			}
			if !errors.Is(err, ErrRateLimited) || calls.Load() != 1 || hooks.Load() != 2 {
				t.Fatalf("retry escaped budget: error=%v calls=%d hooks=%d", err, calls.Load(), hooks.Load())
			}
		})
	}
}
