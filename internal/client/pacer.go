package client

import (
	"context"
	"math/rand"
	"sync"
	"time"
)

const (
	pacerWindow          = time.Minute
	pacerBaseCooldown    = time.Second
	pacerMaxCooldown     = time.Minute
	pacerRecoveryQuiet   = 30 * time.Second
	pacerCooldownJitter  = 0.4
	pacerCooldownBaseMul = 0.8
)

// PacerConfig configures a Pacer. PerMinute is the request budget in a rolling
// minute, where 0 means no rate limit. MaxWait is the longest a caller may be
// queued. A request that would wait longer is refused with ErrRateLimited, and
// 0 refuses any wait.
type PacerConfig struct {
	PerMinute int
	MaxWait   time.Duration
	// Sleep waits for d or until ctx ends. Nil sleeps in real time. Tests set it.
	Sleep func(ctx context.Context, d time.Duration) error
}

// Pacer is one shared gate for Cost Explorer requests. It queues callers instead
// of failing them when the budget is spent, and it backs off for every caller at
// once after AWS throttles, so parallel callers don't retry together.
//
// The rate adapts the way TCP congestion control does: a throttle halves the
// effective limit and starts a growing cooldown, and each quiet interval adds
// one request per minute back until the configured limit returns.
type Pacer struct {
	cfg PacerConfig

	mu           sync.Mutex
	slots        []time.Time
	lastSlot     time.Time
	cooldown     time.Time
	consecutive  int
	effective    int
	lastThrottle time.Time

	now   func() time.Time
	rand  func() float64
	sleep func(context.Context, time.Duration) error
}

// NewPacer returns a Pacer for cfg.
func NewPacer(cfg PacerConfig) *Pacer {
	sleep := cfg.Sleep
	if sleep == nil {
		sleep = sleepContext
	}
	return &Pacer{
		cfg:       cfg,
		effective: cfg.PerMinute,
		now:       time.Now,
		rand:      rand.Float64,
		sleep:     sleep,
	}
}

func sleepContext(ctx context.Context, d time.Duration) error {
	timer := time.NewTimer(d)
	defer timer.Stop()
	select {
	case <-ctx.Done():
		return ctx.Err()
	case <-timer.C:
		return nil
	}
}

// EffectiveLimit is the current per-minute rate after any throttle backoff.
// It is 0 when the pacer has no rate limit.
func (p *Pacer) EffectiveLimit() int {
	p.mu.Lock()
	defer p.mu.Unlock()
	return p.effective
}

// Acquire reserves the next request slot and waits for it. It returns
// ErrRateLimited when the slot is further away than MaxWait or than the
// caller's deadline, so the caller learns that now and not after a long wait.
func (p *Pacer) Acquire(ctx context.Context) error {
	if p == nil {
		return nil
	}
	wait, err := p.reserve(ctx)
	if err != nil {
		return err
	}
	if wait <= 0 {
		return nil
	}
	return p.sleep(ctx, wait)
}

func (p *Pacer) reserve(ctx context.Context) (time.Duration, error) {
	p.mu.Lock()
	defer p.mu.Unlock()

	now := p.now()
	slot := laterOf(now, laterOf(p.cooldown, p.lastSlot))
	if p.effective > 0 {
		slot = p.slotWithinRate(slot, now)
	}
	wait := slot.Sub(now)
	if wait > 0 {
		if wait > p.cfg.MaxWait {
			return 0, ErrRateLimited
		}
		if deadline, ok := ctx.Deadline(); ok && slot.After(deadline) {
			return 0, ErrRateLimited
		}
	}
	p.lastSlot = slot
	if p.effective > 0 {
		p.slots = append(p.slots, slot)
	}
	return wait, nil
}

func (p *Pacer) slotWithinRate(slot, now time.Time) time.Time {
	kept := p.slots[:0]
	for _, s := range p.slots {
		if s.After(now.Add(-pacerWindow)) {
			kept = append(kept, s)
		}
	}
	p.slots = kept
	for {
		var inWindow []time.Time
		for _, s := range p.slots {
			if s.After(slot.Add(-pacerWindow)) && !s.After(slot) {
				inWindow = append(inWindow, s)
			}
		}
		if len(inWindow) < p.effective {
			return slot
		}
		slot = inWindow[len(inWindow)-p.effective].Add(pacerWindow)
	}
}

// Throttled records an AWS throttling response. It starts a cooldown that grows
// with each consecutive throttle and lowers the effective rate.
func (p *Pacer) Throttled() {
	if p == nil {
		return
	}
	p.mu.Lock()
	defer p.mu.Unlock()
	now := p.now()
	p.consecutive++
	delay := pacerBaseCooldown
	for i := 1; i < p.consecutive && delay < pacerMaxCooldown; i++ {
		delay *= 2
	}
	if delay > pacerMaxCooldown {
		delay = pacerMaxCooldown
	}
	delay = time.Duration(float64(delay) * (pacerCooldownBaseMul + pacerCooldownJitter*p.rand()))
	p.cooldown = laterOf(p.cooldown, now.Add(delay))
	p.lastThrottle = now
	if p.cfg.PerMinute > 0 {
		p.effective = max(1, p.effective/2)
	}
}

// Succeeded records a request AWS accepted. It ends the growing cooldown and,
// after a quiet interval, raises the effective rate by one toward the limit.
func (p *Pacer) Succeeded() {
	if p == nil {
		return
	}
	p.mu.Lock()
	defer p.mu.Unlock()
	p.consecutive = 0
	if p.cfg.PerMinute > 0 && p.effective < p.cfg.PerMinute && p.now().Sub(p.lastThrottle) >= pacerRecoveryQuiet {
		p.effective++
		p.lastThrottle = p.now()
	}
}

func laterOf(a, b time.Time) time.Time {
	if a.After(b) {
		return a
	}
	return b
}
