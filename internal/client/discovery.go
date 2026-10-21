package client

import (
	"context"
	"encoding/json"
	"errors"
	"sync"
	"time"

	"github.com/aws/aws-sdk-go-v2/service/costexplorer/types"
)

// discoveryTTL is how long commitment ids found for one filter and interval are
// reused. Reservations and Savings Plans change over days, not minutes.
const discoveryTTL = 5 * time.Minute

type discoveryEntry struct {
	ready   chan struct{}
	ids     []string
	err     error
	expires time.Time
}

// discoveryCache shares commitment discovery between concurrent and repeated
// queries. One lookup per key runs at a time; failures are never kept.
type discoveryCache struct {
	mu      sync.Mutex
	entries map[string]*discoveryEntry
	now     func() time.Time
}

func (d *discoveryCache) clock() time.Time {
	if d.now != nil {
		return d.now()
	}
	return time.Now()
}

func discoveryKey(dimension types.Dimension, interval *types.DateInterval, filter *types.Expression) (string, error) {
	raw, err := json.Marshal(struct {
		Dimension types.Dimension
		Interval  *types.DateInterval
		Filter    *types.Expression
	}{dimension, interval, filter})
	if err != nil {
		return "", err
	}
	return string(raw), nil
}

func (d *discoveryCache) get(ctx context.Context, key string, load func(context.Context) ([]string, error)) ([]string, error) {
	for {
		d.mu.Lock()
		if d.entries == nil {
			d.entries = make(map[string]*discoveryEntry)
		}
		entry, found := d.entries[key]
		if found && entry.finished() && !d.clock().Before(entry.expires) {
			delete(d.entries, key)
			found = false
		}
		if !found {
			d.pruneLocked()
			entry = &discoveryEntry{ready: make(chan struct{})}
			d.entries[key] = entry
			d.mu.Unlock()
			return d.lead(ctx, key, entry, load)
		}
		d.mu.Unlock()
		select {
		case <-entry.ready:
		case <-ctx.Done():
			return nil, ctx.Err()
		}
		if entry.err == nil {
			return append([]string(nil), entry.ids...), nil
		}
		if ctx.Err() != nil {
			return nil, ctx.Err()
		}
		if errors.Is(entry.err, context.Canceled) || errors.Is(entry.err, context.DeadlineExceeded) {
			continue
		}
		return nil, entry.err
	}
}

// maxDiscoveryEntries bounds the cache, since keys vary with callers' accounts
// and periods.
const maxDiscoveryEntries = 256

// pruneLocked drops expired entries, and every finished entry once the cache
// is full. Lookups in flight are kept so their waiters still find them.
func (d *discoveryCache) pruneLocked() {
	now := d.clock()
	full := len(d.entries) >= maxDiscoveryEntries
	for key, entry := range d.entries {
		if entry.finished() && (full || !now.Before(entry.expires)) {
			delete(d.entries, key)
		}
	}
}

func (e *discoveryEntry) finished() bool {
	select {
	case <-e.ready:
		return true
	default:
		return false
	}
}

func (d *discoveryCache) lead(ctx context.Context, key string, entry *discoveryEntry, load func(context.Context) ([]string, error)) ([]string, error) {
	ids, err := load(ctx)
	d.mu.Lock()
	entry.ids, entry.err = ids, err
	entry.expires = d.clock().Add(discoveryTTL)
	if err != nil {
		delete(d.entries, key)
	}
	d.mu.Unlock()
	close(entry.ready)
	if err != nil {
		return nil, err
	}
	return append([]string(nil), ids...), nil
}
