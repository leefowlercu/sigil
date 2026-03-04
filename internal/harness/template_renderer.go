package harness

import (
	"bytes"
	"fmt"
	"strings"
	"text/template"

	"github.com/leefowlercu/sigil/internal/config"
)

// TemplateRenderer renders run prompt/context templates.
type TemplateRenderer interface {
	Render(templateText string, variables map[string]string) (string, error)
}

// StrictTemplateRenderer renders templates with missingkey=error semantics.
type StrictTemplateRenderer struct{}

// NewStrictTemplateRenderer constructs the v1 strict renderer.
func NewStrictTemplateRenderer() *StrictTemplateRenderer {
	return &StrictTemplateRenderer{}
}

// Render applies Go text/template rendering with strict missing-key behavior.
func (r *StrictTemplateRenderer) Render(templateText string, variables map[string]string) (string, error) {
	if r == nil {
		return "", fmt.Errorf("template renderer is required")
	}
	if strings.TrimSpace(templateText) == "" {
		return "", fmt.Errorf("template text is required")
	}

	tmpl, err := template.New("sigil").Option("missingkey=error").Parse(templateText)
	if err != nil {
		return "", fmt.Errorf("failed to parse template; %w", err)
	}

	data := make(map[string]string, len(variables))
	for key, value := range variables {
		data[key] = value
	}

	var buffer bytes.Buffer
	if err := tmpl.Execute(&buffer, data); err != nil {
		return "", fmt.Errorf("failed to render template; %w", err)
	}

	output := buffer.String()
	if strings.TrimSpace(output) == "" {
		return "", fmt.Errorf("rendered template output is empty")
	}

	return output, nil
}

func resolvePromptAndContext(runConfig config.RunConfig, variables map[string]string, renderer TemplateRenderer) (string, string, error) {
	if renderer == nil {
		return "", "", fmt.Errorf("template renderer is required")
	}

	prompt := runConfig.Prompt
	if strings.TrimSpace(prompt) == "" {
		rendered, err := renderer.Render(runConfig.PromptTemplate, variables)
		if err != nil {
			return "", "", fmt.Errorf("failed to render prompt_template; %w", err)
		}
		prompt = rendered
	}

	contextValue := runConfig.Context
	if strings.TrimSpace(contextValue) == "" {
		rendered, err := renderer.Render(runConfig.ContextTemplate, variables)
		if err != nil {
			return "", "", fmt.Errorf("failed to render context_template; %w", err)
		}
		contextValue = rendered
	}

	if strings.TrimSpace(prompt) == "" {
		return "", "", fmt.Errorf("effective prompt is empty")
	}
	if strings.TrimSpace(contextValue) == "" {
		return "", "", fmt.Errorf("effective context is empty")
	}

	return prompt, contextValue, nil
}
