package client

import (
	"context"
	"errors"
	"net/http"
	"net/http/httptest"
	"sync"
	"sync/atomic"
	"testing"
	"time"
)

type fakeClock struct {
	mu  sync.Mutex
	now time.Time
}

func (f *fakeClock) Now() time.Time {
	f.mu.Lock()
	defer f.mu.Unlock()
	return f.now
}

func (f *fakeClock) advance(d time.Duration) {
	f.mu.Lock()
	f.now = f.now.Add(d)
	f.mu.Unlock()
}

func newTestPacer(perMinute int, maxWait time.Duration) (*Pacer, *fakeClock, *[]time.Duration) {
	clock := &fakeClock{now: time.Date(2026, 10, 20, 12, 0, 0, 0, time.UTC)}
	var slept []time.Duration
	p := NewPacer(PacerConfig{PerMinute: perMinute, MaxWait: maxWait})
	p.now = clock.Now
	p.rand = func() float64 { return 0.5 }
	p.sleep = func(_ context.Context, d time.Duration) error {
		slept = append(slept, d)
		clock.advance(d)
		return nil
	}
	return p, clock, &slept
}

func TestPacerAllowsUpToTheLimitWithoutWaiting(t *testing.T) {
	p, _, slept := newTestPacer(3, time.Minute)
	for i := 0; i < 3; i++ {
		if err := p.Acquire(context.Background()); err != nil {
			t.Fatal(err)
		}
	}
	if len(*slept) != 0 {
		t.Fatalf("slept %v, want no waiting inside the budget", *slept)
	}
}

func TestPacerQueuesInsteadOfFailingWhenOverBudget(t *testing.T) {
	p, _, slept := newTestPacer(2, 2*time.Minute)
	for i := 0; i < 3; i++ {
		if err := p.Acquire(context.Background()); err != nil {
			t.Fatalf("request %d: %v", i, err)
		}
	}
	if len(*slept) != 1 || (*slept)[0] != time.Minute {
		t.Fatalf("slept %v, want one wait of a minute for the third request", *slept)
	}
}

func TestPacerFailsFastWhenWaitExceedsMaxWait(t *testing.T) {
	p, _, slept := newTestPacer(1, 10*time.Second)
	if err := p.Acquire(context.Background()); err != nil {
		t.Fatal(err)
	}
	err := p.Acquire(context.Background())
	if !errors.Is(err, ErrRateLimited) {
		t.Fatalf("got %v, want ErrRateLimited", err)
	}
	if len(*slept) != 0 {
		t.Fatalf("slept %v, want none: the wait was refused up front", *slept)
	}
}

func TestPacerWithoutMaxWaitKeepsFailFastBehaviour(t *testing.T) {
	p, _, _ := newTestPacer(1, 0)
	if err := p.Acquire(context.Background()); err != nil {
		t.Fatal(err)
	}
	if err := p.Acquire(context.Background()); !errors.Is(err, ErrRateLimited) {
		t.Fatalf("got %v, want ErrRateLimited", err)
	}
}

func TestPacerRefusesAWaitPastTheCallerDeadline(t *testing.T) {
	p, clock, _ := newTestPacer(1, time.Hour)
	if err := p.Acquire(context.Background()); err != nil {
		t.Fatal(err)
	}
	ctx, cancel := context.WithDeadline(context.Background(), clock.Now().Add(5*time.Second))
	defer cancel()
	if err := p.Acquire(ctx); !errors.Is(err, ErrRateLimited) {
		t.Fatalf("got %v, want ErrRateLimited rather than waiting past the deadline", err)
	}
}

func TestPacerBackoffAfterThrottleDelaysEveryCallerAndGrows(t *testing.T) {
	p, _, slept := newTestPacer(0, time.Minute)
	p.Throttled()
	if err := p.Acquire(context.Background()); err != nil {
		t.Fatal(err)
	}
	p.Throttled()
	if err := p.Acquire(context.Background()); err != nil {
		t.Fatal(err)
	}
	if len(*slept) != 2 || (*slept)[1] <= (*slept)[0] {
		t.Fatalf("slept %v, want two waits with the second longer", *slept)
	}
}

func TestPacerSuccessResetsTheBackoff(t *testing.T) {
	p, _, slept := newTestPacer(0, time.Minute)
	p.Throttled()
	p.Throttled()
	p.Succeeded()
	p.Throttled()
	if err := p.Acquire(context.Background()); err != nil {
		t.Fatal(err)
	}
	if len(*slept) != 1 || (*slept)[0] > 2*time.Second {
		t.Fatalf("slept %v, want the first-throttle delay after a success", *slept)
	}
}

func TestPacerThrottleHalvesTheRateAndQuietTimeRestoresIt(t *testing.T) {
	p, clock, _ := newTestPacer(8, time.Hour)
	p.Throttled()
	if got := p.EffectiveLimit(); got != 4 {
		t.Fatalf("effective limit %d, want 4 after one throttle", got)
	}
	p.Throttled()
	p.Throttled()
	p.Throttled()
	if got := p.EffectiveLimit(); got != 1 {
		t.Fatalf("effective limit %d, want a floor of 1", got)
	}
	for i := 0; i < 20; i++ {
		clock.advance(31 * time.Second)
		p.Succeeded()
	}
	if got := p.EffectiveLimit(); got != 8 {
		t.Fatalf("effective limit %d, want recovery to 8", got)
	}
}

func TestPacerSharesOneQueueAcrossConcurrentCallers(t *testing.T) {
	p := NewPacer(PacerConfig{PerMinute: 4, MaxWait: 5 * time.Minute})
	var mu sync.Mutex
	var total time.Duration
	p.sleep = func(_ context.Context, d time.Duration) error {
		mu.Lock()
		total += d
		mu.Unlock()
		return nil
	}
	var wg sync.WaitGroup
	for i := 0; i < 12; i++ {
		wg.Add(1)
		go func() {
			defer wg.Done()
			if err := p.Acquire(context.Background()); err != nil {
				t.Error(err)
			}
		}()
	}
	wg.Wait()
	if total < 2*time.Minute {
		t.Fatalf("total queued wait %v, want at least 2m for 12 requests at 4 per minute", total)
	}
}

func TestThrottledRequestsCompleteThroughTheSharedPacer(t *testing.T) {
	var calls atomic.Int32
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "application/x-amz-json-1.1")
		if calls.Add(1) <= 2 {
			w.WriteHeader(http.StatusBadRequest)
			_, _ = w.Write([]byte(`{"__type":"LimitExceededException","Message":"slow down"}`))
			return
		}
		_, _ = w.Write([]byte(`{"ResultsByTime":[{"TimePeriod":{"Start":"2026-10-01","End":"2026-10-02"},"Groups":[{"Keys":["Amazon Simple Storage Service"],"Metrics":{"UnblendedCost":{"Amount":"1","Unit":"USD"}}}]}]}`))
	}))
	defer server.Close()
	ce, err := NewClient(context.Background(), Config{Region: "us-east-1", BaseEndpoint: server.URL, AccessKeyID: "test", SecretAccessKey: "test"})
	if err != nil {
		t.Fatal(err)
	}
	p := NewPacer(PacerConfig{PerMinute: 8, MaxWait: time.Minute})
	var waits []time.Duration
	p.sleep = func(_ context.Context, d time.Duration) error { waits = append(waits, d); return nil }
	ctx := WithPacer(context.Background(), p)

	start := time.Date(2026, 10, 1, 0, 0, 0, 0, time.UTC)
	rows, err := ce.GetCost(ctx, nil, []string{"SERVICE"}, start, start.Add(24*time.Hour), "DAILY")
	if err != nil {
		t.Fatalf("the query should complete after backing off, got %v", err)
	}
	if len(rows) != 1 || calls.Load() != 3 {
		t.Fatalf("rows=%d calls=%d, want 1 row after 3 attempts", len(rows), calls.Load())
	}
	if len(waits) != 2 || waits[1] <= waits[0] {
		t.Fatalf("waits %v, want two growing waits from the pacer and no extra per-call delay", waits)
	}
	if got := p.EffectiveLimit(); got >= 8 {
		t.Fatalf("effective limit %d, want it lowered by the throttles", got)
	}
}
