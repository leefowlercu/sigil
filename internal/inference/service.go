package inference

import (
	"context"
	"errors"
	"fmt"
	"strings"

	"github.com/leefowlercu/sigil/internal/inference/schema"
)

// Service orchestrates schema-aware gateway inference.
type Service struct {
	registry *Registry
	schemas  *schema.Registry
	retry    RetryPolicy
}

// ServiceOption mutates Service construction options.
type ServiceOption func(service *Service)

// WithRetryPolicy sets the retry policy used by inference execution.
func WithRetryPolicy(policy RetryPolicy) ServiceOption {
	return func(service *Service) {
		service.retry = policy
	}
}

// NewService creates an inference service with registry and schema dependencies.
func NewService(registry *Registry, schemas *schema.Registry, options ...ServiceOption) (*Service, error) {
	if registry == nil {
		return nil, fmt.Errorf("registry is required")
	}
	if schemas == nil {
		return nil, fmt.Errorf("schema registry is required")
	}

	service := &Service{
		registry: registry,
		schemas:  schemas,
		retry:    DefaultRetryPolicy(),
	}

	for _, option := range options {
		if option != nil {
			option(service)
		}
	}
	service.retry = NormalizeRetryPolicy(service.retry)

	return service, nil
}

// Infer executes gateway inference and returns canonical normalized output.
func (s *Service) Infer(ctx context.Context, request Request) (Result, error) {
	schemaDefinition, err := s.schemas.Resolve(request.SchemaID)
	if err != nil {
		return Result{}, WrapError(ErrorCodeSchemaLookup, "failed to resolve inference schema", err)
	}

	gateway, err := s.registry.Resolve(request)
	if err != nil {
		return Result{}, WrapError(ErrorCodeGatewayResolution, "failed to resolve inference gateway", err)
	}

	gatewayRequest := GatewayRequest{
		Prompt:    request.Prompt,
		Context:   request.Context,
		Provider:  request.Provider,
		Model:     request.Model,
		Schema:    schemaDefinition,
		Reasoning: request.Reasoning,
	}

	maxAttempts := s.retry.MaxAttempts
	if maxAttempts <= 0 {
		maxAttempts = defaultMaxAttempts
	}

	var lastErr error
	for attempt := 1; attempt <= maxAttempts; attempt++ {
		response, runErr := gateway.Infer(ctx, gatewayRequest)
		if runErr == nil {
			result, normalizeErr := s.normalizeResult(request, response, schemaDefinition)
			if normalizeErr != nil {
				return Result{}, normalizeErr
			}
			return result, nil
		}

		lastErr = runErr
		var reasoningErr *ReasoningCapabilityError
		if errors.As(runErr, &reasoningErr) {
			return Result{}, WrapError(ErrorCodeReasoningCapability, "reasoning capability mismatch", runErr)
		}

		if !isTransientGatewayFailure(runErr) || attempt >= maxAttempts {
			break
		}

		delay := s.retry.DelayForAttempt(attempt)
		if sleepErr := s.retry.Sleep(ctx, delay); sleepErr != nil {
			return Result{}, WrapError(ErrorCodeGatewayFailure, "inference retry interrupted", sleepErr)
		}
	}

	return Result{}, WrapError(ErrorCodeGatewayFailure, "gateway inference failed", lastErr)
}

func (s *Service) normalizeResult(request Request, response GatewayResponse, schemaDefinition schema.Definition) (Result, error) {
	if response.StructuredPayload == nil {
		return Result{}, WrapError(ErrorCodeOutputValidation, "gateway response missing structured payload", nil)
	}

	if err := schemaDefinition.Validate(response.StructuredPayload); err != nil {
		return Result{}, WrapError(ErrorCodeOutputValidation, "structured output failed strict schema validation", err)
	}

	reasoning := response.Reasoning
	reasoning.Enabled = request.Reasoning.Enabled
	if request.Reasoning.Enabled {
		effort := strings.TrimSpace(request.Reasoning.Effort)
		reasoning.Effort = &effort
	} else {
		reasoning.Effort = nil
	}

	return Result{
		SchemaID:          request.SchemaID,
		ValidatedPayload:  cloneMap(response.StructuredPayload),
		Gateway:           request.Gateway,
		Provider:          firstNonEmpty(response.Provider, request.Provider),
		Model:             firstNonEmpty(response.Model, request.Model),
		GatewayResponseID: response.GatewayResponseID,
		Usage:             response.Usage,
		Reasoning: Reasoning{
			Enabled:   reasoning.Enabled,
			Effort:    cloneStringPointer(reasoning.Effort),
			Artifacts: cloneMap(reasoning.Artifacts),
		},
		FinishStatus: firstNonEmpty(response.FinishStatus, "completed"),
		RawMetadata:  cloneMap(response.RawMetadata),
	}, nil
}

func isTransientGatewayFailure(err error) bool {
	var httpErr *GatewayHTTPError
	if !errors.As(err, &httpErr) {
		return false
	}

	statusCode := httpErr.StatusCode
	if statusCode == 429 {
		return true
	}

	return statusCode >= 500 && statusCode <= 599
}

func firstNonEmpty(values ...string) string {
	for _, value := range values {
		trimmed := strings.TrimSpace(value)
		if trimmed != "" {
			return trimmed
		}
	}

	return ""
}

func cloneMap(input map[string]any) map[string]any {
	if input == nil {
		return nil
	}

	cloned := make(map[string]any, len(input))
	for key, value := range input {
		cloned[key] = value
	}

	return cloned
}

func cloneStringPointer(value *string) *string {
	if value == nil {
		return nil
	}

	copied := *value
	return &copied
}
