package inference

import (
	"testing"
	"time"
)

func TestNormalizeRetryPolicyAppliesDefaults(t *testing.T) {
	normalized := NormalizeRetryPolicy(RetryPolicy{})
	if normalized.MaxAttempts != 3 {
		t.Fatalf("expected max attempts 3, got %d", normalized.MaxAttempts)
	}
	if normalized.BaseDelay != 250*time.Millisecond {
		t.Fatalf("expected base delay 250ms, got %s", normalized.BaseDelay)
	}
	if normalized.MaxDelay != 2*time.Second {
		t.Fatalf("expected max delay 2s, got %s", normalized.MaxDelay)
	}
	if normalized.Jitter == nil {
		t.Fatal("expected jitter function default")
	}
	if normalized.Sleep == nil {
		t.Fatal("expected sleep function default")
	}
}

func TestDelayForAttemptUsesExponentialBackoffWithCap(t *testing.T) {
	policy := RetryPolicy{
		BaseDelay: 250 * time.Millisecond,
		MaxDelay:  2 * time.Second,
		Jitter: func(_ int, delay time.Duration) time.Duration {
			return delay
		},
	}

	testCases := []struct {
		attempt int
		want    time.Duration
	}{
		{attempt: 1, want: 250 * time.Millisecond},
		{attempt: 2, want: 500 * time.Millisecond},
		{attempt: 3, want: time.Second},
		{attempt: 4, want: 2 * time.Second},
		{attempt: 5, want: 2 * time.Second},
	}

	for _, testCase := range testCases {
		if got := policy.DelayForAttempt(testCase.attempt); got != testCase.want {
			t.Fatalf("attempt %d: expected %s, got %s", testCase.attempt, testCase.want, got)
		}
	}
}

func TestDelayForAttemptAppliesJitterFunction(t *testing.T) {
	policy := RetryPolicy{
		BaseDelay: 250 * time.Millisecond,
		MaxDelay:  2 * time.Second,
		Jitter: func(_ int, delay time.Duration) time.Duration {
			return delay / 2
		},
	}

	if got := policy.DelayForAttempt(1); got != 125*time.Millisecond {
		t.Fatalf("expected jitter-adjusted delay 125ms, got %s", got)
	}
}
