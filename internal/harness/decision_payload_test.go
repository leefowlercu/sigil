package harness

import "testing"

func TestParseDecisionPayloadContinueBranch(t *testing.T) {
	payload := map[string]any{
		"decision": "continue",
		"continuation": map[string]any{
			"repl_code":            `fmt.Print("ok")`,
			"intent":               "inspect context chunk",
			"expected_observation": "needle appears in output",
		},
	}

	parsed, err := parseDecisionPayload(payload)
	if err != nil {
		t.Fatalf("expected parse success, got %v", err)
	}
	if parsed.Decision != "continue" {
		t.Fatalf("expected continue decision, got %q", parsed.Decision)
	}
	if parsed.Continuation == nil {
		t.Fatal("expected continuation payload")
	}
	if parsed.Continuation.ReplCode != `fmt.Print("ok")` {
		t.Fatalf("expected repl_code to round-trip, got %q", parsed.Continuation.ReplCode)
	}
}

func TestParseDecisionPayloadFinalBranch(t *testing.T) {
	payload := map[string]any{
		"decision": "final",
		"final": map[string]any{
			"answer": "done",
			"evidence": []any{
				map[string]any{
					"ref":        "run-output://node/example/context.json",
					"chunk_id":   "chunk-1",
					"span_start": 0,
					"span_end":   42,
				},
			},
			"confidence": "high",
		},
	}

	parsed, err := parseDecisionPayload(payload)
	if err != nil {
		t.Fatalf("expected parse success, got %v", err)
	}
	if parsed.Decision != "final" {
		t.Fatalf("expected final decision, got %q", parsed.Decision)
	}
	if parsed.Final == nil {
		t.Fatal("expected final payload")
	}
	if parsed.Final.Answer != "done" {
		t.Fatalf("expected answer done, got %q", parsed.Final.Answer)
	}
	if len(parsed.Final.Evidence) != 1 {
		t.Fatalf("expected one evidence item, got %d", len(parsed.Final.Evidence))
	}
	if parsed.Final.Confidence == nil || *parsed.Final.Confidence != "high" {
		t.Fatalf("expected confidence high, got %v", parsed.Final.Confidence)
	}
}

func TestParseDecisionPayloadRejectsMalformedEvidenceSpan(t *testing.T) {
	payload := map[string]any{
		"decision": "final",
		"final": map[string]any{
			"answer": "done",
			"evidence": []any{
				map[string]any{
					"ref":        "run-output://node/example/context.json",
					"span_start": 20,
					"span_end":   10,
				},
			},
		},
	}

	if _, err := parseDecisionPayload(payload); err == nil {
		t.Fatal("expected parse failure for malformed span range")
	}
}
