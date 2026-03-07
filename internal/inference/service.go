package inference

import (
	"context"
	"errors"
	"fmt"
	"log/slog"
	"strings"

	"github.com/leefowlercu/sigil/internal/accounting"
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
	if err := validateMessages(request.Messages); err != nil {
		return Result{}, WrapError(ErrorCodeGatewayResolution, "invalid inference messages", err)
	}

	logger := inferenceServiceLogger().With(
		"gateway", request.Gateway,
		"provider", request.Provider,
		"model", request.Model,
		"schema_id", request.SchemaID,
	)
	logger.Info("starting inference request",
		"message_count", len(request.Messages),
		"message_bytes", totalMessageBytes(request.Messages),
		"reasoning_enabled", request.Reasoning.Enabled,
		"reasoning_effort", request.Reasoning.Effort,
	)

	schemaDefinition, err := s.schemas.Resolve(request.SchemaID)
	if err != nil {
		logger.Error("failed to resolve schema definition", "error", err)
		return Result{}, WrapError(ErrorCodeSchemaLookup, "failed to resolve inference schema", err)
	}

	gateway, err := s.registry.Resolve(request)
	if err != nil {
		logger.Error("failed to resolve gateway adapter", "error", err)
		return Result{}, WrapError(ErrorCodeGatewayResolution, "failed to resolve inference gateway", err)
	}

	gatewayRequest := GatewayRequest{
		Messages:  cloneMessages(request.Messages),
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
		logger.Debug("executing inference attempt", "attempt", attempt, "max_attempts", maxAttempts)
		response, runErr := gateway.Infer(ctx, gatewayRequest)
		if runErr == nil {
			result, normalizeErr := s.normalizeResult(request, response, schemaDefinition)
			if normalizeErr != nil {
				logger.Error("failed to normalize gateway response", "attempt", attempt, "error", normalizeErr)
				return Result{}, normalizeErr
			}
			logger.Info("inference request completed",
				"attempt", attempt,
				"gateway_response_id", result.GatewayResponseID,
				"finish_status", result.FinishStatus,
			)
			return result, nil
		}

		lastErr = runErr
		var reasoningErr *ReasoningCapabilityError
		if errors.As(runErr, &reasoningErr) {
			logger.Warn("reasoning capability mismatch", "attempt", attempt, "error", runErr)
			return Result{}, WrapError(ErrorCodeReasoningCapability, "reasoning capability mismatch", runErr)
		}

		if !isTransientGatewayFailure(runErr) || attempt >= maxAttempts {
			logger.Error("inference attempt failed without retry",
				"attempt", attempt,
				"retryable", isTransientGatewayFailure(runErr),
				"error", runErr,
			)
			break
		}

		delay := s.retry.DelayForAttempt(attempt)
		logger.Warn("transient inference failure; retrying",
			"attempt", attempt,
			"max_attempts", maxAttempts,
			"retry_delay", delay.String(),
			"error", runErr,
		)
		if sleepErr := s.retry.Sleep(ctx, delay); sleepErr != nil {
			logger.Error("inference retry interrupted", "attempt", attempt, "error", sleepErr)
			return Result{}, WrapError(ErrorCodeGatewayFailure, "inference retry interrupted", sleepErr)
		}
	}

	logger.Error("inference request failed",
		"max_attempts", maxAttempts,
		"error", lastErr,
	)
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
		Accounting:        normalizeAccountingSample(request, response),
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

func normalizeAccountingSample(request Request, response GatewayResponse) AccountingSample {
	provider := firstNonEmpty(response.Provider, request.Provider)
	model := firstNonEmpty(response.Model, request.Model)
	pricingVersion := strings.TrimSpace(request.Accounting.PricingVersion)

	inputTokens := cloneInt64Pointer(response.Accounting.InputTokens)
	outputTokens := cloneInt64Pointer(response.Accounting.OutputTokens)
	totalTokens := cloneInt64Pointer(response.Accounting.TotalTokens)
	reasoningTokens := cloneInt64Pointer(response.Accounting.ReasoningTokens)
	hasAccountingTokens := inputTokens != nil || outputTokens != nil || totalTokens != nil || reasoningTokens != nil
	hasUsageReport := response.UsageReported || response.Usage.ReasoningTokens != nil ||
		response.Usage.InputTokens > 0 || response.Usage.OutputTokens > 0 || response.Usage.TotalTokens > 0
	allowZeroUsageBackfill := !hasAccountingTokens
	if inputTokens == nil {
		inputTokens = knownUsagePointer(response.Usage.InputTokens, hasUsageReport, allowZeroUsageBackfill)
	}
	if outputTokens == nil {
		outputTokens = knownUsagePointer(response.Usage.OutputTokens, hasUsageReport, allowZeroUsageBackfill)
	}
	if totalTokens == nil {
		totalTokens = knownUsagePointer(response.Usage.TotalTokens, hasUsageReport, allowZeroUsageBackfill)
		if totalTokens == nil && inputTokens != nil && outputTokens != nil {
			totalTokens = int64Pointer(*inputTokens + *outputTokens)
		}
	}
	if reasoningTokens == nil {
		reasoningTokens = cloneInt64Pointer(response.Usage.ReasoningTokens)
	}

	hasUsage := inputTokens != nil || outputTokens != nil || totalTokens != nil || reasoningTokens != nil
	hasGatewayCost := response.Accounting.KnownInputCostMicrousd != nil ||
		response.Accounting.KnownOutputCostMicrousd != nil ||
		response.Accounting.KnownReasoningCostMicrousd != nil ||
		response.Accounting.KnownTotalCostMicrousd != nil
	if !hasUsage && !hasGatewayCost {
		return accounting.UnavailableSummary(provider, model, pricingVersion)
	}

	summary := accounting.BuildLeafSummary(accounting.LeafInput{
		Provider:                     provider,
		Model:                        model,
		PricingVersion:               pricingVersion,
		ReasoningEnabled:             request.Reasoning.Enabled,
		InputTokens:                  inputTokens,
		OutputTokens:                 outputTokens,
		TotalTokens:                  totalTokens,
		ReasoningTokens:              reasoningTokens,
		GatewayInputCostMicrousd:     response.Accounting.KnownInputCostMicrousd,
		GatewayOutputCostMicrousd:    response.Accounting.KnownOutputCostMicrousd,
		GatewayReasoningCostMicrousd: response.Accounting.KnownReasoningCostMicrousd,
		GatewayTotalCostMicrousd:     response.Accounting.KnownTotalCostMicrousd,
		FallbackPricing:              request.Accounting.FallbackPricing,
	})
	preserveRequestedFallbackPricingKey(&summary, request)
	return summary
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

func cloneInt64Pointer(value *int64) *int64 {
	if value == nil {
		return nil
	}

	copied := *value
	return &copied
}

func int64Pointer(value int64) *int64 {
	copied := value
	return &copied
}

func knownUsagePointer(value int64, known bool, allowZero bool) *int64 {
	if !known || (value == 0 && !allowZero) {
		return nil
	}
	return int64Pointer(value)
}

func preserveRequestedFallbackPricingKey(summary *AccountingSample, request Request) {
	if summary == nil || request.Accounting.FallbackPricing == nil {
		return
	}
	if summary.CostSource != accounting.SourceFallbackPricing && summary.CostSource != accounting.SourceMixed {
		return
	}

	provider := strings.TrimSpace(request.Provider)
	model := strings.TrimSpace(request.Model)
	if provider != "" {
		summary.PricingKey.Provider = provider
	}
	if model != "" {
		summary.PricingKey.Model = model
	}
}

func validateMessages(messages []Message) error {
	if len(messages) == 0 {
		return fmt.Errorf("messages must contain at least one entry")
	}

	for index, message := range messages {
		switch message.Role {
		case MessageRoleSystem, MessageRoleUser, MessageRoleAssistant:
		default:
			return fmt.Errorf("messages[%d].role %q is not supported", index, message.Role)
		}
		if strings.TrimSpace(message.Content) == "" {
			return fmt.Errorf("messages[%d].content must be non-empty", index)
		}
	}

	return nil
}

func totalMessageBytes(messages []Message) int {
	total := 0
	for _, message := range messages {
		total += len(message.Content)
	}
	return total
}

func cloneMessages(messages []Message) []Message {
	if len(messages) == 0 {
		return nil
	}

	cloned := make([]Message, len(messages))
	copy(cloned, messages)
	return cloned
}

func inferenceServiceLogger() *slog.Logger {
	return slog.Default().With("component", "inference.service")
}
