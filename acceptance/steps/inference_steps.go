package steps

import (
	"context"
	"errors"
	"fmt"
	"os"
	"strconv"
	"strings"
	"time"

	"github.com/cucumber/godog"
	sigilinference "github.com/leefowlercu/sigil/internal/inference"
	"github.com/leefowlercu/sigil/internal/inference/openrouter"
	sigilschema "github.com/leefowlercu/sigil/internal/inference/schema"
)

func registerInferenceSteps(ctx *godog.ScenarioContext, world *harnessWorld) {
	ctx.Step(`^a valid inference request for gateway resolution$`, world.aValidInferenceRequestForGatewayResolution)
	ctx.Step(`^a valid inference request for execution$`, world.aValidInferenceRequestForExecution)
	ctx.Step(`^central inference schema registry is initialized$`, world.centralInferenceSchemaRegistryIsInitialized)
	ctx.Step(`^central inference schema registry excludes schema_id "([^"]*)"$`, world.centralInferenceSchemaRegistryExcludesSchemaID)
	ctx.Step(`^inference request schema_id is "([^"]*)"$`, world.inferenceRequestSchemaIDIs)
	ctx.Step(`^inference reasoning is enabled with effort "([^"]*)"$`, world.inferenceReasoningIsEnabledWithEffort)
	ctx.Step(`^inference reasoning is disabled$`, world.inferenceReasoningIsDisabled)
	ctx.Step(`^openrouter mock gateway returns payload fixture "([^"]*)"$`, world.openrouterMockGatewayReturnsPayloadFixture)
	ctx.Step(`^openrouter mock gateway returns retry status sequence "([^"]*)"$`, world.openrouterMockGatewayReturnsRetryStatusSequence)
	ctx.Step(`^inference gateway resolution runs$`, world.inferenceGatewayResolutionRuns)
	ctx.Step(`^inference request construction runs$`, world.inferenceRequestConstructionRuns)
	ctx.Step(`^inference execution runs$`, world.inferenceExecutionRuns)
	ctx.Step(`^resolution occurs through gateway registry lookup$`, world.resolutionOccursThroughGatewayRegistryLookup)
	ctx.Step(`^request targets OpenRouter Responses API in non-streaming mode$`, world.requestTargetsOpenRouterResponsesAPIInNonStreamingMode)
	ctx.Step(`^request uses message-array input preserving role order$`, world.requestUsesMessagearrayInputPreservingRoleOrder)
	ctx.Step(`^schema is resolved from central registry and applied to request$`, world.schemaIsResolvedFromCentralRegistryAndAppliedToRequest)
	ctx.Step(`^strict json_schema structured output mode is required$`, world.strictJSONSchemaStructuredOutputModeIsRequired)
	ctx.Step(`^response healing plugin is enabled$`, world.responseHealingPluginIsEnabled)
	ctx.Step(`^reasoning config is included using configured effort "([^"]*)"$`, world.reasoningConfigIsIncludedUsingConfiguredEffort)
	ctx.Step(`^reasoning config is omitted$`, world.reasoningConfigIsOmitted)
	ctx.Step(`^runtime retries with bounded policy \(3 total attempts exponential backoff base 250ms jitter max 2s\)$`, world.runtimeRetriesWithBoundedPolicy)
	ctx.Step(`^inference fails with typed error code "([^"]*)"$`, world.inferenceFailsWithTypedErrorCode)
	ctx.Step(`^normalized output contains all required canonical fields$`, world.normalizedOutputContainsAllRequiredCanonicalFields)
	ctx.Step(`^decision discriminator enforces continue or final$`, world.decisionDiscriminatorEnforcesContinueOrFinal)
	ctx.Step(`^continuation branch invariant is enforced$`, world.continuationBranchInvariantIsEnforced)
	ctx.Step(`^final branch invariant is enforced$`, world.finalBranchInvariantIsEnforced)
	ctx.Step(`^unknown fields are rejected with typed output-validation error$`, world.unknownFieldsAreRejectedWithTypedOutputValidationError)
	ctx.Step(`^reasoning artifacts are under top-level reasoning and reasoning token counts are under usage.reasoning_tokens$`, world.reasoningArtifactsAreUnderTopLevelReasoningAndReasoningTokenCountsAreUnderUsageReasoningTokens)
}

func (w *harnessWorld) aValidInferenceRequestForGatewayResolution() error {
	return w.initializeInferenceHarness()
}

func (w *harnessWorld) aValidInferenceRequestForExecution() error {
	return w.initializeInferenceHarness()
}

func (w *harnessWorld) centralInferenceSchemaRegistryIsInitialized() error {
	if err := w.initializeInferenceHarness(); err != nil {
		return err
	}

	w.inferenceSchemaRegistry = sigilschema.NewRegistry()
	return w.rebuildInferenceService()
}

func (w *harnessWorld) centralInferenceSchemaRegistryExcludesSchemaID(schemaID string) error {
	if err := w.centralInferenceSchemaRegistryIsInitialized(); err != nil {
		return err
	}

	w.inferenceSchemaRegistry = sigilschema.NewEmptyRegistry()
	w.inferenceRequest.SchemaID = strings.TrimSpace(schemaID)
	return w.rebuildInferenceService()
}

func (w *harnessWorld) inferenceRequestSchemaIDIs(schemaID string) error {
	if err := w.initializeInferenceHarness(); err != nil {
		return err
	}

	w.inferenceRequest.SchemaID = strings.TrimSpace(schemaID)
	return nil
}

func (w *harnessWorld) inferenceReasoningIsEnabledWithEffort(effort string) error {
	if err := w.initializeInferenceHarness(); err != nil {
		return err
	}

	w.inferenceRequest.Reasoning.Enabled = true
	w.inferenceRequest.Reasoning.Effort = strings.TrimSpace(effort)
	return nil
}

func (w *harnessWorld) inferenceReasoningIsDisabled() error {
	if err := w.initializeInferenceHarness(); err != nil {
		return err
	}

	w.inferenceRequest.Reasoning.Enabled = false
	return nil
}

func (w *harnessWorld) openrouterMockGatewayReturnsPayloadFixture(fixture string) error {
	if err := w.initializeInferenceHarness(); err != nil {
		return err
	}

	if err := w.ensureInferenceMockServer(); err != nil {
		return err
	}

	responses, err := responsesForFixture(strings.TrimSpace(fixture))
	if err != nil {
		return err
	}
	w.inferenceMockServer.SetResponses(responses...)
	return nil
}

func (w *harnessWorld) openrouterMockGatewayReturnsRetryStatusSequence(sequence string) error {
	if err := w.initializeInferenceHarness(); err != nil {
		return err
	}
	if err := w.ensureInferenceMockServer(); err != nil {
		return err
	}

	parts := strings.Split(sequence, ",")
	responses := make([]mockGatewayResponse, 0, len(parts))
	for index, part := range parts {
		statusCode, err := strconv.Atoi(strings.TrimSpace(part))
		if err != nil {
			return fmt.Errorf("invalid retry status %q; %w", part, err)
		}

		if statusCode == 200 {
			responses = append(responses, mockGatewayResponse{statusCode: 200, body: validFinalGatewayResponseBody()})
			continue
		}

		responses = append(responses, mockGatewayResponse{statusCode: statusCode, body: map[string]any{"error": fmt.Sprintf("transient-%d-%d", statusCode, index)}})
	}

	w.inferenceMockServer.SetResponses(responses...)
	return nil
}

func (w *harnessWorld) inferenceGatewayResolutionRuns() error {
	if err := w.initializeInferenceHarness(); err != nil {
		return err
	}
	if err := w.ensureInferenceMockServer(); err != nil {
		return err
	}
	w.inferenceResolvedErr = nil
	_, w.inferenceResolvedErr = w.inferenceGatewayRegistry.Resolve(w.inferenceRequest)
	w.inferenceRequestBody = w.inferenceMockServer.LastRequestBody()
	return nil
}

func (w *harnessWorld) inferenceRequestConstructionRuns() error {
	if err := w.inferenceExecutionRuns(); err != nil {
		return err
	}

	if w.inferenceErr != nil {
		return fmt.Errorf("expected request construction success, got %v", w.inferenceErr)
	}

	return nil
}

func (w *harnessWorld) inferenceExecutionRuns() error {
	if err := w.initializeInferenceHarness(); err != nil {
		return err
	}

	if strings.TrimSpace(w.inferenceRequest.SchemaID) == "" {
		w.inferenceRequest.SchemaID = sigilschema.SigilRLMResponseV1SchemaID
	}

	if w.inferenceMockServer == nil && w.inferenceRequest.SchemaID == sigilschema.SigilRLMResponseV1SchemaID {
		if err := w.openrouterMockGatewayReturnsPayloadFixture("valid-final"); err != nil {
			return err
		}
	}

	w.inferenceRetryDelays = nil
	w.inferenceResult = sigilinference.Result{}
	w.inferenceErr = nil

	result, err := w.inferenceService.Infer(context.Background(), w.inferenceRequest)
	w.inferenceResult = result
	w.inferenceErr = err
	if w.inferenceMockServer != nil {
		w.inferenceRequestBody = w.inferenceMockServer.LastRequestBody()
	}

	return nil
}

func (w *harnessWorld) resolutionOccursThroughGatewayRegistryLookup() error {
	if w.inferenceResolvedErr != nil {
		return fmt.Errorf("expected gateway registry resolution success, got %v", w.inferenceResolvedErr)
	}

	return nil
}

func (w *harnessWorld) requestTargetsOpenRouterResponsesAPIInNonStreamingMode() error {
	if w.inferenceMockServer == nil {
		return fmt.Errorf("expected openrouter mock server for request assertions")
	}
	if got := w.inferenceMockServer.LastRequestPath(); got != "/responses" {
		return fmt.Errorf("expected request path /responses, got %q", got)
	}
	if w.inferenceRequestBody == nil {
		return fmt.Errorf("expected captured request payload")
	}

	stream, ok := w.inferenceRequestBody["stream"].(bool)
	if !ok || stream {
		return fmt.Errorf("expected stream=false request payload, got %v", w.inferenceRequestBody["stream"])
	}

	return nil
}

func (w *harnessWorld) requestUsesMessagearrayInputPreservingRoleOrder() error {
	if w.inferenceRequestBody == nil {
		return fmt.Errorf("expected captured request payload")
	}

	input, ok := w.inferenceRequestBody["input"].([]any)
	if !ok {
		return fmt.Errorf("expected request input message array, got %T", w.inferenceRequestBody["input"])
	}
	if len(input) != 2 {
		return fmt.Errorf("expected exactly 2 ordered messages (system,user), got %d", len(input))
	}

	first := asMapValue(input[0])
	second := asMapValue(input[1])
	if role, _ := first["role"].(string); role != "system" {
		return fmt.Errorf("expected first message role system, got %q", role)
	}
	if role, _ := second["role"].(string); role != "user" {
		return fmt.Errorf("expected second message role user, got %q", role)
	}

	return nil
}

func (w *harnessWorld) schemaIsResolvedFromCentralRegistryAndAppliedToRequest() error {
	if w.inferenceSchemaRegistry == nil {
		return fmt.Errorf("expected schema registry initialization")
	}
	definition, err := w.inferenceSchemaRegistry.Resolve(w.inferenceRequest.SchemaID)
	if err != nil {
		return fmt.Errorf("expected schema resolve success from central registry; %w", err)
	}

	responseFormat := asMapValue(w.inferenceRequestBody["response_format"])
	jsonSchema := asMapValue(responseFormat["json_schema"])
	if jsonSchema == nil {
		return fmt.Errorf("expected response_format.json_schema block in request payload")
	}

	name, _ := jsonSchema["name"].(string)
	if name != definition.Name {
		return fmt.Errorf("expected request schema name %q, got %q", definition.Name, name)
	}
	if _, ok := jsonSchema["schema"].(map[string]any); !ok {
		return fmt.Errorf("expected request to include json schema object")
	}

	return nil
}

func (w *harnessWorld) strictJSONSchemaStructuredOutputModeIsRequired() error {
	responseFormat := asMapValue(w.inferenceRequestBody["response_format"])
	if responseFormat == nil {
		return fmt.Errorf("expected response_format block")
	}
	responseType, _ := responseFormat["type"].(string)
	if responseType != "json_schema" {
		return fmt.Errorf("expected response_format.type=json_schema, got %q", responseType)
	}

	jsonSchema := asMapValue(responseFormat["json_schema"])
	strict, _ := jsonSchema["strict"].(bool)
	if !strict {
		return fmt.Errorf("expected strict json_schema mode")
	}

	return nil
}

func (w *harnessWorld) responseHealingPluginIsEnabled() error {
	plugins, ok := w.inferenceRequestBody["plugins"].([]any)
	if !ok || len(plugins) == 0 {
		return fmt.Errorf("expected plugins block with response healing enabled")
	}
	plugin := asMapValue(plugins[0])
	pluginID, _ := plugin["id"].(string)
	if pluginID != "response-healing" {
		return fmt.Errorf("expected response-healing plugin id, got %q", pluginID)
	}

	return nil
}

func (w *harnessWorld) reasoningConfigIsIncludedUsingConfiguredEffort(expectedEffort string) error {
	reasoning := asMapValue(w.inferenceRequestBody["reasoning"])
	if reasoning == nil {
		return fmt.Errorf("expected reasoning block in request payload")
	}
	effort, _ := reasoning["effort"].(string)
	if effort != expectedEffort {
		return fmt.Errorf("expected reasoning effort %q, got %q", expectedEffort, effort)
	}

	return nil
}

func (w *harnessWorld) reasoningConfigIsOmitted() error {
	if _, exists := w.inferenceRequestBody["reasoning"]; exists {
		return fmt.Errorf("expected reasoning block to be omitted when disabled")
	}

	return nil
}

func (w *harnessWorld) runtimeRetriesWithBoundedPolicy() error {
	if w.inferenceMockServer == nil {
		return fmt.Errorf("expected openrouter mock server for retry assertions")
	}
	if w.inferenceMockServer.RequestCount() != 3 {
		return fmt.Errorf("expected 3 total attempts, got %d", w.inferenceMockServer.RequestCount())
	}
	if len(w.inferenceRetryDelays) != 2 {
		return fmt.Errorf("expected 2 retry delays, got %d", len(w.inferenceRetryDelays))
	}
	if w.inferenceRetryDelays[0] != 250*time.Millisecond {
		return fmt.Errorf("expected first retry delay 250ms, got %s", w.inferenceRetryDelays[0])
	}
	if w.inferenceRetryDelays[1] != 500*time.Millisecond {
		return fmt.Errorf("expected second retry delay 500ms, got %s", w.inferenceRetryDelays[1])
	}
	for _, delay := range w.inferenceRetryDelays {
		if delay > 2*time.Second {
			return fmt.Errorf("expected retry delay cap at 2s, got %s", delay)
		}
	}

	return nil
}

func (w *harnessWorld) inferenceFailsWithTypedErrorCode(expectedCode string) error {
	if w.inferenceErr == nil {
		return fmt.Errorf("expected inference failure with typed error code %q", expectedCode)
	}

	code := sigilinference.ErrorCode(strings.TrimSpace(expectedCode))
	if !sigilinference.IsCode(w.inferenceErr, code) {
		return fmt.Errorf("expected typed error code %q, got %v", expectedCode, w.inferenceErr)
	}

	return nil
}

func (w *harnessWorld) normalizedOutputContainsAllRequiredCanonicalFields() error {
	if w.inferenceErr != nil {
		return fmt.Errorf("expected successful normalized output, got %v", w.inferenceErr)
	}

	requiredStringFields := []struct {
		name  string
		value string
	}{
		{name: "schema_id", value: w.inferenceResult.SchemaID},
		{name: "gateway", value: w.inferenceResult.Gateway},
		{name: "provider", value: w.inferenceResult.Provider},
		{name: "model", value: w.inferenceResult.Model},
		{name: "gateway_response_id", value: w.inferenceResult.GatewayResponseID},
		{name: "finish_status", value: w.inferenceResult.FinishStatus},
	}
	for _, field := range requiredStringFields {
		if strings.TrimSpace(field.value) == "" {
			return fmt.Errorf("expected canonical field %s to be non-empty", field.name)
		}
	}

	if w.inferenceResult.ValidatedPayload == nil {
		return fmt.Errorf("expected canonical validated_payload field")
	}
	if w.inferenceResult.RawMetadata == nil {
		return fmt.Errorf("expected canonical raw_metadata field")
	}
	if w.inferenceRequest.SchemaID == sigilschema.SigilLLMAnswerV1SchemaID {
		if _, hasOutputText := w.inferenceResult.RawMetadata["output_text"]; hasOutputText {
			extraction := asMapValue(w.inferenceResult.RawMetadata["extraction"])
			if extraction == nil {
				return fmt.Errorf("expected extraction metadata for llm raw-text fallback response")
			}
			mode, _ := extraction["mode"].(string)
			if mode != "raw_text_fallback" {
				return fmt.Errorf("expected extraction.mode raw_text_fallback, got %q", mode)
			}
		}
	}

	return nil
}

func (w *harnessWorld) decisionDiscriminatorEnforcesContinueOrFinal() error {
	if err := w.inferenceFailsWithTypedErrorCode(string(sigilinference.ErrorCodeOutputValidation)); err != nil {
		return err
	}
	if !strings.Contains(w.inferenceErr.Error(), "decision") {
		return fmt.Errorf("expected decision discriminator validation failure, got %v", w.inferenceErr)
	}

	return nil
}

func (w *harnessWorld) continuationBranchInvariantIsEnforced() error {
	if err := w.inferenceFailsWithTypedErrorCode(string(sigilinference.ErrorCodeOutputValidation)); err != nil {
		return err
	}
	if !strings.Contains(w.inferenceErr.Error(), "decision=continue") {
		return fmt.Errorf("expected continuation branch invariant failure, got %v", w.inferenceErr)
	}

	return nil
}

func (w *harnessWorld) finalBranchInvariantIsEnforced() error {
	if err := w.inferenceFailsWithTypedErrorCode(string(sigilinference.ErrorCodeOutputValidation)); err != nil {
		return err
	}
	if !strings.Contains(w.inferenceErr.Error(), "decision=final") {
		return fmt.Errorf("expected final branch invariant failure, got %v", w.inferenceErr)
	}

	return nil
}

func (w *harnessWorld) unknownFieldsAreRejectedWithTypedOutputValidationError() error {
	if err := w.inferenceFailsWithTypedErrorCode(string(sigilinference.ErrorCodeOutputValidation)); err != nil {
		return err
	}
	if !strings.Contains(strings.ToLower(w.inferenceErr.Error()), "unknown") {
		return fmt.Errorf("expected unknown-field validation failure, got %v", w.inferenceErr)
	}

	return nil
}

func (w *harnessWorld) reasoningArtifactsAreUnderTopLevelReasoningAndReasoningTokenCountsAreUnderUsageReasoningTokens() error {
	if w.inferenceErr != nil {
		return fmt.Errorf("expected successful inference result for reasoning mapping assertion, got %v", w.inferenceErr)
	}
	if len(w.inferenceResult.Reasoning.Artifacts) == 0 {
		return fmt.Errorf("expected reasoning artifacts under top-level reasoning key")
	}
	if w.inferenceResult.Usage.ReasoningTokens == nil || *w.inferenceResult.Usage.ReasoningTokens == 0 {
		return fmt.Errorf("expected usage.reasoning_tokens to be populated")
	}

	return nil
}

func (w *harnessWorld) initializeInferenceHarness() error {
	if w.inferenceService != nil {
		return nil
	}

	testAPIKey := "test-openrouter-key"
	if err := osSetEnv("OPENROUTER_API_KEY", testAPIKey); err != nil {
		return err
	}

	w.inferenceSchemaRegistry = sigilschema.NewRegistry()
	w.inferenceGatewayRegistry = sigilinference.NewRegistry()
	if err := w.inferenceGatewayRegistry.Register("openrouter", openrouter.NewGateway); err != nil {
		return fmt.Errorf("failed to register openrouter gateway; %w", err)
	}

	service, err := sigilinference.NewService(
		w.inferenceGatewayRegistry,
		w.inferenceSchemaRegistry,
		sigilinference.WithRetryPolicy(sigilinference.RetryPolicy{
			MaxAttempts: 3,
			BaseDelay:   250 * time.Millisecond,
			MaxDelay:    2 * time.Second,
			Jitter: func(_ int, delay time.Duration) time.Duration {
				return delay
			},
			Sleep: func(_ context.Context, delay time.Duration) error {
				w.inferenceRetryDelays = append(w.inferenceRetryDelays, delay)
				return nil
			},
		}),
	)
	if err != nil {
		return fmt.Errorf("failed to initialize inference service; %w", err)
	}
	w.inferenceService = service
	w.inferenceRequest = sigilinference.Request{
		Messages: []sigilinference.Message{
			{Role: sigilinference.MessageRoleSystem, Content: "test system prompt"},
			{Role: sigilinference.MessageRoleUser, Content: "{\"query\":\"test prompt\"}"},
		},
		SchemaID: sigilschema.SigilRLMResponseV1SchemaID,
		Gateway:  "openrouter",
		Provider: "openai",
		Model:    "gpt-5.1",
		GatewayConfig: sigilinference.GatewayConfig{
			RequestTimeoutMS: 30000,
			APIKeyEnv:        "OPENROUTER_API_KEY",
		},
		Reasoning: sigilinference.ReasoningConfig{
			Enabled: true,
			Effort:  "medium",
		},
	}

	return nil
}

func (w *harnessWorld) rebuildInferenceService() error {
	if w.inferenceGatewayRegistry == nil {
		return fmt.Errorf("gateway registry is not initialized")
	}
	if w.inferenceSchemaRegistry == nil {
		return fmt.Errorf("schema registry is not initialized")
	}

	service, err := sigilinference.NewService(
		w.inferenceGatewayRegistry,
		w.inferenceSchemaRegistry,
		sigilinference.WithRetryPolicy(sigilinference.RetryPolicy{
			MaxAttempts: 3,
			BaseDelay:   250 * time.Millisecond,
			MaxDelay:    2 * time.Second,
			Jitter: func(_ int, delay time.Duration) time.Duration {
				return delay
			},
			Sleep: func(_ context.Context, delay time.Duration) error {
				w.inferenceRetryDelays = append(w.inferenceRetryDelays, delay)
				return nil
			},
		}),
	)
	if err != nil {
		return err
	}
	w.inferenceService = service
	return nil
}

func (w *harnessWorld) ensureInferenceMockServer() error {
	if w.inferenceMockServer != nil {
		return nil
	}

	w.inferenceMockServer = newOpenRouterMockServer()
	w.inferenceRequest.GatewayConfig.BaseURL = w.inferenceMockServer.URL()
	if err := w.rebuildInferenceService(); err != nil {
		return err
	}

	return nil
}

func responsesForFixture(fixture string) ([]mockGatewayResponse, error) {
	switch fixture {
	case "valid-final":
		return []mockGatewayResponse{{statusCode: 200, body: validFinalGatewayResponseBody()}}, nil
	case "valid-continue":
		return []mockGatewayResponse{{statusCode: 200, body: validContinueGatewayResponseBody()}}, nil
	case "schema-invalid":
		return []mockGatewayResponse{{statusCode: 200, body: invalidSchemaGatewayResponseBody()}}, nil
	case "decision-invalid":
		return []mockGatewayResponse{{statusCode: 200, body: decisionInvalidGatewayResponseBody()}}, nil
	case "continue-branch-invalid":
		return []mockGatewayResponse{{statusCode: 200, body: continueBranchInvalidGatewayResponseBody()}}, nil
	case "final-branch-invalid":
		return []mockGatewayResponse{{statusCode: 200, body: finalBranchInvalidGatewayResponseBody()}}, nil
	case "continue-missing-intent":
		return []mockGatewayResponse{{statusCode: 200, body: continueMissingIntentGatewayResponseBody()}}, nil
	case "final-missing-evidence":
		return []mockGatewayResponse{{statusCode: 200, body: finalMissingEvidenceGatewayResponseBody()}}, nil
	case "final-invalid-confidence":
		return []mockGatewayResponse{{statusCode: 200, body: finalInvalidConfidenceGatewayResponseBody()}}, nil
	case "unknown-field":
		return []mockGatewayResponse{{statusCode: 200, body: unknownFieldGatewayResponseBody()}}, nil
	case "reasoning-artifacts":
		return []mockGatewayResponse{{statusCode: 200, body: reasoningArtifactsGatewayResponseBody()}}, nil
	case "reasoning-unsupported":
		return []mockGatewayResponse{{statusCode: 400, body: map[string]any{"error": map[string]any{"code": "reasoning_not_supported", "message": "reasoning unsupported for selected model"}}}}, nil
	case "llm-answer-valid":
		return []mockGatewayResponse{{statusCode: 200, body: llmAnswerValidGatewayResponseBody()}}, nil
	case "llm-answer-empty":
		return []mockGatewayResponse{{statusCode: 200, body: llmAnswerEmptyGatewayResponseBody()}}, nil
	case "llm-answer-raw-text":
		return []mockGatewayResponse{{statusCode: 200, body: llmAnswerRawTextGatewayResponseBody()}}, nil
	case "invalid-json-text":
		return []mockGatewayResponse{{statusCode: 200, body: invalidJSONTextGatewayResponseBody()}}, nil
	default:
		return nil, fmt.Errorf("unknown mock fixture %q", fixture)
	}
}

func validFinalGatewayResponseBody() map[string]any {
	return map[string]any{
		"id":       "resp_valid_final",
		"status":   "completed",
		"provider": "openai",
		"model":    "gpt-5.1",
		"output": []any{
			map[string]any{"content": []any{map[string]any{"type": "output_text", "text": `{"decision":"final","final":{"answer":"done","evidence":[{"ref":"__context_ref__"}],"confidence":"medium"}}`}}},
		},
		"usage": map[string]any{"input_tokens": 10, "output_tokens": 5, "total_tokens": 15},
	}
}

func validContinueGatewayResponseBody() map[string]any {
	return map[string]any{
		"id":       "resp_valid_continue",
		"status":   "completed",
		"provider": "openai",
		"model":    "gpt-5.1",
		"output": []any{
			map[string]any{"content": []any{map[string]any{"type": "output_text", "text": `{"decision":"continue","continuation":{"repl_code":"next","intent":"inspect context chunk","expected_observation":"needle-like token appears"}}`}}},
		},
	}
}

func invalidSchemaGatewayResponseBody() map[string]any {
	return map[string]any{
		"id":       "resp_invalid_schema",
		"status":   "completed",
		"provider": "openai",
		"model":    "gpt-5.1",
		"output": []any{
			map[string]any{"content": []any{map[string]any{"type": "output_text", "text": `{"decision":"final"}`}}},
		},
	}
}

func decisionInvalidGatewayResponseBody() map[string]any {
	return map[string]any{
		"id":       "resp_invalid_decision",
		"status":   "completed",
		"provider": "openai",
		"model":    "gpt-5.1",
		"output": []any{
			map[string]any{"content": []any{map[string]any{"type": "output_text", "text": `{"decision":"maybe","final":{"answer":"done","evidence":[{"ref":"run-output://node/example/context.json"}]}}`}}},
		},
	}
}

func continueBranchInvalidGatewayResponseBody() map[string]any {
	return map[string]any{
		"id":       "resp_invalid_continue_branch",
		"status":   "completed",
		"provider": "openai",
		"model":    "gpt-5.1",
		"output": []any{
			map[string]any{"content": []any{map[string]any{"type": "output_text", "text": `{"decision":"continue","continuation":{"repl_code":"next","intent":"inspect","expected_observation":"match"},"final":{"answer":"done","evidence":[{"ref":"run-output://node/example/context.json"}]}}`}}},
		},
	}
}

func continueMissingIntentGatewayResponseBody() map[string]any {
	return map[string]any{
		"id":       "resp_continue_missing_intent",
		"status":   "completed",
		"provider": "openai",
		"model":    "gpt-5.1",
		"output": []any{
			map[string]any{"content": []any{map[string]any{"type": "output_text", "text": `{"decision":"continue","continuation":{"repl_code":"next","expected_observation":"match"}}`}}},
		},
	}
}

func finalBranchInvalidGatewayResponseBody() map[string]any {
	return map[string]any{
		"id":       "resp_invalid_final_branch",
		"status":   "completed",
		"provider": "openai",
		"model":    "gpt-5.1",
		"output": []any{
			map[string]any{"content": []any{map[string]any{"type": "output_text", "text": `{"decision":"final","final":{"answer":"done","evidence":[{"ref":"run-output://node/example/context.json"}]},"continuation":{"repl_code":"next","intent":"inspect","expected_observation":"match"}}`}}},
		},
	}
}

func finalMissingEvidenceGatewayResponseBody() map[string]any {
	return map[string]any{
		"id":       "resp_final_missing_evidence",
		"status":   "completed",
		"provider": "openai",
		"model":    "gpt-5.1",
		"output": []any{
			map[string]any{"content": []any{map[string]any{"type": "output_text", "text": `{"decision":"final","final":{"answer":"done"}}`}}},
		},
	}
}

func finalInvalidConfidenceGatewayResponseBody() map[string]any {
	return map[string]any{
		"id":       "resp_final_invalid_confidence",
		"status":   "completed",
		"provider": "openai",
		"model":    "gpt-5.1",
		"output": []any{
			map[string]any{"content": []any{map[string]any{"type": "output_text", "text": `{"decision":"final","final":{"answer":"done","evidence":[{"ref":"run-output://node/example/context.json"}],"confidence":"certain"}}`}}},
		},
	}
}

func unknownFieldGatewayResponseBody() map[string]any {
	return map[string]any{
		"id":       "resp_unknown_field",
		"status":   "completed",
		"provider": "openai",
		"model":    "gpt-5.1",
		"output": []any{
			map[string]any{"content": []any{map[string]any{"type": "output_text", "text": `{"decision":"final","final":{"answer":"done","evidence":[{"ref":"run-output://node/example/context.json"}]},"unexpected":"value"}`}}},
		},
	}
}

func reasoningArtifactsGatewayResponseBody() map[string]any {
	return map[string]any{
		"id":       "resp_reasoning_artifacts",
		"status":   "completed",
		"provider": "openai",
		"model":    "gpt-5.1",
		"output": []any{
			map[string]any{"content": []any{map[string]any{"type": "output_text", "text": `{"decision":"continue","continuation":{"repl_code":"next","intent":"inspect","expected_observation":"match"}}`}}},
		},
		"reasoning": map[string]any{"summary": "trace", "encrypted_content": "blob"},
		"usage": map[string]any{
			"input_tokens":  20,
			"output_tokens": 10,
			"total_tokens":  30,
			"output_tokens_details": map[string]any{
				"reasoning_tokens": 7,
			},
		},
	}
}

func llmAnswerValidGatewayResponseBody() map[string]any {
	return map[string]any{
		"id":       "resp_llm_answer_valid",
		"status":   "completed",
		"provider": "openai",
		"model":    "gpt-5.1",
		"output": []any{
			map[string]any{"content": []any{map[string]any{"type": "output_text", "text": `{"answer":"plain-answer"}`}}},
		},
		"usage": map[string]any{"input_tokens": 10, "output_tokens": 5, "total_tokens": 15},
	}
}

func llmAnswerEmptyGatewayResponseBody() map[string]any {
	return map[string]any{
		"id":       "resp_llm_answer_empty",
		"status":   "completed",
		"provider": "openai",
		"model":    "gpt-5.1",
		"output": []any{
			map[string]any{"content": []any{map[string]any{"type": "output_text", "text": `{"answer":""}`}}},
		},
		"usage": map[string]any{"input_tokens": 10, "output_tokens": 5, "total_tokens": 15},
	}
}

func llmAnswerRawTextGatewayResponseBody() map[string]any {
	return map[string]any{
		"id":          "resp_llm_answer_raw_text",
		"status":      "completed",
		"provider":    "openai",
		"model":       "gpt-5.1",
		"output_text": "plain-answer-from-raw-text",
		"usage":       map[string]any{"input_tokens": 4, "output_tokens": 3, "total_tokens": 7},
	}
}

func invalidJSONTextGatewayResponseBody() map[string]any {
	return map[string]any{
		"id":          "resp_invalid_json_text",
		"status":      "completed",
		"provider":    "openai",
		"model":       "gpt-5.1",
		"output_text": "non-json text payload",
		"usage":       map[string]any{"input_tokens": 4, "output_tokens": 3, "total_tokens": 7},
	}
}

func asMapValue(value any) map[string]any {
	if value == nil {
		return nil
	}
	mapped, _ := value.(map[string]any)
	return mapped
}

func osSetEnv(key string, value string) error {
	if strings.TrimSpace(key) == "" {
		return errors.New("environment key is required")
	}
	return os.Setenv(key, value)
}
