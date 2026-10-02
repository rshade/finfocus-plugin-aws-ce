package client

import (
	"context"
	"fmt"
	"io"
	"net"
	"net/http"
	"net/http/httptest"
	"net/url"
	"sync"
	"testing"
	"time"
)

func TestNewClient_RoleARNAssumeRoleLoopback(t *testing.T) {
	t.Setenv("AWS_EC2_METADATA_DISABLED", "true")

	const roleARN = "arn:aws:iam::123456789012:role/finfocus-ce"
	const secret = "ce16-assume-secret"

	var mu sync.Mutex
	var bodies []string
	handler := http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		host, _, err := net.SplitHostPort(r.Host)
		if err != nil {
			host = r.Host
		}
		ip := net.ParseIP(host)
		if ip == nil || !ip.IsLoopback() {
			t.Errorf("request host %s is not loopback", r.Host)
		}
		body, err := io.ReadAll(r.Body)
		if err != nil {
			t.Errorf("read body: %v", err)
		}
		raw := string(body)
		if r.URL.RawQuery != "" {
			raw += "&" + r.URL.RawQuery
		}
		mu.Lock()
		bodies = append(bodies, raw)
		mu.Unlock()
		http.Error(w, "not a cost response", http.StatusBadRequest)
	})

	lis, err := net.Listen("tcp", "127.0.0.1:0")
	if err != nil {
		t.Fatalf("listen: %v", err)
	}
	srv := httptest.NewUnstartedServer(handler)
	srv.Listener = lis
	srv.Start()
	t.Cleanup(srv.Close)

	endpoint, err := url.Parse(srv.URL)
	if err != nil {
		t.Fatalf("parse endpoint: %v", err)
	}
	if endpoint.Hostname() != "127.0.0.1" {
		t.Fatalf("dial target %s is not 127.0.0.1", endpoint.Host)
	}

	dialer := &net.Dialer{}
	transport := &http.Transport{
		DialContext: func(ctx context.Context, network, addr string) (net.Conn, error) {
			host, _, splitErr := net.SplitHostPort(addr)
			if splitErr != nil {
				return nil, splitErr
			}
			ip := net.ParseIP(host)
			if ip == nil || !ip.IsLoopback() {
				return nil, fmt.Errorf("dial target %s is not loopback", addr)
			}
			return dialer.DialContext(ctx, network, addr)
		},
	}

	ctx, cancel := context.WithTimeout(context.Background(), 15*time.Second)
	defer cancel()

	ce, err := NewClient(ctx, Config{
		Region:          "us-east-1",
		BaseEndpoint:    srv.URL,
		AccessKeyID:     "AKIACE16ASSUME",
		SecretAccessKey: secret,
		RoleARN:         roleARN,
		httpClient:      &http.Client{Transport: transport, Timeout: 10 * time.Second},
	})
	if err != nil {
		t.Fatalf("NewClient: %v", err)
	}

	start := time.Date(2026, 9, 1, 0, 0, 0, 0, time.UTC)
	end := time.Date(2026, 9, 2, 0, 0, 0, 0, time.UTC)
	_, costErr := ce.GetCost(ctx, nil, []string{"SERVICE"}, start, end, "DAILY")

	mu.Lock()
	seen := append([]string(nil), bodies...)
	mu.Unlock()

	var found bool
	for _, body := range seen {
		values, parseErr := url.ParseQuery(body)
		if parseErr != nil {
			continue
		}
		if values.Get("Action") == "AssumeRole" && values.Get("RoleArn") == roleARN {
			found = true
		}
	}
	if !found {
		t.Fatalf("no AssumeRole for %s on loopback; costErr=%v bodies=%q", roleARN, costErr, seen)
	}
}
