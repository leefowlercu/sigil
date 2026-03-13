package repl

import (
	"context"
	"fmt"
	"strings"
)

// QueryRequest defines the recursive query contract exposed to Go REPL code.
type QueryRequest struct {
	Prompt  string
	Context string
}

// QueryFunc executes a recursive child-node query and returns a final answer.
type QueryFunc func(ctx context.Context, request QueryRequest) (string, error)

// BatchedQueryRequest defines one batched subcall request item.
type BatchedQueryRequest struct {
	Prompt  string
	Context string
}

// BatchedQueryResult defines one batched subcall response item.
type BatchedQueryResult struct {
	Answer       string
	ErrorCode    string
	ErrorMessage string
}

// BatchedQueryFunc executes batched subcalls and returns per-item results.
type BatchedQueryFunc func(ctx context.Context, requests []BatchedQueryRequest) ([]BatchedQueryResult, error)

// ActionOutput exposes the narrow exact-output fields retrievable from an action artifact.
type ActionOutput struct {
	Status       string
	Stdout       string
	Stderr       string
	ErrorCode    string
	ErrorMessage string
}

// ActionArtifactReadFunc resolves one exact action artifact by canonical action_ref.
type ActionArtifactReadFunc func(actionRef string) (ActionOutput, error)

// SessionOptions defines required construction inputs for a node-local REPL session.
type SessionOptions struct {
	RunID              string
	NodeID             string
	Depth              int
	Context            string
	RunContext         context.Context
	LLMQuery           QueryFunc
	RLMQuery           QueryFunc
	LLMQueryBatched    BatchedQueryFunc
	RLMQueryBatched    BatchedQueryFunc
	ReadActionArtifact ActionArtifactReadFunc
}

// ExecResult captures normalized output from one code execution action.
type ExecResult struct {
	Stdout     string
	Stderr     string
	DurationMS int
}

// Session executes Go code while preserving node-local state across step actions.
type Session interface {
	Exec(ctx context.Context, code string) (ExecResult, error)
	Close() error
}

// SessionFactory creates node-local REPL sessions.
type SessionFactory interface {
	NewSession(ctx context.Context, options SessionOptions) (Session, error)
}

// ValidateSessionOptions validates required session construction inputs.
func ValidateSessionOptions(options SessionOptions) error {
	if strings.TrimSpace(options.RunID) == "" {
		return fmt.Errorf("run id is required; %w", ErrInvalidSessionOptions)
	}
	if strings.TrimSpace(options.NodeID) == "" {
		return fmt.Errorf("node id is required; %w", ErrInvalidSessionOptions)
	}
	if options.Depth < 0 {
		return fmt.Errorf("depth must be >= 0; %w", ErrInvalidSessionOptions)
	}
	if options.LLMQuery == nil {
		return fmt.Errorf("llm_query function is required; %w", ErrInvalidSessionOptions)
	}
	if options.RLMQuery == nil {
		return fmt.Errorf("rlm_query function is required; %w", ErrInvalidSessionOptions)
	}
	if options.LLMQueryBatched == nil {
		return fmt.Errorf("llm_query_batched function is required; %w", ErrInvalidSessionOptions)
	}
	if options.RLMQueryBatched == nil {
		return fmt.Errorf("rlm_query_batched function is required; %w", ErrInvalidSessionOptions)
	}
	if options.ReadActionArtifact == nil {
		return fmt.Errorf("read_action_artifact function is required; %w", ErrInvalidSessionOptions)
	}

	return nil
}

// ParseBatchedCalls validates and converts Go REPL batched call map input.
func ParseBatchedCalls(calls []map[string]string) ([]BatchedQueryRequest, error) {
	if len(calls) == 0 {
		return nil, fmt.Errorf("calls must contain at least one item")
	}

	requests := make([]BatchedQueryRequest, 0, len(calls))
	for index, call := range calls {
		if call == nil {
			return nil, fmt.Errorf("calls[%d] must be an object", index)
		}
		if len(call) != 2 {
			return nil, fmt.Errorf("calls[%d] must contain exactly prompt and context keys", index)
		}

		prompt, promptOK := call["prompt"]
		contextValue, contextOK := call["context"]
		if !promptOK || !contextOK {
			return nil, fmt.Errorf("calls[%d] must contain prompt and context keys", index)
		}
		if strings.TrimSpace(prompt) == "" {
			return nil, fmt.Errorf("calls[%d].prompt must be non-empty", index)
		}

		requests = append(requests, BatchedQueryRequest{
			Prompt:  prompt,
			Context: contextValue,
		})
	}

	return requests, nil
}

// EncodeBatchedResults converts typed batched results to Go REPL map output.
func EncodeBatchedResults(results []BatchedQueryResult) []map[string]string {
	encoded := make([]map[string]string, len(results))
	for index, result := range results {
		encoded[index] = map[string]string{
			"answer":        result.Answer,
			"error_code":    result.ErrorCode,
			"error_message": result.ErrorMessage,
		}
	}

	return encoded
}
