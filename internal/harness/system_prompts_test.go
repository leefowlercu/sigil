package harness

import (
	"strings"
	"testing"
)

func assertContainsAll(t *testing.T, haystack string, needles ...string) {
	t.Helper()
	for _, needle := range needles {
		if !strings.Contains(haystack, needle) {
			t.Fatalf("expected prompt to include %q, got %q", needle, haystack)
		}
	}
}

func TestResolveBaseSelectsMappedProviderPrompt(t *testing.T) {
	resolver := NewSystemPromptResolver()

	provider, prompt, err := resolver.ResolveBase("openai")
	if err != nil {
		t.Fatalf("expected openai base prompt resolution success, got %v", err)
	}
	if provider != ProviderOpenAI {
		t.Fatalf("expected provider %q, got %q", ProviderOpenAI, provider)
	}
	if strings.TrimSpace(prompt) == "" {
		t.Fatal("expected non-empty openai base prompt")
	}

	provider, prompt, err = resolver.ResolveBase("anthropic")
	if err != nil {
		t.Fatalf("expected anthropic base prompt resolution success, got %v", err)
	}
	if provider != ProviderAnthropic {
		t.Fatalf("expected provider %q, got %q", ProviderAnthropic, provider)
	}
	if strings.TrimSpace(prompt) == "" {
		t.Fatal("expected non-empty anthropic base prompt")
	}
}

func TestResolveBaseFallsBackToOpenAIForUnknownProvider(t *testing.T) {
	resolver := NewSystemPromptResolver()

	provider, prompt, err := resolver.ResolveBase("provider-x")
	if err != nil {
		t.Fatalf("expected fallback base prompt resolution success, got %v", err)
	}
	if provider != ProviderOpenAI {
		t.Fatalf("expected fallback provider %q, got %q", ProviderOpenAI, provider)
	}
	if strings.TrimSpace(prompt) == "" {
		t.Fatal("expected non-empty fallback base prompt")
	}
}

func TestResolveEffectiveComposesSystemPromptAppend(t *testing.T) {
	resolver := NewSystemPromptResolver()

	_, basePrompt, err := resolver.ResolveBase("openai")
	if err != nil {
		t.Fatalf("expected openai base prompt resolution success, got %v", err)
	}
	_, effective, err := resolver.ResolveEffective("openai", "Use concise tool calls.")
	if err != nil {
		t.Fatalf("expected effective prompt resolution success, got %v", err)
	}
	expected := basePrompt + "\n\n" + "Use concise tool calls."
	if effective != expected {
		t.Fatalf("expected effective prompt %q, got %q", expected, effective)
	}
}

func TestResolveEffectiveReturnsBaseWhenAppendIsWhitespace(t *testing.T) {
	resolver := NewSystemPromptResolver()

	_, basePrompt, err := resolver.ResolveBase("anthropic")
	if err != nil {
		t.Fatalf("expected anthropic base prompt resolution success, got %v", err)
	}
	_, effective, err := resolver.ResolveEffective("anthropic", "   ")
	if err != nil {
		t.Fatalf("expected effective prompt resolution success, got %v", err)
	}
	if effective != basePrompt {
		t.Fatalf("expected base prompt %q, got %q", basePrompt, effective)
	}
}

func TestResolveBaseRendersSchemaFromRegistry(t *testing.T) {
	resolver := NewSystemPromptResolver()

	_, prompt, err := resolver.ResolveBase("openai")
	if err != nil {
		t.Fatalf("expected prompt render success, got %v", err)
	}
	assertContainsAll(t, prompt, `"decision"`, `"expected_observation"`, `"evidence"`, "copy it byte-for-byte")
}

func TestResolveBaseFailsWhenSchemaRegistryMissing(t *testing.T) {
	resolver := NewSystemPromptResolver()
	resolver.schemaRegistry = nil

	if _, _, err := resolver.ResolveBase("openai"); err == nil {
		t.Fatal("expected schema-registry resolution error")
	}
}

func TestResolveBaseProviderPromptsIntentionallyDiverge(t *testing.T) {
	resolver := NewSystemPromptResolver()

	_, openAIPrompt, err := resolver.ResolveBase("openai")
	if err != nil {
		t.Fatalf("expected openai prompt resolution success, got %v", err)
	}
	_, anthropicPrompt, err := resolver.ResolveBase("anthropic")
	if err != nil {
		t.Fatalf("expected anthropic prompt resolution success, got %v", err)
	}
	if openAIPrompt == anthropicPrompt {
		t.Fatal("expected provider prompts to diverge")
	}

	assertContainsAll(t, openAIPrompt,
		"<tool_selection>",
		"<retrieval_strategy>",
		"<citation_rules>",
		"<finalization_gate>",
		"<recovery_rules>",
		"decision=final is allowed only when all of the following are true:",
	)
	if strings.Contains(anthropicPrompt, "<tool_selection>") {
		t.Fatalf("expected anthropic prompt to remain simpler, got %q", anthropicPrompt)
	}
	assertContainsAll(t, anthropicPrompt,
		"Evidence rules:",
		"Finalization gate:",
		"decision=final is allowed only when the requested deliverable is obtained",
	)
}

func TestResolveBaseOpenAIPromptIncludesPromptRegressionShields(t *testing.T) {
	resolver := NewSystemPromptResolver()

	_, prompt, err := resolver.ResolveBase("openai")
	if err != nil {
		t.Fatalf("expected openai prompt resolution success, got %v", err)
	}

	assertContainsAll(t, prompt,
		"Each continue action may perform at most 4 recursive subcalls and at most 8 total subcalls.",
		"If more expansion is needed, finish the current action, record what narrowed successfully, and use a new step before expanding again.",
		"If execution_state.small_context=true, solve locally with REPL or llm_query and do not call rlm_query or rlm_query_batched.",
		"If execution_state.recursive_subcalls_allowed=false, stay local for this step even if recursive APIs are available.",
		"If stdout_preview or stderr_preview is truncated, treat the preview as partial evidence only.",
		"If an action times out or previous_action_feedback.error_message indicates timeout, reduce chunk size and fan-out on the next step",
		"If a complete local scan of the current context finds no matching evidence, finalize absence now rather than repartitioning the same context again.",
		"include span_start or span_end only when you know exact integer offsets",
		`{"ref":"run-artifact://node/019cc5fc-b991-7b33-bb66-c4e2508378f8/step/019cc5fc-b99b-7b33-bb66-c4e2508378f8/action-1.json"}`,
		`{"decision":"continue","continuation":`,
		`{"decision":"final","final":`,
		`{"decision":"final","final":{"answer":"NONE"`,
	)
}

func TestResolveBaseProviderPromptsExplainPlainSubcallAnswerStringContract(t *testing.T) {
	resolver := NewSystemPromptResolver()

	for _, provider := range []string{"openai", "anthropic"} {
		_, prompt, err := resolver.ResolveBase(provider)
		if err != nil {
			t.Fatalf("expected %s prompt resolution success, got %v", provider, err)
		}

		assertContainsAll(t, prompt,
			"llm_query and rlm_query return a plain string answer to your Go code, not an arbitrary top-level JSON object.",
			`The harness`,
			`{"has_token":true,"token":"...","line":"..."}`,
			"minified JSON text inside the answer string",
		)
	}
}

func TestResolveBaseOpenAIPromptIncludesCompileSafeStructuredPromptGuidance(t *testing.T) {
	resolver := NewSystemPromptResolver()

	_, prompt, err := resolver.ResolveBase("openai")
	if err != nil {
		t.Fatalf("expected openai prompt resolution success, got %v", err)
	}

	assertContainsAll(t, prompt,
		"If a prompt string needs literal JSON examples or many embedded quotes, prefer a raw string literal with backquotes.",
		"Prefer describing required JSON keys in words over embedding heavily escaped JSON examples inside double-quoted Go strings.",
		`Do NOT over-escape prompt strings with sequences like {\\"has_token\\":true} inside double-quoted Go code.`,
		"For structured parsing from map[string]any, prefer predeclared variables plus assignment over compact two-value short declarations.",
		"At REPL top level, do NOT introduce ok/present/type flags with := and then reference them in later statements.",
		`hasRaw := any(nil)`,
		`hasRaw, present = parsed["has_token"]`,
		`hasTokenBool, typeOK = hasRaw.(bool)`,
	)
}

func TestPlainSubcallSystemPromptRequiresGroundedTerseAnswer(t *testing.T) {
	assertContainsAll(t, plainSubcallSystemPrompt,
		"Use only the provided prompt and context.",
		"Keep the answer terse, grounded, and non-speculative.",
		"Return exactly one strict JSON object with key answer and no extra keys.",
		"place that structure as minified JSON text inside the answer string instead of adding top-level keys.",
		`Valid example: {"answer":"{\"has_token\":false}"}.`,
		`Invalid example: {"has_token":false}.`,
		`Invalid example: {"answer":{"has_token":false}}.`,
	)
}
