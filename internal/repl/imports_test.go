package repl

import (
	"strings"
	"testing"
)

func TestExtractImportsIncludesAliasedSingleImport(t *testing.T) {
	imports, err := extractImports(`import o "os"; var sep = o.PathSeparator`)
	if err != nil {
		t.Fatalf("expected import extraction success, got %v", err)
	}
	if len(imports) != 1 || imports[0] != "os" {
		t.Fatalf("expected imports [os], got %v", imports)
	}
}

func TestExtractImportsIncludesDotAndBlankSingleImports(t *testing.T) {
	imports, err := extractImports("import . \"fmt\"\nimport _ \"os/exec\"")
	if err != nil {
		t.Fatalf("expected import extraction success, got %v", err)
	}
	if len(imports) != 2 {
		t.Fatalf("expected 2 imports, got %d (%v)", len(imports), imports)
	}
	if imports[0] != "fmt" || imports[1] != "os/exec" {
		t.Fatalf("expected imports [fmt os/exec], got %v", imports)
	}
}

func TestStripImportDeclsRemovesAliasedSingleImport(t *testing.T) {
	trimmed := stripImportDecls(`import f "fmt"; f.Print("ok")`)
	if strings.Contains(trimmed, "import ") {
		t.Fatalf("expected import declaration to be removed, got %q", trimmed)
	}
	if trimmed != `f.Print("ok")` {
		t.Fatalf("expected stripped body %q, got %q", `f.Print("ok")`, trimmed)
	}
}
