package pricing

import (
	"context"
	"crypto/hmac"
	"crypto/sha256"
	"encoding/base64"
	"encoding/json"
	"errors"
	"fmt"
	"github.com/aws/smithy-go"
	"github.com/rshade/finfocus-plugin-aws-ce/internal/client"
	pbc "github.com/rshade/finfocus-spec/sdk/go/proto/finfocus/v1"
	"google.golang.org/grpc/codes"
	"google.golang.org/grpc/status"
	"google.golang.org/protobuf/types/known/timestamppb"
	"strings"
	"sync"
	"testing"
	"time"
)

func carbonQuery() *pbc.GetCarbonEmissionsRequest {
	return &pbc.GetCarbonEmissionsRequest{Start: timestamppb.New(time.Date(2026, 1, 1, 0, 0, 0, 0, time.UTC)), End: timestamppb.New(time.Date(2026, 2, 1, 0, 0, 0, 0, time.UTC))}
}
func TestCarbonHandlerValidation(t *testing.T) {
	c := NewCalculator(WithCarbonEnabled(true))
	if _, err := c.GetCarbonEmissions(carbonTestContext(t), nil); status.Code(err) != codes.InvalidArgument {
		t.Fatalf("nil: %v", err)
	}
}
func TestCarbonHandlerPages(t *testing.T) {
	c := NewCalculator(WithCarbonEnabled(true))
	calls := 0
	c.openCarbon = func(context.Context, client.Config) (carbonReader, error) {
		return carbonReaderFunc(func(_ context.Context, _ *pbc.GetCarbonEmissionsRequest, token string) (client.CarbonPage, error) {
			calls++
			return client.CarbonPage{NextToken: "next"}, nil
		}), nil
	}
	q := carbonQuery()
	first, err := c.GetCarbonEmissions(carbonTestContext(t), q)
	if err != nil {
		t.Fatal(err)
	}
	if first.NextPageToken == "" || first.NextPageToken == "next" {
		t.Fatal("unprotected token")
	}
	q.PageToken = first.NextPageToken
	if _, err = c.GetCarbonEmissions(carbonTestContext(t), q); status.Code(err) != codes.Internal {
		t.Fatalf("repeat: %v", err)
	}
	q.PageToken = first.NextPageToken + "x"
	if _, err = c.GetCarbonEmissions(carbonTestContext(t), q); status.Code(err) != codes.InvalidArgument {
		t.Fatalf("tamper: %v", err)
	}
	if calls != 2 {
		t.Fatalf("calls %d", calls)
	}
}

type carbonReaderFunc func(context.Context, *pbc.GetCarbonEmissionsRequest, string) (client.CarbonPage, error)

func (f carbonReaderFunc) ReadCarbonPage(ctx context.Context, q *pbc.GetCarbonEmissionsRequest, t string) (client.CarbonPage, error) {
	return f(ctx, q, t)
}

func TestCarbonCursorSecurity(t *testing.T) {
	c := NewCalculator(WithCarbonEnabled(true))
	now := time.Now()
	c.now = func() time.Time { return now }
	digest := carbonDigest([]byte("token"))
	state := carbonCursor{Query: "query", Caller: "caller", Token: "token", Pages: 1, Expires: now.Add(15 * time.Minute).Unix(), Seen: []string{digest}}
	token, err := c.signCarbon(state)
	if err != nil {
		t.Fatal(err)
	}
	if _, err = c.readCarbon(token, "query", "caller"); err != nil {
		t.Fatal(err)
	}
	for _, tc := range []struct{ name, token, query, caller string }{{"tamper", token + "x", "query", "caller"}, {"query", token, "other", "caller"}, {"caller", token, "query", "other"}, {"oversize", string(make([]byte, 16385)), "query", "caller"}} {
		t.Run(tc.name, func(t *testing.T) {
			if _, err := c.readCarbon(tc.token, tc.query, tc.caller); err == nil {
				t.Fatal("accepted invalid cursor")
			}
		})
	}
	if _, err = NewCalculator().readCarbon(token, "query", "caller"); err == nil {
		t.Fatal("accepted cursor after restart")
	}
	now = now.Add(16 * time.Minute)
	if _, err = c.readCarbon(token, "query", "caller"); err == nil {
		t.Fatal("accepted expired cursor")
	}
}
func TestCarbonHandlerRejectsInjectedData(t *testing.T) {
	for _, tc := range []struct {
		name string
		page client.CarbonPage
	}{{"nil-record", client.CarbonPage{Records: []*pbc.CarbonEmissionRecord{nil}}}, {"long-token", client.CarbonPage{NextToken: string(make([]byte, 2001))}}} {
		t.Run(tc.name, func(t *testing.T) {
			c := NewCalculator(WithCarbonEnabled(true))
			c.openCarbon = func(context.Context, client.Config) (carbonReader, error) {
				return carbonReaderFunc(func(context.Context, *pbc.GetCarbonEmissionsRequest, string) (client.CarbonPage, error) {
					return tc.page, nil
				}), nil
			}
			if _, err := c.GetCarbonEmissions(carbonTestContext(t), carbonQuery()); status.Code(err) != codes.Internal {
				t.Fatalf("err %v", err)
			}
		})
	}
}
func TestCarbonHandlerPageCap(t *testing.T) {
	c := NewCalculator(WithCarbonEnabled(true))
	i := 0
	c.openCarbon = func(context.Context, client.Config) (carbonReader, error) {
		return carbonReaderFunc(func(context.Context, *pbc.GetCarbonEmissionsRequest, string) (client.CarbonPage, error) {
			i++
			return client.CarbonPage{NextToken: fmt.Sprint(i)}, nil
		}), nil
	}
	q := carbonQuery()
	for page := 1; page <= 100; page++ {
		r, err := c.GetCarbonEmissions(carbonTestContext(t), q)
		if page == 100 {
			if status.Code(err) != codes.Internal {
				t.Fatalf("cap: %v", err)
			}
			break
		}
		if err != nil {
			t.Fatalf("page %d: %v", page, err)
		}
		q.PageToken = r.NextPageToken
	}
}
func TestCarbonHandlerConcurrent(t *testing.T) {
	c := NewCalculator(WithCarbonEnabled(true))
	c.openCarbon = func(context.Context, client.Config) (carbonReader, error) {
		return carbonReaderFunc(func(ctx context.Context, _ *pbc.GetCarbonEmissionsRequest, _ string) (client.CarbonPage, error) {
			return client.CarbonPage{}, ctx.Err()
		}), nil
	}
	var wg sync.WaitGroup
	for range 12 {
		wg.Go(func() {
			if _, err := c.GetCarbonEmissions(carbonTestContext(t), carbonQuery()); err != nil {
				t.Error(err)
			}
		})
	}
	wg.Wait()
	ctx, cancel := context.WithCancel(carbonTestContext(t))
	cancel()
	if _, err := c.GetCarbonEmissions(ctx, carbonQuery()); status.Code(err) != codes.Canceled {
		t.Fatalf("cancel: %v", err)
	}
}

func TestCarbonHandlerCallerAndQueryBinding(t *testing.T) {
	c := NewCalculator(WithCarbonEnabled(true))
	calls := 0
	c.openCarbon = func(_ context.Context, cfg client.Config) (carbonReader, error) {
		calls++
		if cfg.AccessKeyID != "caller-a" {
			t.Errorf("unexpected credential context")
		}
		return carbonReaderFunc(func(context.Context, *pbc.GetCarbonEmissionsRequest, string) (client.CarbonPage, error) {
			return client.CarbonPage{NextToken: "private-aws-token"}, nil
		}), nil
	}
	a := credentialContext(t, map[string]string{"access_key_id": "caller-a", "secret_access_key": "secret-a"})
	b := credentialContext(t, map[string]string{"access_key_id": "caller-b", "secret_access_key": "secret-b"})
	q := carbonQuery()
	q.Regions = []string{"eu-west-1", "eu-west-1"}
	first, err := c.GetCarbonEmissions(a, q)
	if err != nil {
		t.Fatal(err)
	}
	q.PageToken = first.NextPageToken
	q.Regions = []string{"eu-west-1"}
	if _, err = c.GetCarbonEmissions(b, q); status.Code(err) != codes.InvalidArgument {
		t.Fatalf("wrong caller: %v", err)
	}
	q.PageSize = 5
	if _, err = c.GetCarbonEmissions(a, q); status.Code(err) != codes.InvalidArgument {
		t.Fatalf("wrong size: %v", err)
	}
	q.PageSize = 0
	q.Method = pbc.CarbonAccountingMethod_CARBON_ACCOUNTING_METHOD_LOCATION_BASED
	if _, err = c.GetCarbonEmissions(a, q); status.Code(err) != codes.InvalidArgument {
		t.Fatalf("wrong method: %v", err)
	}
	q.Method = 0
	if _, err = c.GetCarbonEmissions(a, q); status.Code(err) != codes.Internal {
		t.Fatalf("normalized query should reach repeated-token check: %v", err)
	}
	if calls != 2 {
		t.Fatalf("factory calls %d", calls)
	}
}
func TestCarbonStatusSanitized(t *testing.T) {
	for _, tc := range []struct {
		code string
		want codes.Code
	}{{"AccessDeniedException", codes.PermissionDenied}, {"ExpiredTokenException", codes.Unauthenticated}, {"ThrottlingException", codes.ResourceExhausted}, {"ServiceUnavailableException", codes.Unavailable}, {"secret-code", codes.Internal}} {
		err := carbonStatus(&smithy.GenericAPIError{Code: tc.code, Message: "secret-value"})
		if status.Code(err) != tc.want || strings.Contains(err.Error(), "secret") {
			t.Fatalf("unsafe error: %v", err)
		}
	}
}

func TestCarbonHandlerTwoCallerConcurrentWalks(t *testing.T) {
	c := NewCalculator(WithCarbonEnabled(true))
	c.openCarbon = func(_ context.Context, cfg client.Config) (carbonReader, error) {
		caller := cfg.AccessKeyID
		return carbonReaderFunc(func(_ context.Context, _ *pbc.GetCarbonEmissionsRequest, token string) (client.CarbonPage, error) {
			if token == "" {
				return client.CarbonPage{NextToken: caller + "-next"}, nil
			}
			if token != caller+"-next" {
				return client.CarbonPage{}, errors.New("cross-caller pagination")
			}
			return client.CarbonPage{}, nil
		}), nil
	}
	contexts := []context.Context{credentialContext(t, map[string]string{"access_key_id": "a", "secret_access_key": "a-secret"}), credentialContext(t, map[string]string{"access_key_id": "b", "secret_access_key": "b-secret"})}
	var wg sync.WaitGroup
	for _, ctx := range contexts {
		wg.Go(func() {
			q := carbonQuery()
			page, err := c.GetCarbonEmissions(ctx, q)
			if err != nil {
				t.Error(err)
				return
			}
			q.PageToken = page.NextPageToken
			page, err = c.GetCarbonEmissions(ctx, q)
			if err != nil || page == nil || page.NextPageToken != "" {
				t.Errorf("walk: %v %v", page, err)
			}
		})
	}
	wg.Wait()
}

func TestCarbonCursorRejectsStrictPayload(t *testing.T) {
	c := NewCalculator()
	state := carbonCursor{Query: "q", Caller: "c", Token: "t", Pages: 1, Expires: time.Now().Add(time.Minute).Unix(), Seen: []string{carbonDigest([]byte("t"))}}
	body, _ := json.Marshal(state)
	for _, payload := range [][]byte{append(append([]byte{}, body...), []byte(` {}`)...), append(append([]byte{}, body[:len(body)-1]...), []byte(`,"unknown":true}`)...)} {
		mac := hmac.New(sha256.New, []byte(c.carbonKey))
		mac.Write(payload)
		token := base64.RawURLEncoding.EncodeToString(payload) + "." + base64.RawURLEncoding.EncodeToString(mac.Sum(nil))
		if _, err := c.readCarbon(token, "q", "c"); err == nil {
			t.Fatal("accepted malformed signed payload")
		}
	}
}
func TestCarbonHandlerNilReaderAndUnavailable(t *testing.T) {
	for _, upstream := range []error{nil, &smithy.GenericAPIError{Code: "ServiceUnavailableException", Message: "private"}} {
		c := NewCalculator(WithCarbonEnabled(true))
		c.openCarbon = func(context.Context, client.Config) (carbonReader, error) { return nil, upstream }
		_, err := c.GetCarbonEmissions(carbonTestContext(t), carbonQuery())
		want := codes.Internal
		if upstream != nil {
			want = codes.Unavailable
		}
		if status.Code(err) != want {
			t.Fatalf("got %v", err)
		}
	}
}

func TestCarbonValidationErrorStatus(t *testing.T) {
	err := carbonStatus(&smithy.GenericAPIError{Code: "ValidationException", Message: "secret"})
	if status.Code(err) != codes.InvalidArgument {
		t.Fatal(err)
	}
}
func TestCarbonEmptyFiltersBeforeFactory(t *testing.T) {
	c := NewCalculator(WithCarbonEnabled(true))
	c.openCarbon = func(context.Context, client.Config) (carbonReader, error) {
		t.Fatal("factory called for empty filter")
		return nil, nil
	}
	q := carbonQuery()
	q.Regions = []string{" "}
	if _, err := c.GetCarbonEmissions(carbonTestContext(t), q); status.Code(err) != codes.InvalidArgument {
		t.Fatal(err)
	}
}

func carbonTestContext(t *testing.T) context.Context {
	return credentialContext(t, map[string]string{"access_key_id": "fixture-key", "secret_access_key": "fixture-secret"})
}
