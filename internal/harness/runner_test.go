package harness

import (
	"context"
	"encoding/json"
	"errors"
	"os"
	"path/filepath"
	"strings"
	"sync"
	"testing"

	"github.com/leefowlercu/sigil/internal/config"
	"github.com/leefowlercu/sigil/internal/inference"
	"github.com/leefowlercu/sigil/internal/inference/schema"
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
	evidenceItem, ok := evidenceRaw[0].(map[string]any)
	if !ok {
		return result
	}
	refValue, _ := evidenceItem["ref"].(string)
	if refValue != "__context_ref__" {
		return result
	}
	if len(request.Messages) < 2 {
		return result
	}
	var envelope StepInputEnvelope
	if err := json.Unmarshal([]byte(request.Messages[1].Content), &envelope); err != nil {
		return result
	}
	if strings.TrimSpace(envelope.ContextMetadata.ContextRef) == "" {
		return result
	}
	evidenceItem["ref"] = envelope.ContextMetadata.ContextRef
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

	runner := NewRunner(
		WithRunsBaseDir(baseDir),
		WithInferenceFactory(func(_ config.RunConfig) (InferenceClient, error) {
			return inferenceClient, nil
		}),
	)

	_, err := runner.Run(context.Background(), RunInput{
		AppConfigPath: "./sigil.yaml",
		RunConfigPath: "./sigil-run.yaml",
		RunConfig:     testRunConfig("", "prompt {{.missing}}", "root context", ""),
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
