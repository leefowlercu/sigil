package harness

import (
	"context"
	"encoding/json"
	"errors"
	"os"
	"path/filepath"
	"strconv"
	"strings"
	"sync"
	"testing"
	"time"

	"github.com/leefowlercu/sigil/internal/accounting"
	"github.com/leefowlercu/sigil/internal/config"
	"github.com/leefowlercu/sigil/internal/inference"
	"github.com/leefowlercu/sigil/internal/inference/schema"
	"github.com/leefowlercu/sigil/internal/repl"
	"github.com/leefowlercu/sigil/internal/runtime"
)

type queuedInference struct {
	responses []queuedInferenceResponse
	calls     int
	requests  []inference.Request
}

type queuedInferenceResponse struct {
	result inference.Result
	err    error
}

func (q *queuedInference) Infer(_ context.Context, request inference.Request) (inference.Result, error) {
	if q.calls >= len(q.responses) {
		return inference.Result{}, errors.New("unexpected inference call")
	}

	q.requests = append(q.requests, request)
	response := q.responses[q.calls]
	q.calls++
	return hydrateFinalEvidenceRef(response.result, request), response.err
}

type subcallAwareInference struct {
	mu       sync.Mutex
	root     []queuedInferenceResponse
	rootCall int
	requests []inference.Request
}

type recursiveTimeoutInference struct {
	mu               sync.Mutex
	callCount        int
	childHasDeadline bool
	childDeadline    time.Duration
}

type recursiveLevelTimeoutInference struct {
	mu             sync.Mutex
	callCount      int
	depth1Deadline time.Duration
	depth2Deadline time.Duration
}

type childFailureInference struct {
	mu       sync.Mutex
	call     int
	requests []inference.Request
}

type totalStepsGuardrailInference struct {
	mu   sync.Mutex
	call int
}

func (s *subcallAwareInference) Infer(_ context.Context, request inference.Request) (inference.Result, error) {
	s.mu.Lock()
	defer s.mu.Unlock()

	s.requests = append(s.requests, request)
	if request.SchemaID == schema.SigilLLMAnswerV1SchemaID {
		if len(request.Messages) < 2 {
			return inference.Result{}, errors.New("missing user message for subcall")
		}
		var payload map[string]string
		if err := json.Unmarshal([]byte(request.Messages[1].Content), &payload); err != nil {
			return inference.Result{}, err
		}
		answer := payload["prompt"] + "|" + payload["context"]
		return inference.Result{
			SchemaID:          schema.SigilLLMAnswerV1SchemaID,
			ValidatedPayload:  map[string]any{"answer": answer},
			Gateway:           "openrouter",
			Provider:          request.Provider,
			Model:             request.Model,
			GatewayResponseID: "resp_subcall",
			FinishStatus:      "completed",
			RawMetadata:       map[string]any{},
		}, nil
	}

	if s.rootCall >= len(s.root) {
		return inference.Result{}, errors.New("unexpected root inference call")
	}
	response := s.root[s.rootCall]
	s.rootCall++
	return hydrateFinalEvidenceRef(response.result, request), response.err
}

func (s *recursiveTimeoutInference) Infer(ctx context.Context, request inference.Request) (inference.Result, error) {
	s.mu.Lock()
	s.callCount++
	call := s.callCount
	s.mu.Unlock()

	switch call {
	case 1:
		return hydrateFinalEvidenceRef(continueResult(`import "fmt"; _, err := rlm_query("child prompt", "child context"); if err != nil { fmt.Print(err.Error()) }`), request), nil
	case 2:
		if deadline, ok := ctx.Deadline(); ok {
			s.mu.Lock()
			s.childHasDeadline = true
			s.childDeadline = time.Until(deadline)
			s.mu.Unlock()
		}
		<-ctx.Done()
		return inference.Result{}, ctx.Err()
	case 3:
		return hydrateFinalEvidenceRef(finalResult("done"), request), nil
	default:
		return inference.Result{}, errors.New("unexpected inference call")
	}
}

func (s *recursiveLevelTimeoutInference) Infer(ctx context.Context, request inference.Request) (inference.Result, error) {
	s.mu.Lock()
	s.callCount++
	call := s.callCount
	s.mu.Unlock()

	switch call {
	case 1:
		return hydrateFinalEvidenceRef(continueResult(`import "fmt"; _, err := rlm_query("depth1 prompt", "depth1 context"); if err != nil { fmt.Print(err.Error()) }`), request), nil
	case 2:
		if deadline, ok := ctx.Deadline(); ok {
			s.mu.Lock()
			s.depth1Deadline = time.Until(deadline)
			s.mu.Unlock()
		}
		return hydrateFinalEvidenceRef(continueResult(`import "fmt"; import "time"; time.Sleep(220 * time.Millisecond); _, err := rlm_query("depth2 prompt", "depth2 context"); if err != nil { fmt.Print(err.Error()) }`), request), nil
	case 3:
		if deadline, ok := ctx.Deadline(); ok {
			s.mu.Lock()
			s.depth2Deadline = time.Until(deadline)
			s.mu.Unlock()
		}
		return hydrateFinalEvidenceRef(finalResult("depth2 final"), request), nil
	case 4:
		return hydrateFinalEvidenceRef(finalResult("depth1 final"), request), nil
	case 5:
		return hydrateFinalEvidenceRef(finalResult("root final"), request), nil
	default:
		return inference.Result{}, errors.New("unexpected inference call")
	}
}

func (s *childFailureInference) Infer(_ context.Context, request inference.Request) (inference.Result, error) {
	s.mu.Lock()
	defer s.mu.Unlock()

	s.call++
	s.requests = append(s.requests, request)
	switch s.call {
	case 1:
		return hydrateFinalEvidenceRef(continueResult(`import "fmt"; _, err := rlm_query("child prompt", "child context"); if err != nil { fmt.Print(err.Error()) }`), request), nil
	case 2:
		return inference.Result{}, inference.NewError(inference.ErrorCodeGatewayFailure, "child inference failure")
	case 3:
		return hydrateFinalEvidenceRef(finalResult("done"), request), nil
	default:
		return inference.Result{}, errors.New("unexpected inference call")
	}
}

func (s *totalStepsGuardrailInference) Infer(_ context.Context, request inference.Request) (inference.Result, error) {
	s.mu.Lock()
	defer s.mu.Unlock()

	s.call++
	switch s.call {
	case 1:
		return hydrateFinalEvidenceRef(continueResult(`import "fmt"; _, err := rlm_query("child prompt", "child context"); if err != nil { fmt.Print(err.Error()) }`), request), nil
	default:
		return hydrateFinalEvidenceRef(continueResult(`import "fmt"; fmt.Print("child step")`), request), nil
	}
}

func hydrateFinalEvidenceRef(result inference.Result, request inference.Request) inference.Result {
	if result.SchemaID != schema.SigilRLMResponseV1SchemaID {
		return result
	}
	finalPayload, ok := result.ValidatedPayload["final"].(map[string]any)
	if !ok {
		return result
	}
	evidenceRaw, ok := finalPayload["evidence"].([]any)
	if !ok || len(evidenceRaw) == 0 {
		return result
	}

	if len(request.Messages) < 2 {
		return result
	}
	var envelope StepInputEnvelope
	if err := json.Unmarshal([]byte(request.Messages[1].Content), &envelope); err != nil {
		return result
	}

	for _, rawItem := range evidenceRaw {
		evidenceItem, ok := rawItem.(map[string]any)
		if !ok {
			continue
		}
		refValue, _ := evidenceItem["ref"].(string)
		switch refValue {
		case "__context_ref__":
			if strings.TrimSpace(envelope.ContextMetadata.ContextRef) == "" {
				continue
			}
			evidenceItem["ref"] = envelope.ContextMetadata.ContextRef
		case "__previous_output_ref__":
			if envelope.PreviousActionFeedback == nil || strings.TrimSpace(envelope.PreviousActionFeedback.OutputRef) == "" {
				continue
			}
			evidenceItem["ref"] = envelope.PreviousActionFeedback.OutputRef
		case "__previous_output_ref_malformed__":
			if envelope.PreviousActionFeedback == nil || strings.TrimSpace(envelope.PreviousActionFeedback.OutputRef) == "" {
				continue
			}
			parsedRef, err := runtime.ParseActionOutputRef(envelope.PreviousActionFeedback.OutputRef)
			if err != nil {
				continue
			}
			hybridNodeID, ok := buildMalformedHybridActionRefNodeID(parsedRef.NodeID, parsedRef.StepID)
			if !ok {
				continue
			}
			evidenceItem["ref"] = "run-artifact://node/" + hybridNodeID + "/action-" + strconv.Itoa(parsedRef.ActionIndex) + ".json"
		}
	}
	return result
}

func TestRunnerRunRecursiveFlow(t *testing.T) {
	baseDir := filepath.Join(t.TempDir(), "sigil-runs")
	inferenceClient := &queuedInference{
		responses: []queuedInferenceResponse{
			{result: continueResult(`import "fmt"; answer, err := rlm_query("child prompt", "child context"); if err != nil { panic(err) }; fmt.Print(answer)`)},
			{result: finalResult("child final")},
			{result: finalResult("root final")},
		},
	}

	runner := NewRunner(
		WithRunsBaseDir(baseDir),
		WithInferenceFactory(func(_ config.RunConfig) (InferenceClient, error) {
			return inferenceClient, nil
		}),
	)

	result, err := runner.Run(context.Background(), RunInput{
		AppConfigPath: "./sigil.yaml",
		RunConfigPath: "./sigil-run.yaml",
		RunConfig:     testRunConfig("root prompt", "", "root context", ""),
		TemplateVars:  map[string]string{},
	})
	if err != nil {
		t.Fatalf("expected runner success, got %v", err)
	}
	if result.State != "completed" {
		t.Fatalf("expected completed state, got %q", result.State)
	}
	if result.FinalAnswer != "root final" {
		t.Fatalf("expected final answer %q, got %q", "root final", result.FinalAnswer)
	}

	eventsBytes, err := os.ReadFile(result.EventsPath)
	if err != nil {
		t.Fatalf("expected events file read success, got %v", err)
	}
	if !strings.Contains(string(eventsBytes), "recursive_subcall") {
		t.Fatalf("expected child node.started recursive_subcall event in log")
	}
}

func TestRunnerRunBuildsMessageInputAndExcludesRawContext(t *testing.T) {
	baseDir := filepath.Join(t.TempDir(), "sigil-runs")
	rawContext := "needle in haystack context body"
	inferenceClient := &queuedInference{
		responses: []queuedInferenceResponse{
			{result: finalResult("done")},
		},
	}

	runner := NewRunner(
		WithRunsBaseDir(baseDir),
		WithInferenceFactory(func(_ config.RunConfig) (InferenceClient, error) {
			return inferenceClient, nil
		}),
	)

	result, err := runner.Run(context.Background(), RunInput{
		AppConfigPath: "./sigil.yaml",
		RunConfigPath: "./sigil-run.yaml",
		RunConfig:     testRunConfig("root prompt", "", rawContext, ""),
		TemplateVars:  map[string]string{},
	})
	if err != nil {
		t.Fatalf("expected runner success, got %v", err)
	}
	if result.State != "completed" {
		t.Fatalf("expected completed state, got %q", result.State)
	}
	if len(inferenceClient.requests) != 1 {
		t.Fatalf("expected exactly one inference request, got %d", len(inferenceClient.requests))
	}

	request := inferenceClient.requests[0]
	if len(request.Messages) != 2 {
		t.Fatalf("expected 2 messages (system, user), got %d", len(request.Messages))
	}
	if request.Messages[0].Role != inference.MessageRoleSystem {
		t.Fatalf("expected first role system, got %q", request.Messages[0].Role)
	}
	if request.Messages[1].Role != inference.MessageRoleUser {
		t.Fatalf("expected second role user, got %q", request.Messages[1].Role)
	}
	if strings.Contains(request.Messages[0].Content, rawContext) || strings.Contains(request.Messages[1].Content, rawContext) {
		t.Fatalf("expected full raw context to be excluded from model-step messages")
	}

	var envelope StepInputEnvelope
	if err := json.Unmarshal([]byte(request.Messages[1].Content), &envelope); err != nil {
		t.Fatalf("expected user message to contain step envelope JSON, got %v", err)
	}
	if envelope.Query != "root prompt" {
		t.Fatalf("expected envelope.query to match run prompt, got %q", envelope.Query)
	}
	expectedMetadata := buildContextMetadata(rawContext, envelope.ContextMetadata.ContextRef)
	if envelope.ContextMetadata != expectedMetadata {
		t.Fatalf("expected deterministic context metadata %+v, got %+v", expectedMetadata, envelope.ContextMetadata)
	}
	if envelope.PreviousActionFeedback != nil {
		t.Fatalf("expected previous_action_feedback omitted on first step")
	}
}

func TestRunnerRunIncludesPreviousActionFeedbackOnSubsequentStep(t *testing.T) {
	baseDir := filepath.Join(t.TempDir(), "sigil-runs")
	largeStdout := strings.Repeat("a", stepInputPreviewCapBytes+32)
	inferenceClient := &queuedInference{
		responses: []queuedInferenceResponse{
			{result: continueResult(`import "fmt"; fmt.Print("` + largeStdout + `")`)},
			{result: finalResult("done")},
		},
	}

	runner := NewRunner(
		WithRunsBaseDir(baseDir),
		WithInferenceFactory(func(_ config.RunConfig) (InferenceClient, error) {
			return inferenceClient, nil
		}),
	)

	result, err := runner.Run(context.Background(), RunInput{
		AppConfigPath: "./sigil.yaml",
		RunConfigPath: "./sigil-run.yaml",
		RunConfig:     testRunConfig("root prompt", "", "root context", ""),
		TemplateVars:  map[string]string{},
	})
	if err != nil {
		t.Fatalf("expected runner success, got %v", err)
	}
	if result.State != "completed" {
		t.Fatalf("expected completed state, got %q", result.State)
	}
	if len(inferenceClient.requests) != 2 {
		t.Fatalf("expected exactly two inference requests, got %d", len(inferenceClient.requests))
	}

	var firstEnvelope StepInputEnvelope
	if err := json.Unmarshal([]byte(inferenceClient.requests[0].Messages[1].Content), &firstEnvelope); err != nil {
		t.Fatalf("expected first envelope decode success, got %v", err)
	}
	if firstEnvelope.PreviousActionFeedback != nil {
		t.Fatalf("expected first envelope to omit previous action feedback")
	}

	var secondEnvelope StepInputEnvelope
	if err := json.Unmarshal([]byte(inferenceClient.requests[1].Messages[1].Content), &secondEnvelope); err != nil {
		t.Fatalf("expected second envelope decode success, got %v", err)
	}
	if secondEnvelope.PreviousActionFeedback == nil {
		t.Fatal("expected second envelope to include previous action feedback")
	}
	feedback := secondEnvelope.PreviousActionFeedback
	if strings.TrimSpace(feedback.OutputRef) == "" {
		t.Fatal("expected feedback.output_ref to be populated")
	}
	if feedback.Status != string(runtime.ActionExecutionStatusCompleted) {
		t.Fatalf("expected feedback.status=completed, got %q", feedback.Status)
	}
	if !feedback.StdoutTruncated {
		t.Fatal("expected stdout preview to be marked truncated for oversized action output")
	}
	if feedback.StdoutBytes <= stepInputPreviewCapBytes {
		t.Fatalf("expected stdout_bytes > %d, got %d", stepInputPreviewCapBytes, feedback.StdoutBytes)
	}
	if len(feedback.StdoutPreview) != stepInputPreviewCapBytes {
		t.Fatalf("expected stdout preview size %d, got %d", stepInputPreviewCapBytes, len(feedback.StdoutPreview))
	}
}

func TestRunnerRunPropagatesCompileDiagnosticsInNextStepFeedback(t *testing.T) {
	baseDir := filepath.Join(t.TempDir(), "sigil-runs")
	inferenceClient := &queuedInference{
		responses: []queuedInferenceResponse{
			{result: continueResult("if {")},
			{result: finalResult("done")},
		},
	}

	runner := NewRunner(
		WithRunsBaseDir(baseDir),
		WithInferenceFactory(func(_ config.RunConfig) (InferenceClient, error) {
			return inferenceClient, nil
		}),
	)

	result, err := runner.Run(context.Background(), RunInput{
		AppConfigPath: "./sigil.yaml",
		RunConfigPath: "./sigil-run.yaml",
		RunConfig:     testRunConfig("root prompt", "", "root context", ""),
		TemplateVars:  map[string]string{},
	})
	if err != nil {
		t.Fatalf("expected runner success, got %v", err)
	}
	if result.State != "completed" {
		t.Fatalf("expected completed state, got %q", result.State)
	}
	if len(inferenceClient.requests) != 2 {
		t.Fatalf("expected two inference requests, got %d", len(inferenceClient.requests))
	}

	var secondEnvelope StepInputEnvelope
	if err := json.Unmarshal([]byte(inferenceClient.requests[1].Messages[1].Content), &secondEnvelope); err != nil {
		t.Fatalf("expected second envelope decode success, got %v", err)
	}
	if secondEnvelope.PreviousActionFeedback == nil {
		t.Fatal("expected previous_action_feedback in second step")
	}
	if secondEnvelope.PreviousActionFeedback.ErrorDetail == nil {
		t.Fatal("expected compile error_detail in previous_action_feedback")
	}
	if secondEnvelope.PreviousActionFeedback.ErrorDetail.Stage != "compile" {
		t.Fatalf("expected compile error_detail stage, got %q", secondEnvelope.PreviousActionFeedback.ErrorDetail.Stage)
	}
}

func TestRunnerRunNonRecursiveModeReturnsDepthLimitAndNoChildNodes(t *testing.T) {
	baseDir := filepath.Join(t.TempDir(), "sigil-runs")
	inferenceClient := &queuedInference{
		responses: []queuedInferenceResponse{
			{result: continueResult(`import "fmt"; _, err := rlm_query("child prompt", "child context"); if err != nil { fmt.Print(err.Error()) }`)},
			{result: finalResult("done")},
		},
	}

	runConfig := testRunConfig("root prompt", "", "root context", "")
	runConfig.RLM.Enabled = false

	runner := NewRunner(
		WithRunsBaseDir(baseDir),
		WithInferenceFactory(func(_ config.RunConfig) (InferenceClient, error) {
			return inferenceClient, nil
		}),
	)

	result, err := runner.Run(context.Background(), RunInput{
		AppConfigPath: "./sigil.yaml",
		RunConfigPath: "./sigil-run.yaml",
		RunConfig:     runConfig,
	})
	if err != nil {
		t.Fatalf("expected runner success, got %v", err)
	}
	if result.State != "completed" {
		t.Fatalf("expected completed state, got %q", result.State)
	}

	eventsBytes, err := os.ReadFile(result.EventsPath)
	if err != nil {
		t.Fatalf("expected events file read success, got %v", err)
	}
	if strings.Contains(string(eventsBytes), "recursive_subcall") {
		t.Fatalf("expected no recursive_subcall events in non-recursive mode")
	}

	actionArtifacts, err := filepath.Glob(filepath.Join(baseDir, result.RunID, "artifacts", "node", "*", "step", "*", "action-1.json"))
	if err != nil {
		t.Fatalf("expected action artifact glob success, got %v", err)
	}
	if len(actionArtifacts) == 0 {
		t.Fatalf("expected at least one action artifact")
	}

	artifactBytes, err := os.ReadFile(actionArtifacts[0])
	if err != nil {
		t.Fatalf("expected action artifact read success, got %v", err)
	}
	if !strings.Contains(string(artifactBytes), "repl_child_depth_limit") {
		t.Fatalf("expected non-recursive artifact feedback to include repl_child_depth_limit")
	}

	var artifact ActionArtifact
	if err := json.Unmarshal(artifactBytes, &artifact); err != nil {
		t.Fatalf("expected action artifact decode success, got %v", err)
	}
	if len(artifact.Subcalls) != 1 {
		t.Fatalf("expected one recorded subcall, got %d", len(artifact.Subcalls))
	}
	if artifact.Subcalls[0].Accounting == nil {
		t.Fatal("expected subcall accounting in action artifact")
	}
	if artifact.Subcalls[0].Accounting.TokenStatus != accounting.StatusUnavailable {
		t.Fatalf("expected unavailable artifact token status, got %q", artifact.Subcalls[0].Accounting.TokenStatus)
	}
	if artifact.Subcalls[0].Accounting.CostStatus != accounting.StatusUnavailable {
		t.Fatalf("expected unavailable artifact cost status, got %q", artifact.Subcalls[0].Accounting.CostStatus)
	}
	if artifact.Subcalls[0].Accounting.TotalTokens != nil {
		t.Fatalf("expected artifact total_tokens to remain unknown, got %+v", artifact.Subcalls[0].Accounting.TotalTokens)
	}

	events := mustReadPersistedEvents(t, baseDir)
	for _, event := range events {
		if event.Type != runtime.EventTypeNodeSubcallExecuted {
			continue
		}
		payload, ok := event.Payload.(runtime.NodeSubcallExecutedPayload)
		if !ok {
			t.Fatalf("expected node.subcall.executed payload type, got %T", event.Payload)
		}
		if payload.Accounting.TokenStatus != accounting.StatusUnavailable {
			t.Fatalf("expected unavailable event token status, got %q", payload.Accounting.TokenStatus)
		}
		if payload.Accounting.CostStatus != accounting.StatusUnavailable {
			t.Fatalf("expected unavailable event cost status, got %q", payload.Accounting.CostStatus)
		}
		if payload.Accounting.TotalTokens != nil {
			t.Fatalf("expected event total_tokens to remain unknown, got %+v", payload.Accounting.TotalTokens)
		}
		return
	}
	t.Fatal("expected node.subcall.executed event")
}

func TestRunnerRunRecursiveDepthLimitFallsBackToPlainSubcall(t *testing.T) {
	baseDir := filepath.Join(t.TempDir(), "sigil-runs")
	inferenceClient := &subcallAwareInference{
		root: []queuedInferenceResponse{
			{result: continueResult(`import "fmt"; answer, err := rlm_query("child prompt", "child context"); if err != nil { panic(err) }; fmt.Print(answer)`)},
			{result: finalResult("done")},
		},
	}

	runConfig := testRunConfig("root prompt", "", "root context", "")
	runConfig.RLM.MaxDepth = 0

	runner := NewRunner(
		WithRunsBaseDir(baseDir),
		WithInferenceFactory(func(_ config.RunConfig) (InferenceClient, error) {
			return inferenceClient, nil
		}),
	)

	result, err := runner.Run(context.Background(), RunInput{
		AppConfigPath: "./sigil.yaml",
		RunConfigPath: "./sigil-run.yaml",
		RunConfig:     runConfig,
	})
	if err != nil {
		t.Fatalf("expected runner success, got %v", err)
	}
	if result.State != "completed" {
		t.Fatalf("expected completed state, got %q", result.State)
	}

	eventsBytes, err := os.ReadFile(result.EventsPath)
	if err != nil {
		t.Fatalf("expected events file read success, got %v", err)
	}
	if !strings.Contains(string(eventsBytes), string(runtime.EventTypeNodeSubcallExecuted)) {
		t.Fatalf("expected node.subcall.executed event in log")
	}
	if !strings.Contains(string(eventsBytes), `"execution_mode":"fallback"`) {
		t.Fatalf("expected fallback execution_mode in subcall event payload")
	}

	actionArtifacts, err := filepath.Glob(filepath.Join(baseDir, result.RunID, "artifacts", "node", "*", "step", "*", "action-1.json"))
	if err != nil {
		t.Fatalf("expected action artifact glob success, got %v", err)
	}
	if len(actionArtifacts) == 0 {
		t.Fatal("expected at least one action artifact")
	}
	artifactBytes, err := os.ReadFile(actionArtifacts[0])
	if err != nil {
		t.Fatalf("expected action artifact read success, got %v", err)
	}
	var artifact ActionArtifact
	if err := json.Unmarshal(artifactBytes, &artifact); err != nil {
		t.Fatalf("expected artifact decode success, got %v", err)
	}
	if len(artifact.Subcalls) != 1 {
		t.Fatalf("expected one subcall trace, got %d", len(artifact.Subcalls))
	}
	if artifact.Subcalls[0].ExecutionMode != string(runtime.SubcallExecutionModeFallback) {
		t.Fatalf("expected fallback subcall trace mode, got %q", artifact.Subcalls[0].ExecutionMode)
	}
}

func TestRunnerRunRecursiveSubcallUsesIndependentTimeoutBudget(t *testing.T) {
	baseDir := filepath.Join(t.TempDir(), "sigil-runs")
	inferenceClient := &recursiveTimeoutInference{}

	runner := NewRunner(
		WithRunsBaseDir(baseDir),
		WithREPLSessionFactory(repl.NewFactory(
			repl.WithActionTimeout(2*time.Second),
			repl.WithRecursiveSubcallTimeout(300*time.Millisecond),
		)),
		WithInferenceFactory(func(_ config.RunConfig) (InferenceClient, error) {
			return inferenceClient, nil
		}),
	)

	result, err := runner.Run(context.Background(), RunInput{
		AppConfigPath: "./sigil.yaml",
		RunConfigPath: "./sigil-run.yaml",
		RunConfig:     testRunConfig("root prompt", "", "root context", ""),
	})
	if err != nil {
		t.Fatalf("expected runner success, got %v", err)
	}
	if result.State != "completed" {
		t.Fatalf("expected completed state, got %q", result.State)
	}

	inferenceClient.mu.Lock()
	observedDeadline := inferenceClient.childDeadline
	inferenceClient.mu.Unlock()
	if observedDeadline <= 0 {
		t.Fatalf("expected child inference deadline to be recorded, got %s", observedDeadline)
	}
	if observedDeadline >= 1*time.Second {
		t.Fatalf("expected child deadline to reflect recursive timeout budget, got %s", observedDeadline)
	}
}

func TestRunnerRunRecursiveSubcallTimeoutBudgetDecouplesAcrossRecursiveLevels(t *testing.T) {
	baseDir := filepath.Join(t.TempDir(), "sigil-runs")
	inferenceClient := &recursiveLevelTimeoutInference{}

	runner := NewRunner(
		WithRunsBaseDir(baseDir),
		WithREPLSessionFactory(repl.NewFactory(
			repl.WithActionTimeout(2*time.Second),
			repl.WithRecursiveSubcallTimeout(300*time.Millisecond),
		)),
		WithInferenceFactory(func(_ config.RunConfig) (InferenceClient, error) {
			return inferenceClient, nil
		}),
	)

	result, err := runner.Run(context.Background(), RunInput{
		AppConfigPath: "./sigil.yaml",
		RunConfigPath: "./sigil-run.yaml",
		RunConfig:     testRunConfig("root prompt", "", "root context", ""),
	})
	if err != nil {
		t.Fatalf("expected runner success, got %v", err)
	}
	if result.State != "completed" {
		t.Fatalf("expected completed state, got %q", result.State)
	}

	inferenceClient.mu.Lock()
	depth1Deadline := inferenceClient.depth1Deadline
	depth2Deadline := inferenceClient.depth2Deadline
	inferenceClient.mu.Unlock()
	if depth1Deadline <= 0 {
		t.Fatalf("expected depth1 deadline to be recorded, got %s", depth1Deadline)
	}
	if depth2Deadline <= 0 {
		t.Fatalf("expected depth2 deadline to be recorded, got %s", depth2Deadline)
	}
	if depth1Deadline < 200*time.Millisecond || depth1Deadline > 500*time.Millisecond {
		t.Fatalf("expected depth1 deadline near recursive timeout budget, got %s", depth1Deadline)
	}
	if depth2Deadline < 200*time.Millisecond || depth2Deadline > 500*time.Millisecond {
		t.Fatalf("expected depth2 deadline near recursive timeout budget, got %s", depth2Deadline)
	}
}

func TestRunnerRunBatchedPlainSubcallsPersistStableTraceOrdering(t *testing.T) {
	baseDir := filepath.Join(t.TempDir(), "sigil-runs")
	inferenceClient := &subcallAwareInference{
		root: []queuedInferenceResponse{
			{result: continueResult(`import "fmt"; calls := []map[string]string{{"prompt":"p1","context":"c1"},{"prompt":"p2","context":"c2"}}; answers, err := llm_query_batched(calls); if err != nil { panic(err) }; fmt.Print(answers[0]["answer"] + "," + answers[1]["answer"])`)},
			{result: finalResult("done")},
		},
	}

	runner := NewRunner(
		WithRunsBaseDir(baseDir),
		WithInferenceFactory(func(_ config.RunConfig) (InferenceClient, error) {
			return inferenceClient, nil
		}),
	)

	result, err := runner.Run(context.Background(), RunInput{
		AppConfigPath: "./sigil.yaml",
		RunConfigPath: "./sigil-run.yaml",
		RunConfig:     testRunConfig("root prompt", "", "root context", ""),
	})
	if err != nil {
		t.Fatalf("expected runner success, got %v", err)
	}
	if result.State != "completed" {
		t.Fatalf("expected completed state, got %q", result.State)
	}

	actionArtifacts, err := filepath.Glob(filepath.Join(baseDir, result.RunID, "artifacts", "node", "*", "step", "*", "action-1.json"))
	if err != nil {
		t.Fatalf("expected action artifact glob success, got %v", err)
	}
	if len(actionArtifacts) == 0 {
		t.Fatal("expected at least one action artifact")
	}
	artifactBytes, err := os.ReadFile(actionArtifacts[0])
	if err != nil {
		t.Fatalf("expected action artifact read success, got %v", err)
	}
	var artifact ActionArtifact
	if err := json.Unmarshal(artifactBytes, &artifact); err != nil {
		t.Fatalf("expected artifact decode success, got %v", err)
	}
	if len(artifact.Subcalls) != 2 {
		t.Fatalf("expected 2 subcall traces, got %d", len(artifact.Subcalls))
	}
	for index, trace := range artifact.Subcalls {
		expectedIndex := index + 1
		if trace.SubcallIndex != expectedIndex {
			t.Fatalf("expected subcall_index=%d, got %d", expectedIndex, trace.SubcallIndex)
		}
		if trace.SubcallType != string(runtime.SubcallTypeLLMQueryBatched) {
			t.Fatalf("expected subcall_type=%q, got %q", runtime.SubcallTypeLLMQueryBatched, trace.SubcallType)
		}
	}
}

func TestRunnerRunTemplateRenderFailureReturnsTypedError(t *testing.T) {
	baseDir := filepath.Join(t.TempDir(), "sigil-runs")
	inferenceClient := &queuedInference{}
	runCfg := testRunConfig("", "prompt {{.missing}}", "root context", "")
	runCfg.Accounting.PricingVersion = "pricing-v2"

	runner := NewRunner(
		WithRunsBaseDir(baseDir),
		WithInferenceFactory(func(_ config.RunConfig) (InferenceClient, error) {
			return inferenceClient, nil
		}),
	)

	_, err := runner.Run(context.Background(), RunInput{
		AppConfigPath: "./sigil.yaml",
		RunConfigPath: "./sigil-run.yaml",
		RunConfig:     runCfg,
		TemplateVars:  map[string]string{},
	})
	if err == nil {
		t.Fatal("expected template render failure")
	}

	code, ok := CodeOf(err)
	if !ok {
		t.Fatalf("expected typed harness error, got %v", err)
	}
	if code != ErrorCodeTemplateRender {
		t.Fatalf("expected error code %q, got %q", ErrorCodeTemplateRender, code)
	}

	payload := mustReadRunFailedPayload(t, baseDir)
	if payload.Accounting.TreeTotal.PricingVersion != runCfg.Accounting.PricingVersion {
		t.Fatalf("expected pricing_version %q, got %q", runCfg.Accounting.PricingVersion, payload.Accounting.TreeTotal.PricingVersion)
	}
	if payload.Accounting.TreeTotal.PricingKey.Provider != runCfg.LLM.Provider {
		t.Fatalf("expected provider %q, got %q", runCfg.LLM.Provider, payload.Accounting.TreeTotal.PricingKey.Provider)
	}
	if payload.Accounting.TreeTotal.PricingKey.Model != runCfg.LLM.Model {
		t.Fatalf("expected model %q, got %q", runCfg.LLM.Model, payload.Accounting.TreeTotal.PricingKey.Model)
	}
}

func TestRunnerRunInferenceFailureAppendsRunFailed(t *testing.T) {
	baseDir := filepath.Join(t.TempDir(), "sigil-runs")
	inferenceClient := &queuedInference{
		responses: []queuedInferenceResponse{{err: inference.NewError(inference.ErrorCodeGatewayFailure, "gateway down")}},
	}

	runner := NewRunner(
		WithRunsBaseDir(baseDir),
		WithInferenceFactory(func(_ config.RunConfig) (InferenceClient, error) {
			return inferenceClient, nil
		}),
	)

	_, err := runner.Run(context.Background(), RunInput{
		AppConfigPath: "./sigil.yaml",
		RunConfigPath: "./sigil-run.yaml",
		RunConfig:     testRunConfig("root prompt", "", "root context", ""),
	})
	if err == nil {
		t.Fatal("expected inference failure")
	}

	code, ok := CodeOf(err)
	if !ok {
		t.Fatalf("expected typed harness error, got %v", err)
	}
	if code != ErrorCodeInference {
		t.Fatalf("expected error code %q, got %q", ErrorCodeInference, code)
	}

	eventsFiles, err := filepath.Glob(filepath.Join(baseDir, "*", "events.jsonl"))
	if err != nil {
		t.Fatalf("expected events glob success, got %v", err)
	}
	if len(eventsFiles) != 1 {
		t.Fatalf("expected one events file, got %d", len(eventsFiles))
	}

	eventsBytes, err := os.ReadFile(eventsFiles[0])
	if err != nil {
		t.Fatalf("expected events read success, got %v", err)
	}
	if !strings.Contains(string(eventsBytes), "run.failed") {
		t.Fatalf("expected run.failed event in events log")
	}
	if !strings.Contains(string(eventsBytes), string(ErrorCodeInference)) {
		t.Fatalf("expected run.failed payload to include %q", ErrorCodeInference)
	}
	if !strings.Contains(string(eventsBytes), "node.failed") {
		t.Fatalf("expected node.failed event in events log")
	}
	if strings.Index(string(eventsBytes), "node.failed") > strings.Index(string(eventsBytes), "run.failed") {
		t.Fatalf("expected node.failed to be emitted before run.failed")
	}

	payload := mustReadRunFailedPayload(t, baseDir)
	if payload.Accounting.TreeTotal.TokenStatus != accounting.StatusUnavailable {
		t.Fatalf("expected unavailable token status, got %q", payload.Accounting.TreeTotal.TokenStatus)
	}
	if payload.Accounting.TreeTotal.CostStatus != accounting.StatusUnavailable {
		t.Fatalf("expected unavailable cost status, got %q", payload.Accounting.TreeTotal.CostStatus)
	}
	if payload.Accounting.TreeTotal.TotalTokens != nil {
		t.Fatalf("expected total_tokens to remain unknown, got %+v", payload.Accounting.TreeTotal.TotalTokens)
	}
	if payload.Accounting.TreeTotal.KnownTotalCostMicrousd != nil {
		t.Fatalf("expected known_total_cost_microusd to remain unknown, got %+v", payload.Accounting.TreeTotal.KnownTotalCostMicrousd)
	}
}

func TestRunnerRunChildFailureEmitsNodeFailedBeforeParentSubcallFailureEvent(t *testing.T) {
	baseDir := filepath.Join(t.TempDir(), "sigil-runs")
	inferenceClient := &childFailureInference{}

	runner := NewRunner(
		WithRunsBaseDir(baseDir),
		WithInferenceFactory(func(_ config.RunConfig) (InferenceClient, error) {
			return inferenceClient, nil
		}),
	)

	result, err := runner.Run(context.Background(), RunInput{
		AppConfigPath: "./sigil.yaml",
		RunConfigPath: "./sigil-run.yaml",
		RunConfig:     testRunConfig("root prompt", "", "root context", ""),
	})
	if err != nil {
		t.Fatalf("expected runner success despite child failure, got %v", err)
	}
	if result.State != "completed" {
		t.Fatalf("expected completed state, got %q", result.State)
	}

	eventsBytes, err := os.ReadFile(result.EventsPath)
	if err != nil {
		t.Fatalf("expected events read success, got %v", err)
	}
	eventsLog := string(eventsBytes)
	childFailedIndex := strings.Index(eventsLog, `"type":"node.failed"`)
	parentSubcallFailedIndex := strings.Index(eventsLog, `"type":"node.subcall.executed"`)
	if childFailedIndex < 0 || parentSubcallFailedIndex < 0 {
		t.Fatalf("expected node.failed and node.subcall.executed events in log")
	}
	if childFailedIndex > parentSubcallFailedIndex {
		t.Fatalf("expected child node.failed before parent subcall event")
	}
}

func TestRunnerRunFailsWithOutputValidationWhenFinalEvidenceIsUnresolvable(t *testing.T) {
	baseDir := filepath.Join(t.TempDir(), "sigil-runs")
	inferenceClient := &queuedInference{
		responses: []queuedInferenceResponse{
			{
				result: inference.Result{
					SchemaID: "sigil.rlm.response.v1",
					ValidatedPayload: map[string]any{
						"decision": "final",
						"final": map[string]any{
							"answer":   "done",
							"evidence": []any{map[string]any{"ref": "run-output://node/missing/context.json"}},
						},
					},
					Gateway:           "openrouter",
					Provider:          "openai",
					Model:             "gpt-5.1",
					GatewayResponseID: "resp_final",
					FinishStatus:      "completed",
					RawMetadata:       map[string]any{},
				},
			},
		},
	}

	runner := NewRunner(
		WithRunsBaseDir(baseDir),
		WithInferenceFactory(func(_ config.RunConfig) (InferenceClient, error) {
			return inferenceClient, nil
		}),
	)

	_, err := runner.Run(context.Background(), RunInput{
		AppConfigPath: "./sigil.yaml",
		RunConfigPath: "./sigil-run.yaml",
		RunConfig:     testRunConfig("root prompt", "", "root context", ""),
	})
	if err == nil {
		t.Fatal("expected run failure")
	}
	code, ok := CodeOf(err)
	if !ok {
		t.Fatalf("expected typed harness error, got %v", err)
	}
	if code != ErrorCodeOutputValidation {
		t.Fatalf("expected output_validation typed error, got %q", code)
	}
}

func TestRunnerRunNormalizesMalformedPreviousActionOutputRefInFinalEvidence(t *testing.T) {
	baseDir := filepath.Join(t.TempDir(), "sigil-runs")
	inferenceClient := &queuedInference{
		responses: []queuedInferenceResponse{
			{result: continueResult(`import "fmt"; fmt.Print("ok")`)},
			{result: finalResultWithEvidence("done", "__previous_output_ref_malformed__")},
		},
	}

	runner := NewRunner(
		WithRunsBaseDir(baseDir),
		WithInferenceFactory(func(_ config.RunConfig) (InferenceClient, error) {
			return inferenceClient, nil
		}),
	)

	result, err := runner.Run(context.Background(), RunInput{
		AppConfigPath: "./sigil.yaml",
		RunConfigPath: "./sigil-run.yaml",
		RunConfig:     testRunConfig("root prompt", "", "root context", ""),
	})
	if err != nil {
		t.Fatalf("expected runner success, got %v", err)
	}

	if len(inferenceClient.requests) < 2 {
		t.Fatalf("expected at least two inference requests, got %d", len(inferenceClient.requests))
	}
	var secondStepEnvelope StepInputEnvelope
	if err := json.Unmarshal([]byte(inferenceClient.requests[1].Messages[1].Content), &secondStepEnvelope); err != nil {
		t.Fatalf("expected second-step envelope decode success, got %v", err)
	}
	if secondStepEnvelope.PreviousActionFeedback == nil {
		t.Fatal("expected second-step previous_action_feedback")
	}
	expectedRef := secondStepEnvelope.PreviousActionFeedback.OutputRef
	if strings.TrimSpace(expectedRef) == "" {
		t.Fatal("expected previous_action_feedback.output_ref")
	}

	outputPath, err := resolveRunOutputPath(result.FinalAnswerRef)
	if err != nil {
		t.Fatalf("expected final answer ref path resolution success, got %v", err)
	}
	finalAnswerPath := filepath.Join(append([]string{baseDir, result.RunID, "outputs"}, outputPath...)...)
	encoded, err := os.ReadFile(finalAnswerPath)
	if err != nil {
		t.Fatalf("expected final answer artifact read success, got %v", err)
	}

	var artifact finalAnswerArtifact
	if err := json.Unmarshal(encoded, &artifact); err != nil {
		t.Fatalf("expected final answer artifact decode success, got %v", err)
	}
	if len(artifact.Evidence) != 1 {
		t.Fatalf("expected one evidence item, got %d", len(artifact.Evidence))
	}
	if artifact.Evidence[0].Ref != expectedRef {
		t.Fatalf("expected normalized evidence ref %q, got %q", expectedRef, artifact.Evidence[0].Ref)
	}
}

func TestRunnerRunFailsWhenMaxStepsPerNodeGuardrailBreached(t *testing.T) {
	baseDir := filepath.Join(t.TempDir(), "sigil-runs")
	inferenceClient := &queuedInference{
		responses: []queuedInferenceResponse{
			{result: continueResult(`import "fmt"; fmt.Print("loop")`)},
		},
	}

	runConfig := testRunConfig("root prompt", "", "root context", "")
	runConfig.Guardrails.MaxStepsPerNode = 1
	runConfig.Guardrails.MaxTotalStepsPerRun = 10

	runner := NewRunner(
		WithRunsBaseDir(baseDir),
		WithInferenceFactory(func(_ config.RunConfig) (InferenceClient, error) {
			return inferenceClient, nil
		}),
	)

	_, err := runner.Run(context.Background(), RunInput{
		AppConfigPath: "./sigil.yaml",
		RunConfigPath: "./sigil-run.yaml",
		RunConfig:     runConfig,
	})
	if err == nil {
		t.Fatal("expected guardrail run failure")
	}
	code, ok := CodeOf(err)
	if !ok || code != ErrorCodeLimitExceeded {
		t.Fatalf("expected typed limit-exceeded error, got %v", err)
	}
	limit, ok := LimitOf(err)
	if !ok {
		t.Fatalf("expected limit metadata on error")
	}
	if limit.LimitKey != limitKeyMaxStepsPerNode {
		t.Fatalf("expected limit key %q, got %q", limitKeyMaxStepsPerNode, limit.LimitKey)
	}

	payload := mustReadRunFailedPayload(t, baseDir)
	if payload.LimitKey == nil || *payload.LimitKey != limitKeyMaxStepsPerNode {
		t.Fatalf("expected run.failed limit_key %q, got %v", limitKeyMaxStepsPerNode, payload.LimitKey)
	}
	if !strings.Contains(payload.ErrorMessage, "attempted=2") {
		t.Fatalf("expected run.failed error_message to clarify attempted step start, got %q", payload.ErrorMessage)
	}
}

func TestRunnerRunFailsWhenMaxTotalStepsPerRunGuardrailBreachedAcrossRecursiveNodes(t *testing.T) {
	baseDir := filepath.Join(t.TempDir(), "sigil-runs")
	inferenceClient := &totalStepsGuardrailInference{}

	runConfig := testRunConfig("root prompt", "", "root context", "")
	runConfig.Guardrails.MaxStepsPerNode = 2
	runConfig.Guardrails.MaxTotalStepsPerRun = 2

	runner := NewRunner(
		WithRunsBaseDir(baseDir),
		WithInferenceFactory(func(_ config.RunConfig) (InferenceClient, error) {
			return inferenceClient, nil
		}),
	)

	_, err := runner.Run(context.Background(), RunInput{
		AppConfigPath: "./sigil.yaml",
		RunConfigPath: "./sigil-run.yaml",
		RunConfig:     runConfig,
	})
	if err == nil {
		t.Fatal("expected guardrail run failure")
	}
	code, ok := CodeOf(err)
	if !ok || code != ErrorCodeLimitExceeded {
		t.Fatalf("expected typed limit-exceeded error, got %v", err)
	}
	limit, ok := LimitOf(err)
	if !ok {
		t.Fatalf("expected limit metadata on error")
	}
	if limit.LimitKey != limitKeyMaxTotalStepsPerRun {
		t.Fatalf("expected limit key %q, got %q", limitKeyMaxTotalStepsPerRun, limit.LimitKey)
	}

	events := mustReadPersistedEvents(t, baseDir)
	if countFailedActionEvents(events) != 1 {
		t.Fatalf("expected one failed parent node.action.executed event, got %d", countFailedActionEvents(events))
	}
	if countEventType(events, runtime.EventTypeNodeStepCompleted) != 2 {
		t.Fatalf("expected both child and parent steps to emit node.step.completed, got %d", countEventType(events, runtime.EventTypeNodeStepCompleted))
	}
}

func TestRunnerRunAbortsParentActionAfterFatalChildGuardrailBreach(t *testing.T) {
	baseDir := filepath.Join(t.TempDir(), "sigil-runs")
	inferenceClient := &queuedInference{
		responses: []queuedInferenceResponse{
			{result: continueResult(`import "fmt"; _, _ = rlm_query("child prompt", "child context"); fmt.Print("after-child-limit")`)},
			{result: continueResult(`import "fmt"; fmt.Print("child step")`)},
		},
	}

	runConfig := testRunConfig("root prompt", "", "root context", "")
	runConfig.Guardrails.MaxStepsPerNode = 1
	runConfig.Guardrails.MaxTotalStepsPerRun = 10

	runner := NewRunner(
		WithRunsBaseDir(baseDir),
		WithInferenceFactory(func(_ config.RunConfig) (InferenceClient, error) {
			return inferenceClient, nil
		}),
	)

	_, err := runner.Run(context.Background(), RunInput{
		AppConfigPath: "./sigil.yaml",
		RunConfigPath: "./sigil-run.yaml",
		RunConfig:     runConfig,
	})
	if err == nil {
		t.Fatal("expected guardrail run failure")
	}

	for _, artifact := range mustReadActionArtifacts(t, baseDir) {
		if strings.Contains(artifact.Stdout, "after-child-limit") {
			t.Fatalf("expected parent action to abort before post-breach code runs, got stdout %q", artifact.Stdout)
		}
	}
}

func TestRunnerRunStopsBatchedRecursiveSubcallsAfterFatalGuardrailBreach(t *testing.T) {
	baseDir := filepath.Join(t.TempDir(), "sigil-runs")
	inferenceClient := &queuedInference{
		responses: []queuedInferenceResponse{
			{result: continueResult(`import "fmt"; calls := []map[string]string{{"prompt":"child1","context":"ctx1"},{"prompt":"child2","context":"ctx2"}}; _, _ = rlm_query_batched(calls); fmt.Print("after-batched-child-limit")`)},
		},
	}

	runConfig := testRunConfig("root prompt", "", "root context", "")
	runConfig.Guardrails.MaxStepsPerNode = 10
	runConfig.Guardrails.MaxTotalStepsPerRun = 1

	runner := NewRunner(
		WithRunsBaseDir(baseDir),
		WithInferenceFactory(func(_ config.RunConfig) (InferenceClient, error) {
			return inferenceClient, nil
		}),
	)

	_, err := runner.Run(context.Background(), RunInput{
		AppConfigPath: "./sigil.yaml",
		RunConfigPath: "./sigil-run.yaml",
		RunConfig:     runConfig,
	})
	if err == nil {
		t.Fatal("expected guardrail run failure")
	}

	events := mustReadPersistedEvents(t, baseDir)
	if countEventType(events, runtime.EventTypeNodeSubcallExecuted) != 1 {
		t.Fatalf("expected batched recursive subcalls to stop after first fatal guardrail, got %d node.subcall.executed events", countEventType(events, runtime.EventTypeNodeSubcallExecuted))
	}
	for _, artifact := range mustReadActionArtifacts(t, baseDir) {
		if strings.Contains(artifact.Stdout, "after-batched-child-limit") {
			t.Fatalf("expected batched parent action to abort before post-breach code runs, got stdout %q", artifact.Stdout)
		}
	}
}

func TestRunnerRunFailsWhenMaxRunDurationGuardrailBreached(t *testing.T) {
	baseDir := filepath.Join(t.TempDir(), "sigil-runs")
	inferenceClient := &queuedInference{
		responses: []queuedInferenceResponse{
			{result: continueResult(`import "fmt"; import "time"; time.Sleep(50 * time.Millisecond); fmt.Print("after-sleep")`)},
		},
	}

	runConfig := testRunConfig("root prompt", "", "root context", "")
	runConfig.Guardrails.MaxRunDurationMS = 15
	runConfig.Guardrails.MaxStepsPerNode = 10
	runConfig.Guardrails.MaxTotalStepsPerRun = 20

	runner := NewRunner(
		WithRunsBaseDir(baseDir),
		WithInferenceFactory(func(_ config.RunConfig) (InferenceClient, error) {
			return inferenceClient, nil
		}),
	)

	_, err := runner.Run(context.Background(), RunInput{
		AppConfigPath: "./sigil.yaml",
		RunConfigPath: "./sigil-run.yaml",
		RunConfig:     runConfig,
	})
	if err == nil {
		t.Fatal("expected guardrail run failure")
	}
	limit, ok := LimitOf(err)
	if !ok {
		t.Fatalf("expected limit metadata on error")
	}
	if limit.LimitKey != limitKeyMaxRunDurationMS {
		t.Fatalf("expected limit key %q, got %q", limitKeyMaxRunDurationMS, limit.LimitKey)
	}

	events := mustReadPersistedEvents(t, baseDir)
	if countEventType(events, runtime.EventTypeNodeStepCompleted) != 0 {
		t.Fatalf("expected interrupted duration-guardrail step to avoid node.step.completed, got %d", countEventType(events, runtime.EventTypeNodeStepCompleted))
	}
	actionArtifacts, err := filepath.Glob(filepath.Join(baseDir, "*", "artifacts", "node", "*", "step", "*", "action-1.json"))
	if err != nil {
		t.Fatalf("expected action artifact glob success, got %v", err)
	}
	for _, artifactPath := range actionArtifacts {
		artifactBytes, readErr := os.ReadFile(artifactPath)
		if readErr != nil {
			t.Fatalf("expected action artifact read success, got %v", readErr)
		}

		var artifact ActionArtifact
		if err := json.Unmarshal(artifactBytes, &artifact); err != nil {
			t.Fatalf("expected artifact decode success, got %v", err)
		}
		if strings.Contains(artifact.Stdout, "after-sleep") {
			t.Fatalf("expected duration guardrail to interrupt before post-sleep output, got stdout %q", artifact.Stdout)
		}
	}
}

func TestRunnerRunFailsWhenMaxRunDurationBudgetIsConsumedBeforeFirstStep(t *testing.T) {
	baseDir := filepath.Join(t.TempDir(), "sigil-runs")
	inferenceClient := &queuedInference{
		responses: []queuedInferenceResponse{
			{result: finalResult("done")},
		},
	}

	runConfig := testRunConfig("root prompt", "", "root context", "")
	runConfig.Guardrails.MaxRunDurationMS = 5
	runConfig.Guardrails.MaxStepsPerNode = 10
	runConfig.Guardrails.MaxTotalStepsPerRun = 20

	runner := NewRunner(
		WithRunsBaseDir(baseDir),
		WithInferenceFactory(func(_ config.RunConfig) (InferenceClient, error) {
			time.Sleep(20 * time.Millisecond)
			return inferenceClient, nil
		}),
	)

	_, err := runner.Run(context.Background(), RunInput{
		AppConfigPath: "./sigil.yaml",
		RunConfigPath: "./sigil-run.yaml",
		RunConfig:     runConfig,
	})
	if err == nil {
		t.Fatal("expected guardrail run failure before first step")
	}
	limit, ok := LimitOf(err)
	if !ok {
		t.Fatalf("expected limit metadata on error")
	}
	if limit.LimitKey != limitKeyMaxRunDurationMS {
		t.Fatalf("expected limit key %q, got %q", limitKeyMaxRunDurationMS, limit.LimitKey)
	}
	if inferenceClient.calls != 0 {
		t.Fatalf("expected no inference calls once startup already consumed the budget, got %d", inferenceClient.calls)
	}

	events := mustReadPersistedEvents(t, baseDir)
	if countEventType(events, runtime.EventTypeNodeStepStarted) != 0 {
		t.Fatalf("expected no node.step.started events, got %d", countEventType(events, runtime.EventTypeNodeStepStarted))
	}
}

func TestRunnerRunCancelsRecursiveSubcallsWhenRunDurationGuardrailExpires(t *testing.T) {
	baseDir := filepath.Join(t.TempDir(), "sigil-runs")
	inferenceClient := &recursiveTimeoutInference{}

	runConfig := testRunConfig("root prompt", "", "root context", "")
	runConfig.Guardrails.MaxRunDurationMS = 50
	runConfig.Guardrails.MaxStepsPerNode = 10
	runConfig.Guardrails.MaxTotalStepsPerRun = 20

	runner := NewRunner(
		WithRunsBaseDir(baseDir),
		WithREPLSessionFactory(repl.NewFactory(
			repl.WithActionTimeout(2*time.Second),
			repl.WithRecursiveSubcallTimeout(300*time.Millisecond),
		)),
		WithInferenceFactory(func(_ config.RunConfig) (InferenceClient, error) {
			return inferenceClient, nil
		}),
	)

	_, err := runner.Run(context.Background(), RunInput{
		AppConfigPath: "./sigil.yaml",
		RunConfigPath: "./sigil-run.yaml",
		RunConfig:     runConfig,
	})
	if err == nil {
		t.Fatal("expected guardrail run failure")
	}

	limit, ok := LimitOf(err)
	if !ok {
		t.Fatalf("expected limit metadata on error")
	}
	if limit.LimitKey != limitKeyMaxRunDurationMS {
		t.Fatalf("expected limit key %q, got %q", limitKeyMaxRunDurationMS, limit.LimitKey)
	}

	inferenceClient.mu.Lock()
	childHasDeadline := inferenceClient.childHasDeadline
	observedDeadline := inferenceClient.childDeadline
	inferenceClient.mu.Unlock()
	if !childHasDeadline {
		t.Fatal("expected child inference deadline to be recorded")
	}
	if observedDeadline > 150*time.Millisecond {
		t.Fatalf("expected recursive child deadline to honor run-duration guardrail, got %s", observedDeadline)
	}
}

func TestRunnerRunFailsWhenConsecutiveFailedContinueActionsGuardrailBreached(t *testing.T) {
	baseDir := filepath.Join(t.TempDir(), "sigil-runs")
	inferenceClient := &queuedInference{
		responses: []queuedInferenceResponse{
			{result: continueResult("if {")},
			{result: continueResult("if {")},
		},
	}

	runConfig := testRunConfig("root prompt", "", "root context", "")
	runConfig.Guardrails.MaxConsecutiveStepFailures = 2
	runConfig.Guardrails.MaxStepsPerNode = 10
	runConfig.Guardrails.MaxTotalStepsPerRun = 20

	runner := NewRunner(
		WithRunsBaseDir(baseDir),
		WithInferenceFactory(func(_ config.RunConfig) (InferenceClient, error) {
			return inferenceClient, nil
		}),
	)

	_, err := runner.Run(context.Background(), RunInput{
		AppConfigPath: "./sigil.yaml",
		RunConfigPath: "./sigil-run.yaml",
		RunConfig:     runConfig,
	})
	if err == nil {
		t.Fatal("expected guardrail run failure")
	}
	limit, ok := LimitOf(err)
	if !ok {
		t.Fatalf("expected limit metadata on error")
	}
	if limit.LimitKey != limitKeyMaxConsecutiveStepErrors {
		t.Fatalf("expected limit key %q, got %q", limitKeyMaxConsecutiveStepErrors, limit.LimitKey)
	}

	events := mustReadPersistedEvents(t, baseDir)
	if countEventType(events, runtime.EventTypeNodeStepCompleted) != 2 {
		t.Fatalf("expected both failed continue steps to emit node.step.completed, got %d", countEventType(events, runtime.EventTypeNodeStepCompleted))
	}
}

func TestRunnerRunFailsWhenChildGuardrailBreachAlsoTriggersParentExecError(t *testing.T) {
	baseDir := filepath.Join(t.TempDir(), "sigil-runs")
	inferenceClient := &queuedInference{
		responses: []queuedInferenceResponse{
			{result: continueResult(`import "fmt"; _, err := rlm_query("child prompt", "child context"); if err != nil { panic(err) }`)},
			{result: continueResult("if {")},
			{result: continueResult("if {")},
			{result: finalResult("should not reach")},
		},
	}

	runConfig := testRunConfig("root prompt", "", "root context", "")
	runConfig.Guardrails.MaxConsecutiveStepFailures = 2
	runConfig.Guardrails.MaxStepsPerNode = 10
	runConfig.Guardrails.MaxTotalStepsPerRun = 20

	runner := NewRunner(
		WithRunsBaseDir(baseDir),
		WithInferenceFactory(func(_ config.RunConfig) (InferenceClient, error) {
			return inferenceClient, nil
		}),
	)

	_, err := runner.Run(context.Background(), RunInput{
		AppConfigPath: "./sigil.yaml",
		RunConfigPath: "./sigil-run.yaml",
		RunConfig:     runConfig,
	})
	if err == nil {
		t.Fatal("expected guardrail run failure")
	}
	code, ok := CodeOf(err)
	if !ok || code != ErrorCodeLimitExceeded {
		t.Fatalf("expected typed limit-exceeded error, got %v", err)
	}
	limit, ok := LimitOf(err)
	if !ok {
		t.Fatalf("expected limit metadata on error")
	}
	if limit.LimitKey != limitKeyMaxConsecutiveStepErrors {
		t.Fatalf("expected limit key %q, got %q", limitKeyMaxConsecutiveStepErrors, limit.LimitKey)
	}
	if inferenceClient.calls != 3 {
		t.Fatalf("expected guardrail terminalization before root final inference, got %d inference calls", inferenceClient.calls)
	}

	events := mustReadPersistedEvents(t, baseDir)
	if countEventType(events, runtime.EventTypeRunFailed) != 1 {
		t.Fatalf("expected one run.failed event, got %d", countEventType(events, runtime.EventTypeRunFailed))
	}
}

func TestRunnerRunResetsConsecutiveFailureCounterAfterSuccessfulContinue(t *testing.T) {
	baseDir := filepath.Join(t.TempDir(), "sigil-runs")
	inferenceClient := &queuedInference{
		responses: []queuedInferenceResponse{
			{result: continueResult("if {")},
			{result: continueResult(`import "fmt"; fmt.Print("ok")`)},
			{result: continueResult("if {")},
			{result: finalResult("done")},
		},
	}

	runConfig := testRunConfig("root prompt", "", "root context", "")
	runConfig.Guardrails.MaxConsecutiveStepFailures = 2
	runConfig.Guardrails.MaxStepsPerNode = 10
	runConfig.Guardrails.MaxTotalStepsPerRun = 20

	runner := NewRunner(
		WithRunsBaseDir(baseDir),
		WithInferenceFactory(func(_ config.RunConfig) (InferenceClient, error) {
			return inferenceClient, nil
		}),
	)

	result, err := runner.Run(context.Background(), RunInput{
		AppConfigPath: "./sigil.yaml",
		RunConfigPath: "./sigil-run.yaml",
		RunConfig:     runConfig,
	})
	if err != nil {
		t.Fatalf("expected run completion when failure counter resets, got %v", err)
	}
	if result.State != "completed" {
		t.Fatalf("expected completed state, got %q", result.State)
	}
}

func TestRunnerRunOmitsNodeFailedStepIDWhenGuardrailBlocksNextStepStart(t *testing.T) {
	baseDir := filepath.Join(t.TempDir(), "sigil-runs")
	inferenceClient := &queuedInference{
		responses: []queuedInferenceResponse{
			{result: continueResult(`import "fmt"; fmt.Print("loop")`)},
		},
	}

	runConfig := testRunConfig("root prompt", "", "root context", "")
	runConfig.Guardrails.MaxStepsPerNode = 1
	runConfig.Guardrails.MaxTotalStepsPerRun = 10

	runner := NewRunner(
		WithRunsBaseDir(baseDir),
		WithInferenceFactory(func(_ config.RunConfig) (InferenceClient, error) {
			return inferenceClient, nil
		}),
	)

	_, err := runner.Run(context.Background(), RunInput{
		AppConfigPath: "./sigil.yaml",
		RunConfigPath: "./sigil-run.yaml",
		RunConfig:     runConfig,
	})
	if err == nil {
		t.Fatal("expected guardrail run failure")
	}

	payload := mustReadNodeFailedPayload(t, baseDir)
	if payload.FailedStepID != nil {
		t.Fatalf("expected node.failed failed_step_id to be omitted for pre-step guardrail breach, got %q", *payload.FailedStepID)
	}
}

func mustReadNodeFailedPayload(t *testing.T, runsBaseDir string) runtime.NodeFailedPayload {
	t.Helper()

	events := mustReadPersistedEvents(t, runsBaseDir)
	for index := len(events) - 1; index >= 0; index-- {
		event := events[index]
		if event.Type != runtime.EventTypeNodeFailed {
			continue
		}
		payload, ok := event.Payload.(runtime.NodeFailedPayload)
		if !ok {
			t.Fatalf("expected node.failed payload type, got %T", event.Payload)
		}
		return payload
	}

	t.Fatalf("expected node.failed payload in events")
	return runtime.NodeFailedPayload{}
}

func mustReadActionArtifacts(t *testing.T, runsBaseDir string) []ActionArtifact {
	t.Helper()

	actionArtifacts, err := filepath.Glob(filepath.Join(runsBaseDir, "*", "artifacts", "node", "*", "step", "*", "action-1.json"))
	if err != nil {
		t.Fatalf("expected action artifact glob success, got %v", err)
	}
	if len(actionArtifacts) == 0 {
		t.Fatal("expected at least one action artifact")
	}

	artifacts := make([]ActionArtifact, 0, len(actionArtifacts))
	for _, artifactPath := range actionArtifacts {
		artifactBytes, readErr := os.ReadFile(artifactPath)
		if readErr != nil {
			t.Fatalf("expected action artifact read success, got %v", readErr)
		}

		var artifact ActionArtifact
		if err := json.Unmarshal(artifactBytes, &artifact); err != nil {
			t.Fatalf("expected artifact decode success, got %v", err)
		}
		artifacts = append(artifacts, artifact)
	}

	return artifacts
}

func mustReadRunFailedPayload(t *testing.T, runsBaseDir string) runtime.RunFailedPayload {
	t.Helper()

	events := mustReadPersistedEvents(t, runsBaseDir)
	for index := len(events) - 1; index >= 0; index-- {
		event := events[index]
		if event.Type != runtime.EventTypeRunFailed {
			continue
		}
		payload, ok := event.Payload.(runtime.RunFailedPayload)
		if !ok {
			t.Fatalf("expected run.failed payload type, got %T", event.Payload)
		}
		return payload
	}

	t.Fatalf("expected run.failed payload in events")
	return runtime.RunFailedPayload{}
}

func mustReadPersistedEvents(t *testing.T, runsBaseDir string) []runtime.EventEnvelope {
	t.Helper()

	eventsFiles, err := filepath.Glob(filepath.Join(runsBaseDir, "*", "events.jsonl"))
	if err != nil {
		t.Fatalf("expected events glob success, got %v", err)
	}
	if len(eventsFiles) != 1 {
		t.Fatalf("expected one events file, got %d", len(eventsFiles))
	}

	encoded, err := os.ReadFile(eventsFiles[0])
	if err != nil {
		t.Fatalf("expected events file read success, got %v", err)
	}

	lines := strings.Split(strings.TrimSpace(string(encoded)), "\n")
	events := make([]runtime.EventEnvelope, 0, len(lines))
	for _, rawLine := range lines {
		line := strings.TrimSpace(rawLine)
		if line == "" {
			continue
		}
		event, parseErr := runtime.ParseEventEnvelopeStrict([]byte(line))
		if parseErr != nil {
			t.Fatalf("expected parse success for persisted event, got %v", parseErr)
		}
		events = append(events, event)
	}

	return events
}

func countEventType(events []runtime.EventEnvelope, eventType runtime.EventType) int {
	count := 0
	for _, event := range events {
		if event.Type == eventType {
			count++
		}
	}
	return count
}

func countFailedActionEvents(events []runtime.EventEnvelope) int {
	count := 0
	for _, event := range events {
		if event.Type != runtime.EventTypeNodeActionExecuted {
			continue
		}
		payload, ok := event.Payload.(runtime.NodeActionExecutedPayload)
		if ok && payload.Status == runtime.ActionExecutionStatusFailed {
			count++
		}
	}
	return count
}

func continueResult(replCode string) inference.Result {
	return inference.Result{
		SchemaID:          "sigil.rlm.response.v1",
		ValidatedPayload:  map[string]any{"decision": "continue", "continuation": map[string]any{"repl_code": replCode, "intent": "inspect state", "expected_observation": "deterministic output to guide next step"}},
		Gateway:           "openrouter",
		Provider:          "openai",
		Model:             "gpt-5.1",
		GatewayResponseID: "resp_continue",
		FinishStatus:      "completed",
		RawMetadata:       map[string]any{},
	}
}

func finalResult(answer string) inference.Result {
	return inference.Result{
		SchemaID: "sigil.rlm.response.v1",
		ValidatedPayload: map[string]any{
			"decision": "final",
			"final": map[string]any{
				"answer":   answer,
				"evidence": []any{map[string]any{"ref": "__context_ref__"}},
			},
		},
		Gateway:           "openrouter",
		Provider:          "openai",
		Model:             "gpt-5.1",
		GatewayResponseID: "resp_final",
		FinishStatus:      "completed",
		RawMetadata:       map[string]any{},
	}
}

func finalResultWithEvidence(answer string, ref string) inference.Result {
	return inference.Result{
		SchemaID: "sigil.rlm.response.v1",
		ValidatedPayload: map[string]any{
			"decision": "final",
			"final": map[string]any{
				"answer":   answer,
				"evidence": []any{map[string]any{"ref": ref}},
			},
		},
		Gateway:           "openrouter",
		Provider:          "openai",
		Model:             "gpt-5.1",
		GatewayResponseID: "resp_final",
		FinishStatus:      "completed",
		RawMetadata:       map[string]any{},
	}
}

func testRunConfig(prompt string, promptTemplate string, contextValue string, contextTemplate string) config.RunConfig {
	cfg := config.NewDefaultRunConfig()
	cfg.Prompt = prompt
	cfg.PromptTemplate = promptTemplate
	cfg.Context = contextValue
	cfg.ContextTemplate = contextTemplate
	cfg.LLM.Provider = "openai"
	cfg.LLM.Model = "gpt-5.1"
	cfg.LLM.Gateway = "openrouter"
	cfg.LLM.OpenRouter.BaseURL = "http://127.0.0.1:1"
	cfg.LLM.OpenRouter.APIKeyEnv = "OPENROUTER_API_KEY"
	cfg.LLM.OpenRouter.RequestTimeoutMS = 500
	cfg.RLM.Enabled = true
	cfg.RLM.MaxDepth = 3
	return cfg
}

func TestRunnerRunIncludesAccountingInRunResultAndEvents(t *testing.T) {
	baseDir := filepath.Join(t.TempDir(), "sigil-runs")
	inputTokens := int64(12)
	outputTokens := int64(5)
	totalTokens := int64(17)
	totalCost := int64(1750)

	inferenceClient := &queuedInference{
		responses: []queuedInferenceResponse{
			{result: inference.Result{
				SchemaID:          schema.SigilRLMResponseV1SchemaID,
				ValidatedPayload:  map[string]any{"decision": "final", "final": map[string]any{"answer": "done", "evidence": []any{map[string]any{"ref": "__context_ref__"}}}},
				Gateway:           "openrouter",
				Provider:          "openai",
				Model:             "gpt-5.1",
				GatewayResponseID: "resp_final",
				FinishStatus:      "completed",
				RawMetadata:       map[string]any{},
				Accounting: accounting.BuildLeafSummary(accounting.LeafInput{
					Provider:                 "openai",
					Model:                    "gpt-5.1",
					PricingVersion:           "v1",
					InputTokens:              &inputTokens,
					OutputTokens:             &outputTokens,
					TotalTokens:              &totalTokens,
					GatewayTotalCostMicrousd: &totalCost,
				}),
			}},
		},
	}

	runner := NewRunner(
		WithRunsBaseDir(baseDir),
		WithInferenceFactory(func(_ config.RunConfig) (InferenceClient, error) {
			return inferenceClient, nil
		}),
	)

	result, err := runner.Run(context.Background(), RunInput{
		AppConfigPath: "./sigil.yaml",
		RunConfigPath: "./sigil-run.yaml",
		RunConfig:     testRunConfig("root prompt", "", "root context", ""),
		TemplateVars:  map[string]string{},
	})
	if err != nil {
		t.Fatalf("expected runner success, got %v", err)
	}
	if result.Accounting.TreeTotal.TotalTokens == nil || *result.Accounting.TreeTotal.TotalTokens != 17 {
		t.Fatalf("expected run accounting tree total_tokens=17, got %+v", result.Accounting.TreeTotal.TotalTokens)
	}
	if result.Accounting.TreeTotal.KnownTotalCostMicrousd == nil || *result.Accounting.TreeTotal.KnownTotalCostMicrousd != 1750 {
		t.Fatalf("expected run accounting known_total_cost_microusd=1750, got %+v", result.Accounting.TreeTotal.KnownTotalCostMicrousd)
	}

	eventsBytes, err := os.ReadFile(result.EventsPath)
	if err != nil {
		t.Fatalf("expected events file read success, got %v", err)
	}
	eventsText := string(eventsBytes)
	if !strings.Contains(eventsText, `"accounting_ref":"run-output://run/accounting.json"`) {
		t.Fatalf("expected run.completed accounting_ref in events, got %q", eventsText)
	}
	if !strings.Contains(eventsText, `"accounting_ref":"run-output://node/`) {
		t.Fatalf("expected node.step.completed accounting_ref in events, got %q", eventsText)
	}
}

func TestRunResultIsJSONSerializable(t *testing.T) {
	payload := RunResult{
		RunID:          "run",
		State:          "completed",
		FinalAnswer:    "done",
		FinalAnswerRef: "run-output://node/x/final-answer.json",
		EventsPath:     "/tmp/events.jsonl",
	}
	if _, err := json.Marshal(payload); err != nil {
		t.Fatalf("expected run result JSON marshal success, got %v", err)
	}
}
