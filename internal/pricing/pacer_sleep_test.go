package pricing

import (
	"context"
	"time"
)

// Backoff waits are real time in production. Tests that provoke throttling
// would otherwise sit through them, so skip the sleep.
func init() {
	pacerSleep = func(context.Context, time.Duration) error { return nil }
}
