package harness

import (
	"crypto/sha256"
	"encoding/hex"
	"strings"
	"testing"

	"github.com/leefowlercu/sigil/internal/runtime"
)

func TestBuildContextMetadataReturnsDeterministicIdentity(t *testing.T) {
	raw := "line-1\nline-2"
	contextRef := "run-output://node/test/context.json"

	metadata := buildContextMetadata(raw, contextRef)
	sum := sha256.Sum256([]byte(raw))

	if metadata.ContextType != "string" {
		t.Fatalf("expected context_type=string, got %q", metadata.ContextType)
	}
	if metadata.ContextBytes != len(raw) {
		t.Fatalf("expected context_bytes=%d, got %d", len(raw), metadata.ContextBytes)
	}
	if metadata.ContextLineCount != 2 {
		t.Fatalf("expected context_line_count=2, got %d", metadata.ContextLineCount)
	}
	if metadata.ContextSHA256 != hex.EncodeToString(sum[:]) {
		t.Fatalf("expected context_sha256=%q, got %q", hex.EncodeToString(sum[:]), metadata.ContextSHA256)
	}
	if metadata.ContextRef != contextRef {
		t.Fatalf("expected context_ref=%q, got %q", contextRef, metadata.ContextRef)
	}
}

func TestBuildPreviousActionFeedbackCapsStdoutAndStderrPreviews(t *testing.T) {
	baseDir := t.TempDir()
	store, err := NewActionArtifactStore(baseDir)
	if err != nil {
		t.Fatalf("expected artifact store creation success, got %v", err)
	}

	runID := mustUUIDv7String(t)
	nodeID := mustUUIDv7String(t)
	stepID := mustUUIDv7String(t)
	errorCode := "repl_execution_runtime"
	errorMessage := "execution failed"
	artifact := ActionArtifact{
		RunID:        runID,
		NodeID:       nodeID,
		StepID:       stepID,
		ActionIndex:  1,
		ActionType:   "repl_code",
		Language:     "go",
		Status:       string(runtime.ActionExecutionStatusFailed),
		Code:         `fmt.Println("hello")`,
		Stdout:       strings.Repeat("o", stepInputPreviewCapBytes+32),
		Stderr:       strings.Repeat("e", stepInputPreviewCapBytes+16),
		DurationMS:   5,
		ErrorCode:    &errorCode,
		ErrorMessage: &errorMessage,
		ErrorDetail: &ActionErrorDetail{
			Stage:   "compile",
			Message: "undefined: missing",
			Line:    intPointer(3),
			Column:  intPointer(4),
		},
	}
	outputRef, err := store.Persist(artifact)
	if err != nil {
		t.Fatalf("expected action artifact persist success, got %v", err)
	}

	feedback, err := buildPreviousActionFeedback(runID, store, runtime.NodeActionExecutedPayload{
		StepID:       stepID,
		ActionIndex:  1,
		ActionType:   "repl_code",
		Language:     "go",
		Status:       runtime.ActionExecutionStatusFailed,
		DurationMS:   5,
		OutputRef:    outputRef,
		ErrorCode:    &errorCode,
		ErrorMessage: &errorMessage,
	})
	if err != nil {
		t.Fatalf("expected previous-action feedback build success, got %v", err)
	}

	if feedback.OutputRef != outputRef {
		t.Fatalf("expected feedback output_ref %q, got %q", outputRef, feedback.OutputRef)
	}
	if !feedback.StdoutTruncated {
		t.Fatalf("expected stdout_truncated=true")
	}
	if !feedback.StderrTruncated {
		t.Fatalf("expected stderr_truncated=true")
	}
	if feedback.StdoutBytes != stepInputPreviewCapBytes+32 {
		t.Fatalf("expected stdout_bytes=%d, got %d", stepInputPreviewCapBytes+32, feedback.StdoutBytes)
	}
	if feedback.StderrBytes != stepInputPreviewCapBytes+16 {
		t.Fatalf("expected stderr_bytes=%d, got %d", stepInputPreviewCapBytes+16, feedback.StderrBytes)
	}
	if len(feedback.StdoutPreview) != stepInputPreviewCapBytes {
		t.Fatalf("expected stdout preview size=%d, got %d", stepInputPreviewCapBytes, len(feedback.StdoutPreview))
	}
	if len(feedback.StderrPreview) != stepInputPreviewCapBytes {
		t.Fatalf("expected stderr preview size=%d, got %d", stepInputPreviewCapBytes, len(feedback.StderrPreview))
	}
	if feedback.ErrorDetail == nil {
		t.Fatal("expected compile error_detail to be propagated")
	}
	if feedback.ErrorDetail.Stage != "compile" {
		t.Fatalf("expected compile error stage, got %q", feedback.ErrorDetail.Stage)
	}
	if feedback.ErrorDetail.Line == nil || *feedback.ErrorDetail.Line != 3 {
		t.Fatalf("expected line=3, got %+v", feedback.ErrorDetail.Line)
	}
}

func intPointer(value int) *int {
	copied := value
	return &copied
}
