package harness

import (
	"errors"
	"testing"

	"github.com/leefowlercu/sigil/internal/repl"
)

func TestCompileErrorDetailParsesLocationAndSymbol(t *testing.T) {
	execErr := repl.WrapError(repl.ErrorCodeExecutionCompile, "compile failed", errors.New("_.go:7:12: undefined: missingSymbol"))
	stderr := "line-1\nline-2\nline-3\nline-4\nline-5\nline-6\nmissingSymbol()\nline-8"

	detail := compileErrorDetail(execErr, stderr)
	if detail == nil {
		t.Fatal("expected compile diagnostic detail")
	}
	if detail.Stage != "compile" {
		t.Fatalf("expected compile stage, got %q", detail.Stage)
	}
	if detail.Line == nil || *detail.Line != 7 {
		t.Fatalf("expected line 7, got %+v", detail.Line)
	}
	if detail.Column == nil || *detail.Column != 12 {
		t.Fatalf("expected column 12, got %+v", detail.Column)
	}
	if detail.Symbol == nil || *detail.Symbol != "missingSymbol" {
		t.Fatalf("expected symbol missingSymbol, got %+v", detail.Symbol)
	}
	if detail.SourceLine == nil || *detail.SourceLine != "missingSymbol()" {
		t.Fatalf("expected source_line missingSymbol(), got %+v", detail.SourceLine)
	}
}

func TestCompileErrorDetailReturnsNilForNonCompileErrors(t *testing.T) {
	execErr := repl.WrapError(repl.ErrorCodeExecutionRuntime, "runtime failed", errors.New("panic"))
	if detail := compileErrorDetail(execErr, ""); detail != nil {
		t.Fatalf("expected nil compile detail for runtime error, got %+v", detail)
	}
}
