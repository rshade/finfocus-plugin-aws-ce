package client

import (
	"context"
	"errors"
	"sync"
	"sync/atomic"
	"testing"
	"time"
)

func TestDiscoveryRunsOneLookupForConcurrentCallers(t *testing.T) {
	t.Parallel()
	var d discoveryCache
	var loads atomic.Int32
	release := make(chan struct{})
	load := func(context.Context) ([]string, error) {
		loads.Add(1)
		<-release
		return []string{"r-1"}, nil
	}
	var wg sync.WaitGroup
	results := make([][]string, 8)
	for i := range results {
		wg.Add(1)
		go func() {
			defer wg.Done()
			ids, err := d.get(context.Background(), "k", load)
			if err != nil {
				t.Errorf("get: %v", err)
			}
			results[i] = ids
		}()
	}
	time.Sleep(20 * time.Millisecond)
	close(release)
	wg.Wait()
	if loads.Load() != 1 {
		t.Fatalf("loads = %d, want 1", loads.Load())
	}
	for _, ids := range results {
		if len(ids) != 1 || ids[0] != "r-1" {
			t.Fatalf("ids = %v", ids)
		}
	}
}

func TestDiscoveryExpiresAfterTTL(t *testing.T) {
	t.Parallel()
	now := time.Date(2026, 10, 20, 12, 0, 0, 0, time.UTC)
	d := discoveryCache{now: func() time.Time { return now }}
	var loads int
	load := func(context.Context) ([]string, error) { loads++; return nil, nil }
	for range 3 {
		if _, err := d.get(context.Background(), "k", load); err != nil {
			t.Fatal(err)
		}
	}
	if loads != 1 {
		t.Fatalf("loads inside TTL = %d, want 1", loads)
	}
	now = now.Add(discoveryTTL)
	if _, err := d.get(context.Background(), "k", load); err != nil {
		t.Fatal(err)
	}
	if loads != 2 {
		t.Fatalf("loads after TTL = %d, want 2", loads)
	}
}

func TestDiscoveryDoesNotKeepFailures(t *testing.T) {
	t.Parallel()
	var d discoveryCache
	boom := errors.New("boom")
	var loads int
	load := func(context.Context) ([]string, error) {
		loads++
		if loads == 1 {
			return nil, boom
		}
		return []string{"r-1"}, nil
	}
	if _, err := d.get(context.Background(), "k", load); !errors.Is(err, boom) {
		t.Fatalf("first err = %v", err)
	}
	ids, err := d.get(context.Background(), "k", load)
	if err != nil || len(ids) != 1 {
		t.Fatalf("retry = %v, %v", ids, err)
	}
}

func TestDiscoveryLeaderCancelDoesNotFailFollower(t *testing.T) {
	t.Parallel()
	var d discoveryCache
	leaderCtx, cancel := context.WithCancel(context.Background())
	started := make(chan struct{})
	var loads atomic.Int32
	load := func(ctx context.Context) ([]string, error) {
		if loads.Add(1) == 1 {
			close(started)
			<-ctx.Done()
			return nil, ctx.Err()
		}
		return []string{"r-2"}, nil
	}
	leaderDone := make(chan error, 1)
	go func() {
		_, err := d.get(leaderCtx, "k", load)
		leaderDone <- err
	}()
	<-started
	followerDone := make(chan []string, 1)
	go func() {
		ids, err := d.get(context.Background(), "k", load)
		if err != nil {
			t.Errorf("follower: %v", err)
		}
		followerDone <- ids
	}()
	time.Sleep(20 * time.Millisecond)
	cancel()
	if err := <-leaderDone; !errors.Is(err, context.Canceled) {
		t.Fatalf("leader err = %v", err)
	}
	if ids := <-followerDone; len(ids) != 1 || ids[0] != "r-2" {
		t.Fatalf("follower ids = %v", ids)
	}
}
