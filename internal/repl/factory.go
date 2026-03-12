package repl

import (
	"bytes"
	"context"
	"errors"
	"fmt"
	"log/slog"
	"reflect"
	"strconv"
	"strings"
	"sync"
	"time"

	"github.com/traefik/yaegi/interp"
	"github.com/traefik/yaegi/stdlib"
)

const (
	// OutputTruncationMarker is appended when output exceeds configured caps.
	OutputTruncationMarker = "\n[TRUNCATED]\n"
)

var defaultAllowedImports = map[string]struct{}{
	"fmt":           {},
	"strings":       {},
	"strconv":       {},
	"sort":          {},
	"regexp":        {},
	"encoding/json": {},
	"bytes":         {},
	"math":          {},
	"time":          {},
	"slices":        {},
}

// Factory is the production session factory for an embedded Go REPL.
type Factory struct {
	actionTimeout           time.Duration
	recursiveSubcallTimeout time.Duration
	maxCodeBytes            int
	stdoutCapBytes          int
	stderrCapBytes          int
	allowedImports          map[string]struct{}
}

// FactoryOption mutates factory defaults.
type FactoryOption func(*Factory)

// WithActionTimeout sets per-action timeout.
func WithActionTimeout(timeout time.Duration) FactoryOption {
	return func(factory *Factory) {
		factory.actionTimeout = timeout
	}
}

// WithRecursiveSubcallTimeout sets timeout budget for recursive subcalls.
func WithRecursiveSubcallTimeout(timeout time.Duration) FactoryOption {
	return func(factory *Factory) {
		factory.recursiveSubcallTimeout = timeout
	}
}

// WithMaxCodeBytes sets maximum action code payload bytes.
func WithMaxCodeBytes(maxBytes int) FactoryOption {
	return func(factory *Factory) {
		factory.maxCodeBytes = maxBytes
	}
}

// WithOutputCaps sets stdout/stderr capture caps.
func WithOutputCaps(stdoutCap int, stderrCap int) FactoryOption {
	return func(factory *Factory) {
		factory.stdoutCapBytes = stdoutCap
		factory.stderrCapBytes = stderrCap
	}
}

// NewFactory constructs a Yaegi-backed session factory.
func NewFactory(options ...FactoryOption) *Factory {
	factory := &Factory{
		actionTimeout:           180 * time.Second,
		recursiveSubcallTimeout: 300 * time.Second,
		maxCodeBytes:            65536,
		stdoutCapBytes:          1048576,
		stderrCapBytes:          1048576,
		allowedImports:          cloneImportSet(defaultAllowedImports),
	}
	for _, option := range options {
		if option != nil {
			option(factory)
		}
	}

	return factory
}

// NewSession validates options and returns a node-local Yaegi session.
func (f *Factory) NewSession(_ context.Context, options SessionOptions) (Session, error) {
	if f == nil {
		return nil, WrapError(ErrorCodeSessionInit, "session factory is required", nil)
	}
	if err := ValidateSessionOptions(options); err != nil {
		return nil, err
	}
	if f.actionTimeout <= 0 {
		return nil, WrapError(ErrorCodeSessionInit, "action timeout must be > 0", nil)
	}
	if f.recursiveSubcallTimeout <= 0 {
		return nil, WrapError(ErrorCodeSessionInit, "recursive subcall timeout must be > 0", nil)
	}
	if f.maxCodeBytes < 1 {
		return nil, WrapError(ErrorCodeSessionInit, "max code bytes must be >= 1", nil)
	}
	if f.stdoutCapBytes < 1 || f.stderrCapBytes < 1 {
		return nil, WrapError(ErrorCodeSessionInit, "output caps must be >= 1", nil)
	}

	stdout := newCappedBuffer(f.stdoutCapBytes)
	stderr := newCappedBuffer(f.stderrCapBytes)
	interpreter := interp.New(interp.Options{
		Stdout: stdout,
		Stderr: stderr,
	})

	if err := interpreter.Use(stdlib.Symbols); err != nil {
		return nil, WrapError(ErrorCodeSessionInit, "failed to register stdlib symbols", err)
	}

	session := &yaegiSession{
		interpreter:             interpreter,
		stdout:                  stdout,
		stderr:                  stderr,
		runContextSource:        options.RunContext,
		actionTimeout:           f.actionTimeout,
		recursiveSubcallTimeout: f.recursiveSubcallTimeout,
		maxCodeBytes:            f.maxCodeBytes,
		allowedImports:          cloneImportSet(f.allowedImports),
		imported:                make(map[string]struct{}, len(f.allowedImports)),
		runID:                   options.RunID,
		nodeID:                  options.NodeID,
		nodeDepth:               options.Depth,
	}
	exports := interp.Exports{
		"sigil/repl/repl": map[string]reflect.Value{
			"LLMQuery": reflect.ValueOf(func(prompt string, subContext string) (string, error) {
				execCtx, ctxErr := session.activeExecContext()
				if ctxErr != nil {
					return "", ctxErr
				}
				answer, err := options.LLMQuery(execCtx, QueryRequest{Prompt: prompt, Context: subContext})
				if err != nil {
					if IsFatalExecution(err) {
						session.cancelCurrentExec()
						panic(UnwrapFatalExecution(err))
					}
					if _, ok := CodeOf(err); ok {
						return "", err
					}
					return "", WrapError(ErrorCodeSubcallInference, "llm_query failed", err)
				}
				return answer, nil
			}),
			"RLMQuery": reflect.ValueOf(func(prompt string, subContext string) (string, error) {
				subcallCtx, cancel, ctxErr := session.newRecursiveSubcallContext()
				if ctxErr != nil {
					return "", ctxErr
				}
				defer cancel()
				answer, err := options.RLMQuery(subcallCtx, QueryRequest{Prompt: prompt, Context: subContext})
				if err != nil {
					if IsFatalExecution(err) {
						session.cancelCurrentExec()
						panic(UnwrapFatalExecution(err))
					}
					if _, ok := CodeOf(err); ok {
						return "", err
					}
					return "", WrapError(ErrorCodeSubcallInference, "rlm_query failed", err)
				}
				return answer, nil
			}),
			"LLMQueryBatched": reflect.ValueOf(func(calls []map[string]string) ([]map[string]string, error) {
				execCtx, ctxErr := session.activeExecContext()
				if ctxErr != nil {
					return nil, ctxErr
				}
				requests, err := ParseBatchedCalls(calls)
				if err != nil {
					return nil, WrapError(ErrorCodeSubcallInvalidInput, "llm_query_batched input is invalid", err)
				}
				results, err := options.LLMQueryBatched(execCtx, requests)
				if err != nil {
					if IsFatalExecution(err) {
						session.cancelCurrentExec()
						panic(UnwrapFatalExecution(err))
					}
					if _, ok := CodeOf(err); ok {
						return nil, err
					}
					return nil, WrapError(ErrorCodeSubcallInference, "llm_query_batched failed", err)
				}
				if len(results) != len(requests) {
					return nil, WrapError(ErrorCodeSubcallInference, "llm_query_batched returned mismatched result length", nil)
				}
				return EncodeBatchedResults(results), nil
			}),
			"RLMQueryBatched": reflect.ValueOf(func(calls []map[string]string) ([]map[string]string, error) {
				subcallCtx, cancel, ctxErr := session.newRecursiveSubcallContext()
				if ctxErr != nil {
					return nil, ctxErr
				}
				defer cancel()
				requests, err := ParseBatchedCalls(calls)
				if err != nil {
					return nil, WrapError(ErrorCodeSubcallInvalidInput, "rlm_query_batched input is invalid", err)
				}
				results, err := options.RLMQueryBatched(subcallCtx, requests)
				if err != nil {
					if IsFatalExecution(err) {
						session.cancelCurrentExec()
						panic(UnwrapFatalExecution(err))
					}
					if _, ok := CodeOf(err); ok {
						return nil, err
					}
					return nil, WrapError(ErrorCodeSubcallInference, "rlm_query_batched failed", err)
				}
				if len(results) != len(requests) {
					return nil, WrapError(ErrorCodeSubcallInference, "rlm_query_batched returned mismatched result length", nil)
				}
				return EncodeBatchedResults(results), nil
			}),
			"ReadActionOutput": reflect.ValueOf(func(outputRef string) (ActionOutput, error) {
				if _, ctxErr := session.activeExecContext(); ctxErr != nil {
					return ActionOutput{}, ctxErr
				}
				output, err := options.ReadActionOutput(outputRef)
				if err != nil {
					if _, ok := CodeOf(err); ok {
						return ActionOutput{}, err
					}
					return ActionOutput{}, WrapError(
						ErrorCodeActionOutputRead,
						fmt.Sprintf("read_action_output failed for %q", outputRef),
						err,
					)
				}
				return output, nil
			}),
		},
	}
	if err := interpreter.Use(exports); err != nil {
		return nil, WrapError(ErrorCodeSessionInit, "failed to register repl exports", err)
	}

	if _, err := interpreter.Eval(`import . "sigil/repl"`); err != nil {
		return nil, WrapError(ErrorCodeSessionInit, "failed to import repl exports", err)
	}
	if _, err := interpreter.Eval(`var llm_query = LLMQuery`); err != nil {
		return nil, WrapError(ErrorCodeSessionInit, "failed to expose llm_query binding", err)
	}
	if _, err := interpreter.Eval(`var rlm_query = RLMQuery`); err != nil {
		return nil, WrapError(ErrorCodeSessionInit, "failed to expose rlm_query binding", err)
	}
	if _, err := interpreter.Eval(`var llm_query_batched = LLMQueryBatched`); err != nil {
		return nil, WrapError(ErrorCodeSessionInit, "failed to expose llm_query_batched binding", err)
	}
	if _, err := interpreter.Eval(`var rlm_query_batched = RLMQueryBatched`); err != nil {
		return nil, WrapError(ErrorCodeSessionInit, "failed to expose rlm_query_batched binding", err)
	}
	if _, err := interpreter.Eval(`var read_action_output = ReadActionOutput`); err != nil {
		return nil, WrapError(ErrorCodeSessionInit, "failed to expose read_action_output binding", err)
	}
	if _, err := interpreter.Eval(`var context string`); err != nil {
		return nil, WrapError(ErrorCodeSessionInit, "failed to declare context binding", err)
	}
	if _, err := interpreter.Eval("context = " + strconv.Quote(options.Context)); err != nil {
		return nil, WrapError(ErrorCodeSessionInit, "failed to initialize context binding", err)
	}

	factoryLogger().Info("initialized yaegi repl session",
		"run_id", options.RunID,
		"node_id", options.NodeID,
		"node_depth", options.Depth,
		"action_timeout", f.actionTimeout.String(),
		"recursive_subcall_timeout", f.recursiveSubcallTimeout.String(),
		"max_code_bytes", f.maxCodeBytes,
		"stdout_cap_bytes", f.stdoutCapBytes,
		"stderr_cap_bytes", f.stderrCapBytes,
		"allowlist_import_count", len(f.allowedImports),
	)

	return session, nil
}

type yaegiSession struct {
	mu                      sync.Mutex
	ctxMu                   sync.RWMutex
	interpreter             *interp.Interpreter
	stdout                  *cappedBuffer
	stderr                  *cappedBuffer
	runContextSource        context.Context
	currentExecContext      context.Context
	currentExecCancel       context.CancelFunc
	currentRunContext       context.Context
	closed                  bool
	actionTimeout           time.Duration
	recursiveSubcallTimeout time.Duration
	maxCodeBytes            int
	allowedImports          map[string]struct{}
	imported                map[string]struct{}
	runID                   string
	nodeID                  string
	nodeDepth               int
}

func (s *yaegiSession) Exec(ctx context.Context, code string) (ExecResult, error) {
	s.mu.Lock()
	defer s.mu.Unlock()

	logger := factoryLogger().With(
		"run_id", s.runID,
		"node_id", s.nodeID,
		"node_depth", s.nodeDepth,
	)

	if s.closed {
		logger.Warn("repl execution rejected because session is closed")
		return ExecResult{}, ErrSessionClosed
	}

	codeBytes := len([]byte(code))
	if codeBytes > s.maxCodeBytes {
		logger.Warn("repl code exceeded max size",
			"code_bytes", codeBytes,
			"max_code_bytes", s.maxCodeBytes,
		)
		return ExecResult{}, WrapError(ErrorCodeCodeSizeExceeded, fmt.Sprintf("repl_code exceeds max bytes (%d > %d)", codeBytes, s.maxCodeBytes), nil)
	}

	imports, err := extractImports(code)
	if err != nil {
		logger.Warn("repl import extraction failed", "error", err)
		return ExecResult{}, WrapError(ErrorCodeImportBlocked, "failed to parse imports", err)
	}
	for _, importPath := range imports {
		if _, ok := s.allowedImports[importPath]; !ok {
			logger.Warn("blocked repl import",
				"import_path", importPath,
			)
			return ExecResult{}, WrapError(ErrorCodeImportBlocked, fmt.Sprintf("import %q is not allowed", importPath), nil)
		}
	}
	logger.Debug("executing repl code",
		"code_bytes", codeBytes,
		"import_count", len(imports),
	)

	s.stdout.Reset()
	s.stderr.Reset()

	execCtx, cancel := context.WithTimeout(ctx, s.actionTimeout)
	defer cancel()
	runCtx := s.runContextSource
	if runCtx == nil {
		runCtx = ctx
	}
	if runCtx == nil {
		runCtx = context.Background()
	}
	s.setCurrentRunContext(runCtx)
	s.setCurrentExecContext(execCtx, cancel)
	defer func() {
		s.setCurrentExecContext(nil, nil)
		s.setCurrentRunContext(nil)
	}()

	start := time.Now().UTC()
	var evalErr error
	for _, importPath := range imports {
		if _, imported := s.imported[importPath]; imported {
			continue
		}
		_, evalErr = s.interpreter.EvalWithContext(execCtx, fmt.Sprintf(`import %q`, importPath))
		if evalErr != nil {
			break
		}
		s.imported[importPath] = struct{}{}
	}
	if evalErr == nil {
		body := stripImportDecls(code)
		if body != "" {
			_, evalErr = s.interpreter.EvalWithContext(execCtx, body)
		}
	}

	durationMS := int(time.Since(start).Milliseconds())
	if durationMS < 0 {
		durationMS = 0
	}

	result := ExecResult{
		Stdout:     s.stdout.String(),
		Stderr:     s.stderr.String(),
		DurationMS: durationMS,
	}

	if evalErr == nil {
		logger.Info("repl execution completed",
			"duration_ms", result.DurationMS,
			"stdout_bytes", len(result.Stdout),
			"stderr_bytes", len(result.Stderr),
			"stdout_truncated", s.stdout.truncated,
			"stderr_truncated", s.stderr.truncated,
		)
		return result, nil
	}
	if errors.Is(evalErr, context.DeadlineExceeded) || errors.Is(evalErr, context.Canceled) || errors.Is(execCtx.Err(), context.DeadlineExceeded) || errors.Is(execCtx.Err(), context.Canceled) {
		logger.Warn("repl execution timed out",
			"duration_ms", result.DurationMS,
			"timeout", s.actionTimeout.String(),
		)
		return result, WrapError(ErrorCodeExecutionTimeout, "repl execution timed out", evalErr)
	}

	classification := classifyEvalError(evalErr)
	if classification == ErrorCodeExecutionRuntime {
		logger.Warn("repl runtime error",
			"duration_ms", result.DurationMS,
			"error", evalErr,
		)
		return result, WrapError(ErrorCodeExecutionRuntime, "repl execution failed at runtime", evalErr)
	}

	logger.Warn("repl compile error",
		"duration_ms", result.DurationMS,
		"error", evalErr,
	)
	return result, WrapError(ErrorCodeExecutionCompile, "repl execution failed at compile stage", evalErr)
}

func (s *yaegiSession) Close() error {
	s.mu.Lock()
	defer s.mu.Unlock()

	s.closed = true
	s.setCurrentExecContext(nil, nil)
	s.setCurrentRunContext(nil)
	factoryLogger().Info("closed yaegi repl session",
		"run_id", s.runID,
		"node_id", s.nodeID,
		"node_depth", s.nodeDepth,
	)
	return nil
}

func classifyEvalError(err error) ErrorCode {
	message := strings.ToLower(err.Error())
	if strings.Contains(message, "panic") || strings.Contains(message, "runtime error") {
		return ErrorCodeExecutionRuntime
	}
	return ErrorCodeExecutionCompile
}

func (s *yaegiSession) activeExecContext() (context.Context, error) {
	s.ctxMu.RLock()
	execCtx := s.currentExecContext
	s.ctxMu.RUnlock()

	if execCtx == nil {
		return nil, WrapError(ErrorCodeExecutionTimeout, "repl action context is unavailable", context.Canceled)
	}
	if err := execCtx.Err(); err != nil {
		return nil, WrapError(ErrorCodeExecutionTimeout, "repl action context is canceled", err)
	}

	return execCtx, nil
}

func (s *yaegiSession) setCurrentExecContext(ctx context.Context, cancel context.CancelFunc) {
	s.ctxMu.Lock()
	s.currentExecContext = ctx
	s.currentExecCancel = cancel
	s.ctxMu.Unlock()
}

func (s *yaegiSession) cancelCurrentExec() {
	s.ctxMu.RLock()
	cancel := s.currentExecCancel
	s.ctxMu.RUnlock()
	if cancel != nil {
		cancel()
	}
}

func (s *yaegiSession) activeRunContext() (context.Context, error) {
	s.ctxMu.RLock()
	runCtx := s.currentRunContext
	s.ctxMu.RUnlock()

	if runCtx == nil {
		return nil, WrapError(ErrorCodeExecutionTimeout, "repl run context is unavailable", context.Canceled)
	}
	if err := runCtx.Err(); err != nil {
		return nil, WrapError(ErrorCodeExecutionTimeout, "repl run context is canceled", err)
	}

	return runCtx, nil
}

func (s *yaegiSession) newRecursiveSubcallContext() (context.Context, context.CancelFunc, error) {
	runCtx, err := s.activeRunContext()
	if err != nil {
		return nil, nil, err
	}

	subcallCtx, cancel := context.WithTimeout(runCtx, s.recursiveSubcallTimeout)
	return subcallCtx, cancel, nil
}

func (s *yaegiSession) setCurrentRunContext(ctx context.Context) {
	s.ctxMu.Lock()
	s.currentRunContext = ctx
	s.ctxMu.Unlock()
}

type cappedBuffer struct {
	limit     int
	buffer    bytes.Buffer
	truncated bool
}

func newCappedBuffer(limit int) *cappedBuffer {
	return &cappedBuffer{limit: limit}
}

func (b *cappedBuffer) Reset() {
	b.buffer.Reset()
	b.truncated = false
}

func (b *cappedBuffer) Write(data []byte) (int, error) {
	if b.limit <= 0 {
		b.truncated = true
		return len(data), nil
	}

	remaining := b.limit - b.buffer.Len()
	if remaining <= 0 {
		b.truncated = true
		return len(data), nil
	}

	if len(data) <= remaining {
		_, err := b.buffer.Write(data)
		return len(data), err
	}

	_, err := b.buffer.Write(data[:remaining])
	b.truncated = true
	if err != nil {
		return 0, err
	}

	return len(data), nil
}

func (b *cappedBuffer) String() string {
	output := b.buffer.String()
	if !b.truncated {
		return output
	}
	return output + OutputTruncationMarker
}

func cloneImportSet(source map[string]struct{}) map[string]struct{} {
	cloned := make(map[string]struct{}, len(source))
	for key := range source {
		cloned[key] = struct{}{}
	}
	return cloned
}

func factoryLogger() *slog.Logger {
	return slog.Default().With("component", "repl.yaegi")
}
