package client

import (
	"context"
	"errors"
	"io"
	"net"
	"net/http"
	"strings"
	"syscall"
	"testing"
	"time"
)

type retryHTTPClient struct {
	do func(*http.Request) (*http.Response, error)
}

func (c retryHTTPClient) Do(req *http.Request) (*http.Response, error) { return c.do(req) }

func TestTransientRetriesUseRequestBudget(t *testing.T) {
	for _, failure := range []string{"http503", "connection reset"} {
		for _, limited := range []bool{false, true} {
			name := failure + "/success"
			if limited {
				name = failure + "/budget exhausted"
			}
			t.Run(name, func(t *testing.T) {
				t.Setenv("AWS_MAX_ATTEMPTS", "3")
				calls, hooks := 0, 0
				transport := retryHTTPClient{do: func(req *http.Request) (*http.Response, error) {
					calls++
					if calls == 1 && failure == "connection reset" {
						return nil, &net.OpError{Op: "read", Net: "tcp", Err: syscall.ECONNRESET}
					}
					status, body := http.StatusOK, `{"ResultsByTime":[{"TimePeriod":{"Start":"2026-10-01","End":"2026-10-02"},"Groups":[{"Keys":["S3"],"Metrics":{"UnblendedCost":{"Amount":"1","Unit":"USD"}}}]}]}`
					if calls == 1 {
						status = http.StatusServiceUnavailable
						body = `{"__type":"ServiceUnavailableException","Message":"temporary"}`
					}
					return &http.Response{StatusCode: status, Header: http.Header{"Content-Type": []string{"application/x-amz-json-1.1"}}, Body: io.NopCloser(strings.NewReader(body)), Request: req}, nil
				}}
				ce, err := NewClient(context.Background(), Config{Region: "us-east-1", BaseEndpoint: "https://ce.example.test", AccessKeyID: "test", SecretAccessKey: "test", httpClient: transport})
				if err != nil {
					t.Fatal(err)
				}
				ctx := WithPageHook(context.Background(), func() error {
					hooks++
					if limited && hooks > 1 {
						return ErrRateLimited
					}
					return nil
				})
				start := time.Date(2026, 10, 1, 0, 0, 0, 0, time.UTC)
				rows, err := ce.GetCost(ctx, nil, []string{"SERVICE"}, start, start.Add(24*time.Hour), "DAILY")
				if limited {
					if !errors.Is(err, ErrRateLimited) || rows != nil || calls != 1 || hooks != 2 {
						t.Fatalf("budget escaped: calls=%d hooks=%d rows=%v err=%v", calls, hooks, rows, err)
					}
				} else if err != nil || len(rows) != 1 || calls != 2 || hooks != 2 {
					t.Fatalf("transient retry: calls=%d hooks=%d rows=%v err=%v", calls, hooks, rows, err)
				}
			})
		}
	}
}
