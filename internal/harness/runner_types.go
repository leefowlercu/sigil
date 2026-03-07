package harness

import (
	"context"

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
