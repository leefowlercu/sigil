package harness

import (
	"context"
	"sync"

	"github.com/leefowlercu/sigil/internal/accounting"
	"github.com/leefowlercu/sigil/internal/config"
	"github.com/leefowlercu/sigil/internal/inference"
	"github.com/leefowlercu/sigil/internal/repl"
	"github.com/leefowlercu/sigil/internal/runtime"
)

// RunInput defines one blocking harness execution request.
type RunInput struct {
	AppConfigPath string
	RunConfigPath string
	RunConfig     config.RunConfig
	TemplateVars  map[string]string
	QueuedSource  runtime.RunQueuedSource

	ProcessMetadataSource  string
	SubmittedRunConfigYAML *string
}

// RunResult defines canonical run-start success output contract.
type RunResult struct {
	RunID          string            `json:"run_id"`
	State          string            `json:"state"`
	FinalAnswer    string            `json:"final_answer"`
	FinalAnswerRef string            `json:"final_answer_ref"`
	EventsPath     string            `json:"events_path"`
	Accounting     accounting.Rollup `json:"accounting"`
}

// RunCompletion captures one asynchronous run outcome.
type RunCompletion struct {
	Result RunResult
	Err    error
}

// StartedRun exposes one accepted durable run plus asynchronous completion state.
type StartedRun struct {
	RunID                 string
	EventsPath            string
	AsOfSeq               int64
	SubmittedRunConfigRef *string
	done                  <-chan RunCompletion
	cancel                context.CancelCauseFunc
	launch                func()
	launchOnce            sync.Once
}

// Done returns the asynchronous completion stream for one started run.
func (s *StartedRun) Done() <-chan RunCompletion {
	if s == nil {
		return nil
	}
	return s.done
}

// Cancel requests graceful interruption for one started run.
func (s *StartedRun) Cancel(cause error) {
	if s == nil || s.cancel == nil {
		return
	}
	s.cancel(cause)
}

// Launch begins background execution for one accepted run.
func (s *StartedRun) Launch() {
	if s == nil || s.launch == nil {
		return
	}
	s.launchOnce.Do(s.launch)
}

// InferenceClient is the minimal inference interface required by runner.
type InferenceClient interface {
	Infer(ctx context.Context, request inference.Request) (inference.Result, error)
}

// InferenceFactory builds an inference client from run configuration.
type InferenceFactory func(runConfig config.RunConfig) (InferenceClient, error)

// Runner executes blocking harness runs for `sigil run start`.
type Runner struct {
	runsBaseDir      string
	templateRenderer TemplateRenderer
	promptResolver   *SystemPromptResolver
	replFactory      repl.SessionFactory
	inferenceFactory InferenceFactory
	eventObservers   []runtime.EventObserver
}

// RunnerOption mutates runner construction behavior.
type RunnerOption func(*Runner)

// WithRunsBaseDir overrides default run storage base directory.
func WithRunsBaseDir(path string) RunnerOption {
	return func(r *Runner) {
		r.runsBaseDir = path
	}
}

// WithTemplateRenderer overrides template renderer dependency.
func WithTemplateRenderer(renderer TemplateRenderer) RunnerOption {
	return func(r *Runner) {
		r.templateRenderer = renderer
	}
}

// WithREPLSessionFactory overrides REPL session factory dependency.
func WithREPLSessionFactory(factory repl.SessionFactory) RunnerOption {
	return func(r *Runner) {
		r.replFactory = factory
	}
}

// WithInferenceFactory overrides inference client construction.
func WithInferenceFactory(factory InferenceFactory) RunnerOption {
	return func(r *Runner) {
		r.inferenceFactory = factory
	}
}

// WithEventObserver registers one lifecycle event observer for the run.
func WithEventObserver(observer runtime.EventObserver) RunnerOption {
	return func(r *Runner) {
		if observer == nil {
			return
		}
		r.eventObservers = append(r.eventObservers, observer)
	}
}

// AddEventObserver registers one lifecycle event observer on an existing runner.
func (r *Runner) AddEventObserver(observer runtime.EventObserver) {
	if r == nil || observer == nil {
		return
	}
	r.eventObservers = append(r.eventObservers, observer)
}

// NewRunner constructs a runner with v1 defaults.
func NewRunner(options ...RunnerOption) *Runner {
	runner := &Runner{
		runsBaseDir:      runtime.DefaultRunsBaseDir,
		templateRenderer: NewStrictTemplateRenderer(),
		promptResolver:   NewSystemPromptResolver(),
		replFactory:      repl.NewFactory(),
		inferenceFactory: newDefaultInferenceClient,
	}

	for _, option := range options {
		if option != nil {
			option(runner)
		}
	}

	return runner
}
