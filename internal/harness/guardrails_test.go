package harness

import (
	"testing"
	"time"

	"github.com/leefowlercu/sigil/internal/config"
)

func TestDeterministicGuardrailsCheckRunDurationTripsAtExactDeadline(t *testing.T) {
	start := time.Unix(1700000000, 0).UTC()
	guardrails := newDeterministicGuardrails(config.RunGuardrailsConfig{
		MaxStepsPerNode:            1,
		MaxTotalStepsPerRun:        1,
		MaxRunDurationMS:           15,
		MaxConsecutiveStepFailures: 1,
	}, start)

	err := guardrails.CheckRunDuration("node-id", "step-id", start.Add(15*time.Millisecond))
	if err == nil {
		t.Fatal("expected max_run_duration_ms breach at exact deadline")
	}

	limit, ok := LimitOf(err)
	if !ok {
		t.Fatalf("expected guardrail limit metadata, got %v", err)
	}
	if limit.LimitKey != limitKeyMaxRunDurationMS {
		t.Fatalf("expected limit key %q, got %q", limitKeyMaxRunDurationMS, limit.LimitKey)
	}
	if limit.ConfiguredValue != "15" {
		t.Fatalf("expected configured_value 15, got %q", limit.ConfiguredValue)
	}
	if limit.ObservedValue != "15" {
		t.Fatalf("expected observed_value 15, got %q", limit.ObservedValue)
	}
}
