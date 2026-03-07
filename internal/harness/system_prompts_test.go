package harness

import (
	"strings"
	"testing"
)

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
	if !strings.Contains(prompt, `"decision"`) {
		t.Fatalf("expected prompt schema block to include decision field, got %q", prompt)
	}
	if !strings.Contains(prompt, `"expected_observation"`) {
		t.Fatalf("expected prompt schema block to include expected_observation field, got %q", prompt)
	}
	if !strings.Contains(prompt, `"evidence"`) {
		t.Fatalf("expected prompt schema block to include evidence field, got %q", prompt)
	}
	if !strings.Contains(prompt, "copy it byte-for-byte") {
		t.Fatalf("expected prompt to instruct verbatim action output_ref copying, got %q", prompt)
	}
}

func TestResolveBaseFailsWhenSchemaRegistryMissing(t *testing.T) {
	resolver := NewSystemPromptResolver()
	resolver.schemaRegistry = nil

	if _, _, err := resolver.ResolveBase("openai"); err == nil {
		t.Fatal("expected schema-registry resolution error")
	}
}
