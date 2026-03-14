package protocol

import (
	"bytes"
	"strings"
	"testing"
)

func TestGenerateTypeScriptBindingsIsDeterministic(t *testing.T) {
	first, err := GenerateTypeScriptBindings()
	if err != nil {
		t.Fatalf("expected first generation success, got %v", err)
	}
	second, err := GenerateTypeScriptBindings()
	if err != nil {
		t.Fatalf("expected second generation success, got %v", err)
	}

	if first != second {
		t.Fatal("expected deterministic TypeScript generation output")
	}
	if !strings.Contains(first, `export interface InitializeParams`) {
		t.Fatalf("expected InitializeParams interface, got %q", first)
	}
	if !strings.Contains(first, `"server/ping"`) {
		t.Fatalf("expected server/ping method mapping, got %q", first)
	}
}

func TestGenerateJSONSchemaBundleIsDeterministic(t *testing.T) {
	first, err := GenerateJSONSchemaBundle()
	if err != nil {
		t.Fatalf("expected first generation success, got %v", err)
	}
	second, err := GenerateJSONSchemaBundle()
	if err != nil {
		t.Fatalf("expected second generation success, got %v", err)
	}

	if !bytes.Equal(first, second) {
		t.Fatal("expected deterministic JSON Schema generation output")
	}
	if !bytes.Contains(first, []byte(`"initialize"`)) {
		t.Fatalf("expected initialize method definition, got %q", first)
	}
	if !bytes.Contains(first, []byte(`"$defs"`)) {
		t.Fatalf("expected schema definitions bundle, got %q", first)
	}
}
