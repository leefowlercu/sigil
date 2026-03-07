package harness

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"log/slog"
	"strings"
	"sync"
	"time"

	"github.com/leefowlercu/sigil/internal/accounting"
	"github.com/leefowlercu/sigil/internal/config"
	"github.com/leefowlercu/sigil/internal/inference"
	"github.com/leefowlercu/sigil/internal/inference/schema"
	"github.com/leefowlercu/sigil/internal/repl"
	"github.com/leefowlercu/sigil/internal/runtime"
)

const (
	defaultLLMQueryBatchWorkers = 4
)

const plainSubcallSystemPrompt = "" +
	"You are a lightweight subcall helper. " +
	"Use only the provided prompt and context. " +
	"Keep the answer terse, grounded, and non-speculative. " +
	"If the context is insufficient, return the best grounded answer the context supports without inventing facts. " +
	"Return exactly one strict JSON object with key answer and no extra keys. " +
	"If the caller asks for structured data, place that structure as minified JSON text inside the answer string instead of adding top-level keys. " +
	`Valid example: {"answer":"{\"has_token\":false}"}. ` +
	`Valid example: {"answer":"{\"has_token\":true,\"token\":\"SIGIL-NEEDLE-2026-03-03-ALPHA-OMEGA-1234\",\"line\":\"CHUNK-0001 | ...\"}"}. ` +
	`Invalid example: {"has_token":false}. ` +
	`Invalid example: {"answer":{"has_token":false}}.`

type childNodeExecutor func(ctx context.Context, child runtime.Node, prompt string, subContext string) (nodeExecutionResult, error)

type subcallRecord struct {
	answer      string
	err         error
	durationMS  int
	mode        runtime.SubcallExecutionMode
	childNodeID *string
	accounting  accounting.Summary
}

type SubcallRouter struct {
	lifecycle    *runtime.Lifecycle
	inference    InferenceClient
	runConfig    config.RunConfig
	node         runtime.Node
	stepID       string
	actionIndex  int
	nonRecursive bool
	turnOutputs  *TurnOutputStore
	ledger       *accounting.Ledger
	guardrails   *deterministicGuardrails
	executeChild childNodeExecutor
	logger       *slog.Logger
	batchWorkers int

	mu             sync.Mutex
	nextSubcallIdx int
	traces         []ActionSubcallTrace
	fatalErr       error
}

type SubcallRouterInput struct {
	Lifecycle    *runtime.Lifecycle
	Inference    InferenceClient
	RunConfig    config.RunConfig
	Node         runtime.Node
	StepID       string
	ActionIndex  int
	NonRecursive bool
	TurnOutputs  *TurnOutputStore
	Ledger       *accounting.Ledger
	Guardrails   *deterministicGuardrails
	ExecuteChild childNodeExecutor
}

func NewSubcallRouter(input SubcallRouterInput) (*SubcallRouter, error) {
	if input.Lifecycle == nil {
		return nil, fmt.Errorf("lifecycle is required")
	}
	if input.Inference == nil {
		return nil, fmt.Errorf("inference client is required")
	}
	if strings.TrimSpace(input.Node.ID) == "" {
		return nil, fmt.Errorf("node is required")
	}
	if strings.TrimSpace(input.StepID) == "" {
		return nil, fmt.Errorf("step id is required")
	}
	if input.ActionIndex < 1 {
		return nil, fmt.Errorf("action index must be >= 1")
	}
	if input.TurnOutputs == nil {
		return nil, fmt.Errorf("turn output store is required")
	}
	if input.Ledger == nil {
		return nil, fmt.Errorf("accounting ledger is required")
	}
	if input.ExecuteChild == nil {
		return nil, fmt.Errorf("child executor is required")
	}

	router := &SubcallRouter{
		lifecycle:      input.Lifecycle,
		inference:      input.Inference,
		runConfig:      input.RunConfig,
		node:           input.Node,
		stepID:         input.StepID,
		actionIndex:    input.ActionIndex,
		nonRecursive:   input.NonRecursive,
		turnOutputs:    input.TurnOutputs,
		ledger:         input.Ledger,
		guardrails:     input.Guardrails,
		executeChild:   input.ExecuteChild,
		logger:         subcallRouterLogger().With("run_id", input.Lifecycle.RunID(), "node_id", input.Node.ID, "step_id", input.StepID),
		batchWorkers:   defaultLLMQueryBatchWorkers,
		nextSubcallIdx: 1,
		traces:         make([]ActionSubcallTrace, 0, 4),
	}

	return router, nil
}

func (r *SubcallRouter) LLMQuery(ctx context.Context, request repl.QueryRequest) (string, error) {
	record := r.executeLLMQuery(ctx, request)
	if err := r.persistRecord(runtime.SubcallTypeLLMQuery, request, record); err != nil {
		return record.answer, repl.MarkFatalExecution(err)
	}
	return record.answer, record.err
}

func (r *SubcallRouter) RLMQuery(ctx context.Context, request repl.QueryRequest) (string, error) {
	record, fatalErr := r.executeRLMQuery(ctx, request)
	if err := r.persistRecord(runtime.SubcallTypeRLMQuery, request, record); err != nil {
		return record.answer, repl.MarkFatalExecution(err)
	}
	if fatalErr != nil {
		if record.err != nil {
			return record.answer, repl.MarkFatalExecution(record.err)
		}
		return record.answer, repl.MarkFatalExecution(fatalErr)
	}
	return record.answer, record.err
}

func (r *SubcallRouter) LLMQueryBatched(ctx context.Context, requests []repl.BatchedQueryRequest) ([]repl.BatchedQueryResult, error) {
	if len(requests) == 0 {
		return nil, repl.WrapError(repl.ErrorCodeSubcallInvalidInput, "llm_query_batched requires at least one call item", nil)
	}

	results := make([]subcallRecord, len(requests))
	type workItem struct {
		index int
		req   repl.BatchedQueryRequest
	}
	queue := make(chan workItem)
	var wg sync.WaitGroup
	workers := r.batchWorkers
	if workers < 1 {
		workers = 1
	}
	if workers > len(requests) {
		workers = len(requests)
	}

	for worker := 0; worker < workers; worker++ {
		wg.Add(1)
		go func() {
			defer wg.Done()
			for item := range queue {
				results[item.index] = r.executeLLMQuery(ctx, repl.QueryRequest{
					Prompt:  item.req.Prompt,
					Context: item.req.Context,
				})
			}
		}()
	}
	for index, request := range requests {
		queue <- workItem{index: index, req: request}
	}
	close(queue)
	wg.Wait()

	output := make([]repl.BatchedQueryResult, len(requests))
	for index, request := range requests {
		record := results[index]
		if err := r.persistRecord(runtime.SubcallTypeLLMQueryBatched, repl.QueryRequest{
			Prompt:  request.Prompt,
			Context: request.Context,
		}, record); err != nil {
			return nil, err
		}
		output[index] = toBatchedResult(record.answer, record.err)
	}

	return output, nil
}

func (r *SubcallRouter) RLMQueryBatched(ctx context.Context, requests []repl.BatchedQueryRequest) ([]repl.BatchedQueryResult, error) {
	if len(requests) == 0 {
		return nil, repl.WrapError(repl.ErrorCodeSubcallInvalidInput, "rlm_query_batched requires at least one call item", nil)
	}

	output := make([]repl.BatchedQueryResult, 0, len(requests))
	for _, request := range requests {
		record, fatalErr := r.executeRLMQuery(ctx, repl.QueryRequest{
			Prompt:  request.Prompt,
			Context: request.Context,
		})
		if err := r.persistRecord(runtime.SubcallTypeRLMQueryBatched, repl.QueryRequest{
			Prompt:  request.Prompt,
			Context: request.Context,
		}, record); err != nil {
			return nil, repl.MarkFatalExecution(err)
		}
		if fatalErr != nil {
			if record.err != nil {
				return nil, repl.MarkFatalExecution(record.err)
			}
			return nil, repl.MarkFatalExecution(fatalErr)
		}
		output = append(output, toBatchedResult(record.answer, record.err))
	}

	return output, nil
}

func (r *SubcallRouter) Traces() []ActionSubcallTrace {
	r.mu.Lock()
	defer r.mu.Unlock()

	if len(r.traces) == 0 {
		return nil
	}
	cloned := make([]ActionSubcallTrace, len(r.traces))
	copy(cloned, r.traces)
	return cloned
}

func (r *SubcallRouter) FatalError() error {
	r.mu.Lock()
	defer r.mu.Unlock()
	return r.fatalErr
}

func (r *SubcallRouter) executeLLMQuery(ctx context.Context, request repl.QueryRequest) subcallRecord {
	start := time.Now().UTC()
	answer, summary, err := r.inferLLMAnswer(ctx, request.Prompt, request.Context)
	return subcallRecord{
		answer:     answer,
		err:        err,
		durationMS: durationMS(start),
		mode:       runtime.SubcallExecutionModePlain,
		accounting: summary,
	}
}

func (r *SubcallRouter) executeRLMQuery(ctx context.Context, request repl.QueryRequest) (subcallRecord, error) {
	start := time.Now().UTC()
	if r.nonRecursive {
		err := repl.WrapError(repl.ErrorCodeChildDepthLimit, "rlm_query is disabled in non-recursive mode", nil)
		return subcallRecord{
			err:        err,
			durationMS: durationMS(start),
			mode:       runtime.SubcallExecutionModeFallback,
			accounting: accounting.UnavailableSummary(r.runConfig.LLM.Provider, r.runConfig.LLM.Model, r.runConfig.Accounting.PricingVersion),
		}, nil
	}

	childNode, err := r.lifecycle.CreateChildNode(r.node.ID)
	if err != nil {
		if errors.Is(err, runtime.ErrDepthLimitExceeded) {
			answer, summary, fallbackErr := r.inferLLMAnswer(ctx, request.Prompt, request.Context)
			return subcallRecord{
				answer:     answer,
				err:        fallbackErr,
				durationMS: durationMS(start),
				mode:       runtime.SubcallExecutionModeFallback,
				accounting: summary,
			}, nil
		}

		return subcallRecord{
			err:        repl.WrapError(repl.ErrorCodeChildFailure, "rlm_query child creation failed", err),
			durationMS: durationMS(start),
			mode:       runtime.SubcallExecutionModeRecursive,
			accounting: accounting.UnavailableSummary(r.runConfig.LLM.Provider, r.runConfig.LLM.Model, r.runConfig.Accounting.PricingVersion),
		}, nil
	}

	result, childErr := r.executeChild(ctx, childNode, request.Prompt, request.Context)
	childAccounting := r.ledger.NodeRollup(childNode.ID).TreeTotal
	if childErr != nil {
		record := subcallRecord{
			err:         repl.WrapError(repl.ErrorCodeChildFailure, "rlm_query child execution failed", childErr),
			durationMS:  durationMS(start),
			mode:        runtime.SubcallExecutionModeRecursive,
			childNodeID: cloneStringPointer(childNode.ID),
			accounting:  childAccounting,
		}
		if code, ok := CodeOf(childErr); ok && code == ErrorCodeLimitExceeded {
			r.setFatal(childErr)
			return record, childErr
		}
		return record, nil
	}

	return subcallRecord{
		answer:      result.answer,
		durationMS:  durationMS(start),
		mode:        runtime.SubcallExecutionModeRecursive,
		childNodeID: cloneStringPointer(childNode.ID),
		accounting:  childAccounting,
	}, nil
}

func (r *SubcallRouter) inferLLMAnswer(ctx context.Context, prompt string, subContext string) (string, accounting.Summary, error) {
	userPayload := map[string]string{
		"prompt":  prompt,
		"context": subContext,
	}
	encodedPayload, err := json.Marshal(userPayload)
	if err != nil {
		return "", accounting.UnavailableSummary(r.runConfig.LLM.Provider, r.runConfig.LLM.Model, r.runConfig.Accounting.PricingVersion), repl.WrapError(repl.ErrorCodeSubcallInference, "failed to encode plain subcall input", err)
	}

	result, err := r.inference.Infer(ctx, inference.Request{
		Messages: []inference.Message{
			{Role: inference.MessageRoleSystem, Content: plainSubcallSystemPrompt},
			{Role: inference.MessageRoleUser, Content: string(encodedPayload)},
		},
		SchemaID: schema.SigilLLMAnswerV1SchemaID,
		Gateway:  r.runConfig.LLM.Gateway,
		Provider: r.runConfig.LLM.Provider,
		Model:    r.runConfig.LLM.Model,
		GatewayConfig: inference.GatewayConfig{
			BaseURL:          r.runConfig.LLM.OpenRouter.BaseURL,
			RequestTimeoutMS: r.runConfig.LLM.OpenRouter.RequestTimeoutMS,
			APIKeyEnv:        r.runConfig.LLM.OpenRouter.APIKeyEnv,
		},
		Reasoning: inference.ReasoningConfig{
			Enabled: false,
			Effort:  "",
		},
		Accounting: buildInferenceAccountingConfig(r.runConfig),
	})
	if err != nil {
		return "", accounting.UnavailableSummary(r.runConfig.LLM.Provider, r.runConfig.LLM.Model, r.runConfig.Accounting.PricingVersion), repl.WrapError(repl.ErrorCodeSubcallInference, "plain subcall inference failed", err)
	}

	answerRaw, ok := result.ValidatedPayload["answer"]
	if !ok {
		return "", result.Accounting, repl.WrapError(repl.ErrorCodeSubcallInference, "plain subcall payload missing answer", nil)
	}
	answer, ok := answerRaw.(string)
	if !ok || strings.TrimSpace(answer) == "" {
		return "", result.Accounting, repl.WrapError(repl.ErrorCodeSubcallInference, "plain subcall payload answer must be non-empty", nil)
	}

	return answer, result.Accounting, nil
}

func (r *SubcallRouter) persistRecord(subcallType runtime.SubcallType, request repl.QueryRequest, record subcallRecord) error {
	subcallIndex := r.nextIndex()
	subcallAccounting := record.accounting
	if subcallAccounting.Currency == "" {
		subcallAccounting = accounting.UnavailableSummary(r.runConfig.LLM.Provider, r.runConfig.LLM.Model, r.runConfig.Accounting.PricingVersion)
	}
	r.ledger.RecordSubcall(r.node.ID, r.stepID, subcallAccounting)
	accountingRef, err := r.turnOutputs.PersistSubcallAccounting(r.lifecycle.RunID(), r.node.ID, r.stepID, subcallIndex, subcallAccounting)
	if err != nil {
		wrapped := repl.WrapError(repl.ErrorCodeSubcallEventPersist, "failed to persist subcall accounting output", err)
		r.setFatal(wrapped)
		return wrapped
	}

	trace := ActionSubcallTrace{
		SubcallIndex:  subcallIndex,
		SubcallType:   string(subcallType),
		ExecutionMode: string(record.mode),
		Status:        string(runtime.ActionExecutionStatusCompleted),
		Provider:      r.runConfig.LLM.Provider,
		Model:         r.runConfig.LLM.Model,
		PromptBytes:   len(request.Prompt),
		ContextBytes:  len(request.Context),
		AnswerBytes:   len(record.answer),
		DurationMS:    record.durationMS,
		ChildNodeID:   cloneOptional(record.childNodeID),
		Accounting:    &subcallAccounting,
	}
	payload := runtime.NodeSubcallExecutedPayload{
		StepID:        r.stepID,
		ActionIndex:   r.actionIndex,
		SubcallIndex:  subcallIndex,
		SubcallType:   subcallType,
		ExecutionMode: record.mode,
		Status:        runtime.ActionExecutionStatusCompleted,
		Provider:      r.runConfig.LLM.Provider,
		Model:         r.runConfig.LLM.Model,
		PromptBytes:   len(request.Prompt),
		ContextBytes:  len(request.Context),
		AnswerBytes:   len(record.answer),
		DurationMS:    record.durationMS,
		ChildNodeID:   cloneOptional(record.childNodeID),
		Accounting:    subcallAccounting,
		AccountingRef: accountingRef,
	}

	if record.err != nil {
		trace.Status = string(runtime.ActionExecutionStatusFailed)
		payload.Status = runtime.ActionExecutionStatusFailed

		errorCode := string(repl.ErrorCodeSubcallInference)
		if typedCode, ok := repl.CodeOf(record.err); ok {
			errorCode = string(typedCode)
		}
		errorMessage := record.err.Error()
		trace.ErrorCode = &errorCode
		trace.ErrorMessage = &errorMessage
		payload.ErrorCode = &errorCode
		payload.ErrorMessage = &errorMessage
	}

	if err := r.lifecycle.AppendNodeSubcallExecuted(r.node.ID, payload); err != nil {
		wrapped := repl.WrapError(repl.ErrorCodeSubcallEventPersist, "failed to persist node.subcall.executed", err)
		r.setFatal(wrapped)
		return wrapped
	}

	r.mu.Lock()
	r.traces = append(r.traces, trace)
	r.mu.Unlock()

	if r.guardrails != nil {
		if err := r.guardrails.CheckRunAccounting(r.node.ID, r.stepID, r.ledger.RunRollup().TreeTotal); err != nil {
			r.setFatal(err)
			return err
		}
	}
	return nil
}

func (r *SubcallRouter) nextIndex() int {
	r.mu.Lock()
	defer r.mu.Unlock()
	index := r.nextSubcallIdx
	r.nextSubcallIdx++
	return index
}

func (r *SubcallRouter) setFatal(err error) {
	if err == nil {
		return
	}
	r.mu.Lock()
	defer r.mu.Unlock()
	if r.fatalErr == nil {
		r.fatalErr = err
	}
}

func toBatchedResult(answer string, err error) repl.BatchedQueryResult {
	result := repl.BatchedQueryResult{
		Answer: answer,
	}
	if err == nil {
		return result
	}

	if code, ok := repl.CodeOf(err); ok {
		result.ErrorCode = string(code)
	} else {
		result.ErrorCode = string(repl.ErrorCodeSubcallInference)
	}
	result.ErrorMessage = err.Error()
	return result
}

func cloneStringPointer(value string) *string {
	copyValue := value
	return &copyValue
}

func subcallRouterLogger() *slog.Logger {
	return slog.Default().With("component", "harness.subcall_router")
}
