package harness

import (
	"strings"
	"testing"
)

func TestStrictTemplateRendererRenderSuccess(t *testing.T) {
	renderer := NewStrictTemplateRenderer()
	result, err := renderer.Render("hello {{.name}}", map[string]string{"name": "world"})
	if err != nil {
		t.Fatalf("expected render success, got %v", err)
	}
	if result != "hello world" {
		t.Fatalf("expected rendered output %q, got %q", "hello world", result)
	}
}

func TestStrictTemplateRendererRenderFailsOnMissingKey(t *testing.T) {
	renderer := NewStrictTemplateRenderer()
	_, err := renderer.Render("hello {{.name}}", map[string]string{})
	if err == nil {
		t.Fatal("expected render failure for missing key")
	}
	if !strings.Contains(err.Error(), "render") {
		t.Fatalf("expected render failure message, got %v", err)
	}
}

func TestResolvePromptAndContextFromTemplates(t *testing.T) {
	renderer := NewStrictTemplateRenderer()
	prompt, contextValue, err := resolvePromptAndContext(testRunConfig(
		"", "ask {{.thing}}", "", "ctx {{.thing}}",
	), map[string]string{"thing": "alpha"}, renderer)
	if err != nil {
		t.Fatalf("expected template resolution success, got %v", err)
	}
	if prompt != "ask alpha" {
		t.Fatalf("expected prompt %q, got %q", "ask alpha", prompt)
	}
	if contextValue != "ctx alpha" {
		t.Fatalf("expected context %q, got %q", "ctx alpha", contextValue)
	}
}

func TestResolvePromptAndContextUsesDirectFieldsWhenPresent(t *testing.T) {
	renderer := NewStrictTemplateRenderer()
	prompt, contextValue, err := resolvePromptAndContext(testRunConfig(
		"direct prompt", "", "direct context", "",
	), map[string]string{"thing": "alpha"}, renderer)
	if err != nil {
		t.Fatalf("expected direct field resolution success, got %v", err)
	}
	if prompt != "direct prompt" {
		t.Fatalf("expected prompt %q, got %q", "direct prompt", prompt)
	}
	if contextValue != "direct context" {
		t.Fatalf("expected context %q, got %q", "direct context", contextValue)
	}
}
