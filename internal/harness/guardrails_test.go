package harness

import (
	"errors"
	"strings"
	"testing"
	"time"

	"github.com/leefowlercu/sigil/internal/config"
)

func TestDeterministicGuardrailsCheckRunDurationTripsAtExactDeadline(t *testing.T) {
	start := time.Unix(1700000000, 0).UTC()
	guardrails, err := newDeterministicGuardrails(config.RunGuardrailsConfig{
		MaxStepsPerNode:            1,
		MaxTotalStepsPerRun:        1,
		MaxRunDurationMS:           15,
		MaxConsecutiveStepFailures: 1,
	}, start)
	if err != nil {
		t.Fatalf("expected guardrail construction success, got %v", err)
	}

	err = guardrails.CheckRunDuration("node-id", "step-id", start.Add(15*time.Millisecond))
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

func TestDeterministicGuardrailsCheckBeforeStepClarifiesAttemptedStepStart(t *testing.T) {
	guardrails, err := newDeterministicGuardrails(config.RunGuardrailsConfig{
		MaxStepsPerNode:            3,
		MaxTotalStepsPerRun:        10,
		MaxRunDurationMS:           1000,
		MaxConsecutiveStepFailures: 1,
	}, time.Unix(1700000000, 0).UTC())
	if err != nil {
		t.Fatalf("expected guardrail construction success, got %v", err)
	}

	for i := 0; i < 3; i++ {
		guardrails.RecordStepStarted("node-id")
	}

	err = guardrails.CheckBeforeStep("node-id", time.Unix(1700000000, 0).UTC())
	if err == nil {
		t.Fatal("expected max_steps_per_node breach after three started steps")
	}

	limit, ok := LimitOf(err)
	if !ok {
		t.Fatalf("expected guardrail limit metadata, got %v", err)
	}
	if limit.LimitKey != limitKeyMaxStepsPerNode {
		t.Fatalf("expected limit key %q, got %q", limitKeyMaxStepsPerNode, limit.LimitKey)
	}
	if limit.ObservedValue != "3" {
		t.Fatalf("expected observed_value 3, got %q", limit.ObservedValue)
	}

	var typed *Error
	if !errors.As(err, &typed) {
		t.Fatalf("expected typed harness error, got %T", err)
	}
	if !strings.Contains(typed.Message, "attempted=4") {
		t.Fatalf("expected attempted next-step count in message, got %q", typed.Message)
	}
	if !strings.Contains(typed.Message, "while blocking a new step start") {
		t.Fatalf("expected step-start context in message, got %q", typed.Message)
	}
}

func TestDeterministicGuardrailsRejectsMalformedCostBudget(t *testing.T) {
	value := "1.2345678"
	_, err := newDeterministicGuardrails(config.RunGuardrailsConfig{
		MaxStepsPerNode:            1,
		MaxTotalStepsPerRun:        1,
		MaxRunDurationMS:           1000,
		MaxConsecutiveStepFailures: 1,
		MaxTotalCostUSD:            &value,
	}, time.Unix(1700000000, 0).UTC())
	if err == nil {
		t.Fatal("expected malformed guardrail cost budget to fail construction")
	}
}
