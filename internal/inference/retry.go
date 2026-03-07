package inference

import (
	"context"
	"math"
	"math/rand"
	"sync"
	"time"
)

const (
	defaultMaxAttempts = 3
)

var (
	jitterMu sync.Mutex
	jitterR  = rand.New(rand.NewSource(time.Now().UnixNano()))
)

// RetryPolicy controls transient retry behavior.
type RetryPolicy struct {
	MaxAttempts int
	BaseDelay   time.Duration
	MaxDelay    time.Duration
	Jitter      func(attempt int, delay time.Duration) time.Duration
	Sleep       func(ctx context.Context, delay time.Duration) error
}

// DefaultRetryPolicy returns PRD-0300 retry defaults.
func DefaultRetryPolicy() RetryPolicy {
	return RetryPolicy{
		MaxAttempts: defaultMaxAttempts,
		BaseDelay:   250 * time.Millisecond,
		MaxDelay:    2 * time.Second,
		Jitter:      defaultJitter,
		Sleep:       sleepWithContext,
	}
}

// NormalizeRetryPolicy applies defaults for zero-value policy fields.
func NormalizeRetryPolicy(policy RetryPolicy) RetryPolicy {
	defaults := DefaultRetryPolicy()
	normalized := policy

	if normalized.MaxAttempts <= 0 {
		normalized.MaxAttempts = defaults.MaxAttempts
	}
	if normalized.BaseDelay <= 0 {
		normalized.BaseDelay = defaults.BaseDelay
	}
	if normalized.MaxDelay <= 0 {
		normalized.MaxDelay = defaults.MaxDelay
	}
	if normalized.Jitter == nil {
		normalized.Jitter = defaults.Jitter
	}
	if normalized.Sleep == nil {
		normalized.Sleep = defaults.Sleep
	}

	return normalized
}

// DelayForAttempt returns the retry backoff delay for a failed attempt number.
// The attempt value is 1-based and corresponds to the failed attempt index.
func (p RetryPolicy) DelayForAttempt(attempt int) time.Duration {
	if attempt <= 0 {
		attempt = 1
	}

	base := p.BaseDelay
	maxDelay := p.MaxDelay
	if base <= 0 {
		base = 250 * time.Millisecond
	}
	if maxDelay <= 0 {
		maxDelay = 2 * time.Second
	}

	exponent := float64(attempt - 1)
	delayFloat := float64(base) * math.Pow(2, exponent)
	if delayFloat > float64(maxDelay) {
		delayFloat = float64(maxDelay)
	}

	delay := time.Duration(delayFloat)
	if p.Jitter != nil {
		delay = p.Jitter(attempt, delay)
	}
	if delay < 0 {
		delay = 0
	}
	if delay > maxDelay {
		delay = maxDelay
	}

	return delay
}

func defaultJitter(_ int, delay time.Duration) time.Duration {
	jitterMu.Lock()
	factor := 0.5 + jitterR.Float64()
	jitterMu.Unlock()

	return time.Duration(float64(delay) * factor)
}

func sleepWithContext(ctx context.Context, delay time.Duration) error {
	if delay <= 0 {
		return nil
	}

	timer := time.NewTimer(delay)
	defer timer.Stop()

	select {
	case <-ctx.Done():
		return ctx.Err()
	case <-timer.C:
		return nil
	}
}
