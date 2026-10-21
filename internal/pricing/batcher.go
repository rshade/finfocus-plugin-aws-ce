package pricing

import (
	"context"
	"os"
	"strconv"
	"strings"
	"sync"
	"time"

	"github.com/rshade/finfocus-plugin-aws-ce/internal/client"
)

// batchWindowEnv is the time, in milliseconds, that resource cost lookups wait
// for others to join one Cost Explorer query. 0 sends every lookup alone.
const batchWindowEnv = "FINFOCUS_AWS_CE_BATCH_WINDOW_MS"

const (
	defaultBatchWindow = 50 * time.Millisecond
	maxBatchWindow     = 5 * time.Second
	// maxBatchIDs bounds one RESOURCE_ID filter.
	maxBatchIDs = 50
	// batchFlushTimeout bounds a shared query that no single caller owns.
	batchFlushTimeout = 2 * time.Minute
)

func batchWindowFromEnv() time.Duration {
	raw := strings.TrimSpace(os.Getenv(batchWindowEnv))
	if raw == "" {
		return defaultBatchWindow
	}
	ms, err := strconv.Atoi(raw)
	if err != nil || ms < 0 {
		return defaultBatchWindow
	}
	return min(time.Duration(ms)*time.Millisecond, maxBatchWindow)
}

type batchKey struct {
	service, account string
	start, end       int64
}

type rowQuery func(ctx context.Context, ids []string) ([]client.CostResult, error)

type batchWaiter struct {
	ids      []string
	done     chan struct{}
	rows     []client.CostResult
	requests int
	err      error
}

type batchGroup struct {
	waiters  []*batchWaiter
	ids      []string
	seen     map[string]bool
	timer    *time.Timer
	ctx      context.Context
	run      rowQuery
	flushed  bool
	requests int
}

// resourceBatcher joins resource cost lookups that arrive together into one
// Cost Explorer query per account, service and period.
type resourceBatcher struct {
	window time.Duration
	maxIDs int
	mu     sync.Mutex
	groups map[batchKey]*batchGroup
}

func newResourceBatcher(window time.Duration) *resourceBatcher {
	return &resourceBatcher{window: window, maxIDs: maxBatchIDs, groups: make(map[batchKey]*batchGroup)}
}

// fetch returns the rows for ids and the number of Cost Explorer requests spent
// by the shared query. A nil batcher or a zero window queries alone.
func (b *resourceBatcher) fetch(ctx context.Context, key batchKey, ids []string, run rowQuery) ([]client.CostResult, int, error) {
	if b == nil || b.window <= 0 {
		rows, err := run(ctx, ids)
		return rows, 0, err
	}
	w := &batchWaiter{ids: ids, done: make(chan struct{})}
	b.mu.Lock()
	g := b.groups[key]
	if g != nil && len(g.ids)+g.newIDs(ids) > b.maxIDs {
		b.takeLocked(key, g)
		go b.flush(g)
		g = nil
	}
	if g == nil {
		g = &batchGroup{seen: make(map[string]bool), ctx: ctx, run: run}
		g.timer = time.AfterFunc(b.window, func() { b.flushDue(key, g) })
		b.groups[key] = g
	}
	g.add(w)
	if len(g.ids) >= b.maxIDs {
		b.takeLocked(key, g)
		go b.flush(g)
	}
	b.mu.Unlock()
	select {
	case <-w.done:
		return w.rows, w.requests, w.err
	case <-ctx.Done():
		return nil, 0, ctx.Err()
	}
}

func (g *batchGroup) newIDs(ids []string) int {
	n := 0
	for _, id := range ids {
		if !g.seen[id] {
			n++
		}
	}
	return n
}

func (g *batchGroup) add(w *batchWaiter) {
	g.waiters = append(g.waiters, w)
	for _, id := range w.ids {
		if !g.seen[id] {
			g.seen[id] = true
			g.ids = append(g.ids, id)
		}
	}
}

// takeLocked claims g for one flush. It reports false if another flush has it.
func (b *resourceBatcher) takeLocked(key batchKey, g *batchGroup) bool {
	if g.flushed {
		return false
	}
	g.flushed = true
	g.timer.Stop()
	if b.groups[key] == g {
		delete(b.groups, key)
	}
	return true
}

func (b *resourceBatcher) flushDue(key batchKey, g *batchGroup) {
	b.mu.Lock()
	ok := b.takeLocked(key, g)
	b.mu.Unlock()
	if ok {
		b.flush(g)
	}
}

func (b *resourceBatcher) flush(g *batchGroup) {
	base, cancel := context.WithTimeout(context.WithoutCancel(g.ctx), batchFlushTimeout)
	defer cancel()
	ctx := client.WithPageHook(base, func() error { g.requests++; return nil })
	rows, err := g.run(ctx, g.ids)
	if err != nil {
		g.finish(nil, err)
		return
	}
	g.finish(g.split(ctx, rows), nil)
}

type batchOutcome struct {
	rows []client.CostResult
	err  error
}

// split gives each waiter the rows keyed by its ids. A waiter with no rows
// while other rows went unclaimed may have been keyed differently, so it is
// queried alone instead of being reported as having no cost data.
func (g *batchGroup) split(ctx context.Context, rows []client.CostResult) []batchOutcome {
	out := make([]batchOutcome, len(g.waiters))
	if len(g.waiters) == 1 {
		out[0].rows = rows
		return out
	}
	claimed := make([]bool, len(rows))
	for i, w := range g.waiters {
		want := make(map[string]bool, len(w.ids))
		for _, id := range w.ids {
			want[id] = true
		}
		for j, row := range rows {
			if want[row.ServiceName] {
				out[i].rows = append(out[i].rows, row)
				claimed[j] = true
			}
		}
	}
	unclaimed := false
	for _, ok := range claimed {
		if !ok {
			unclaimed = true
			break
		}
	}
	for i, w := range g.waiters {
		if len(out[i].rows) > 0 {
			continue
		}
		if !unclaimed {
			out[i].err = client.ErrNoCostData
			continue
		}
		out[i].rows, out[i].err = g.run(ctx, w.ids)
	}
	return out
}

func (g *batchGroup) finish(outcomes []batchOutcome, err error) {
	for i, w := range g.waiters {
		w.requests = g.requests
		if err != nil {
			w.err = err
		} else {
			w.rows, w.err = outcomes[i].rows, outcomes[i].err
		}
		close(w.done)
	}
}
