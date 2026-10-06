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
	var costCalls int
	expires := time.Now().UTC().Add(2 * time.Hour).Format(time.RFC3339)
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

		values, _ := url.ParseQuery(raw)
		if values.Get("Action") == "AssumeRole" {
			w.Header().Set("Content-Type", "text/xml")
			_, _ = io.WriteString(w, `<AssumeRoleResponse xmlns="https://sts.amazonaws.com/doc/2011-06-15/">
  <AssumeRoleResult>
    <Credentials>
      <AccessKeyId>ASIACE16TEMP</AccessKeyId>
      <SecretAccessKey>ce16-temp-secret</SecretAccessKey>
      <SessionToken>ce16-temp-session</SessionToken>
      <Expiration>`+expires+`</Expiration>
    </Credentials>
  </AssumeRoleResult>
  <ResponseMetadata><RequestId>ce16-assume</RequestId></ResponseMetadata>
</AssumeRoleResponse>`)
			return
		}

		mu.Lock()
		costCalls++
		page := costCalls
		mu.Unlock()
		w.Header().Set("Content-Type", "application/x-amz-json-1.1")
		token := ""
		if page == 1 {
			token = `,"NextPageToken":"page-2"`
		}
		_, _ = io.WriteString(w, `{"ResultsByTime":[{"Estimated":false,"TimePeriod":{"Start":"2026-09-01","End":"2026-09-02"},"Groups":[{"Keys":["AmazonS3"],"Metrics":{"UnblendedCost":{"Amount":"1.00","Unit":"USD"}}}]}]`+token+`}`)
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
	pages := costCalls
	mu.Unlock()

	assumeRole := 0
	for _, body := range seen {
		values, parseErr := url.ParseQuery(body)
		if parseErr != nil {
			continue
		}
		if values.Get("Action") == "AssumeRole" && values.Get("RoleArn") == roleARN {
			assumeRole++
		}
	}
	if pages < 2 {
		t.Fatalf("cost pages = %d, want at least 2; costErr=%v bodies=%d", pages, costErr, len(seen))
	}
	if assumeRole != 1 {
		t.Fatalf("AssumeRole calls = %d, want 1; costErr=%v", assumeRole, costErr)
	}
}
