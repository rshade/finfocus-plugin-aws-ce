package pricing

import (
	"context"
	"os"
	"strconv"
	"strings"
	"time"

	"github.com/rshade/finfocus-plugin-aws-ce/internal/client"
)

// maxRequestsEnv is the per-minute Cost Explorer page budget.
// Unset or 0 means no limit.
const maxRequestsEnv = "FINFOCUS_AWS_CE_MAX_REQUESTS_PER_MINUTE"

// maxWaitEnv is the longest, in seconds, a request may queue for the budget or
// for a throttle cooldown. 0 fails an over-budget request at once.
const maxWaitEnv = "FINFOCUS_AWS_CE_MAX_WAIT_SECONDS"

const defaultMaxWait = 90 * time.Second

// pacerSleep replaces real-time waiting in the pacer. Nil waits for real.
var pacerSleep func(context.Context, time.Duration) error

func maxWaitFromEnv() time.Duration {
	raw := strings.TrimSpace(os.Getenv(maxWaitEnv))
	if raw == "" {
		return defaultMaxWait
	}
	n, err := strconv.Atoi(raw)
	if err != nil || n < 0 {
		return defaultMaxWait
	}
	return time.Duration(n) * time.Second
}

func maxRequestsPerMinuteFromEnv() int {
	return parseMaxRequestsPerMinute(os.Getenv(maxRequestsEnv))
}

func parseMaxRequestsPerMinute(raw string) int {
	raw = strings.TrimSpace(raw)
	if raw == "" {
		return 0
	}
	n, err := strconv.Atoi(raw)
	if err != nil || n <= 0 {
		return 0
	}
	return n
}

// withCostBudget queues every Cost Explorer attempt behind the shared pacer and
// counts attempts in pages. Over budget, a request waits its turn up to the
// maximum wait instead of failing, and an AWS throttle slows every caller.
func (c *Calculator) withCostBudget(ctx context.Context, pages *int) context.Context {
	ctx = client.WithPacer(ctx, c.costPacer())
	return client.WithPageHook(ctx, func() error {
		if pages != nil {
			*pages++
		}
		return nil
	})
}

func (c *Calculator) costPacer() *client.Pacer {
	c.pacerOnce.Do(func() {
		c.pacer = client.NewPacer(client.PacerConfig{PerMinute: c.maxRequestsPerMinute, MaxWait: c.maxWait, Sleep: pacerSleep})
	})
	return c.pacer
}
