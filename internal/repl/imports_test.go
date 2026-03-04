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

func TestStripImportDeclsRemovesBlockImport(t *testing.T) {
	trimmed := stripImportDecls(`import (
		"fmt"
		"strings"
	)
	fmt.Println(strings.ToUpper("ok"))`)
	if strings.Contains(trimmed, "import ") {
		t.Fatalf("expected import declaration to be removed, got %q", trimmed)
	}
	if !strings.Contains(trimmed, `fmt.Println(strings.ToUpper("ok"))`) {
		t.Fatalf("expected import body to remain, got %q", trimmed)
	}
}

func TestStripImportDeclsDoesNotModifyRawStringContainingImportText(t *testing.T) {
	code := "msg := `import (\\n\\t\\\"os\\\"\\n)`\nfmt.Println(msg)"
	trimmed := stripImportDecls(code)
	if trimmed != code {
		t.Fatalf("expected code to remain unchanged, got %q", trimmed)
	}
}

func TestExtractImportsIgnoresStringLiteralImportText(t *testing.T) {
	imports, err := extractImports(`fmt.Println("import (\"os\")")`)
	if err != nil {
		t.Fatalf("expected import extraction success, got %v", err)
	}
	if len(imports) != 0 {
		t.Fatalf("expected no imports, got %v", imports)
	}
}

func TestStripImportDeclsPreservesLoopCallAssignments(t *testing.T) {
	code := `import (
	"fmt"
	"strings"
)

c := context
chunks := []string{c}
for i, ch := range chunks {
	prompt := "find token"
	ans, err := rlm_query(prompt, ch)
	if err != nil {
		fmt.Println("chunk", i, "error:", err)
	} else {
		fmt.Println(strings.TrimSpace(ans))
	}
}`

	trimmed := stripImportDecls(code)
	if !strings.Contains(trimmed, "ans, err := rlm_query(prompt, ch)") {
		t.Fatalf("expected assignment line to remain, got %q", trimmed)
	}
}

func TestStripImportDeclsPreservesAssignmentsWithRawStringRegex(t *testing.T) {
	code := `import (
  "fmt"
  "strings"
  "regexp"
)

for i := 0; i < 1; i++ {
  prompt := "find token"
  ans, err := rlm_query(prompt, context)
  if err != nil {
    fmt.Println(err)
  } else {
    a := strings.TrimSpace(ans)
    fmt.Println(a)
  }
}

re, err := regexp.Compile(` + "`" + `SIGIL-NEEDLE-2026-03-03-ALPHA-OMEGA-[0-9]{4}` + "`" + `)
if err != nil {
  fmt.Println(err)
}
_ = re`

	trimmed := stripImportDecls(code)
	if strings.Contains(trimmed, "import ") {
		t.Fatalf("expected import declaration to be removed, got %q", trimmed)
	}
	if !strings.Contains(trimmed, "ans, err := rlm_query(prompt, context)") {
		t.Fatalf("expected rlm_query assignment line to remain, got %q", trimmed)
	}
	if !strings.Contains(trimmed, "regexp.Compile(`SIGIL-NEEDLE-2026-03-03-ALPHA-OMEGA-[0-9]{4}`)") {
		t.Fatalf("expected raw string regex compile line to remain, got %q", trimmed)
	}
}
