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

func TestParseDecisionPayloadRejectsFinalBranch(t *testing.T) {
	payload := map[string]any{
		"decision": "final",
		"final": map[string]any{
			"answer": "done",
			"evidence": []any{
				map[string]any{
					"ref":        "run-artifact://node/example/context.json",
					"chunk_id":   "chunk-1",
					"span_start": 0,
					"span_end":   42,
				},
			},
			"confidence": "high",
		},
	}

	if _, err := parseDecisionPayload(payload); err == nil {
		t.Fatal("expected final branch rejection")
	}
}

func TestParseDecisionPayloadRejectsDirectFinalBeforeEvidenceParsing(t *testing.T) {
	payload := map[string]any{
		"decision": "final",
		"final": map[string]any{
			"answer": "done",
			"evidence": []any{
				map[string]any{
					"ref":        "run-artifact://node/example/context.json",
					"span_start": 20,
					"span_end":   10,
				},
			},
		},
	}

	if _, err := parseDecisionPayload(payload); err == nil {
		t.Fatal("expected parse failure for direct final decision")
	}
}
