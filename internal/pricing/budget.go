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

func (c *Calculator) withCostBudget(ctx context.Context, pages *int) context.Context {
	return client.WithPageHook(ctx, func() error {
		if err := c.allowCostExplorerRequest(time.Now()); err != nil {
			return err
		}
		if pages != nil {
			*pages++
		}
		return nil
	})
}

func (c *Calculator) allowCostExplorerRequest(now time.Time) error {
	if c == nil || c.maxRequestsPerMinute <= 0 {
		return nil
	}
	c.requestMu.Lock()
	defer c.requestMu.Unlock()

	cutoff := now.Add(-time.Minute)
	kept := make([]time.Time, 0, len(c.requestTimes)+1)
	for _, ts := range c.requestTimes {
		if ts.After(cutoff) {
			kept = append(kept, ts)
		}
	}
	if len(kept) >= c.maxRequestsPerMinute {
		c.requestTimes = kept
		return client.ErrRateLimited
	}
	c.requestTimes = append(kept, now)
	return nil
}
