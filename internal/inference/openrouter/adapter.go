package openrouter

import (
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"io"
	"log/slog"
	"net/http"
	"net/url"
	"os"
	"strconv"
	"strings"
	"time"

	"github.com/leefowlercu/sigil/internal/inference"
	"github.com/leefowlercu/sigil/internal/inference/schema"
)

const (
	defaultRequestTimeoutMS = 30000
)

// Adapter executes non-streaming OpenRouter Responses API requests.
type Adapter struct {
	baseURL   string
	apiKeyEnv string
	client    *http.Client
}

// NewGateway builds an OpenRouter gateway adapter from a generic inference request.
func NewGateway(request inference.Request) (inference.Gateway, error) {
	baseURL := strings.TrimSpace(request.GatewayConfig.BaseURL)
	if baseURL == "" {
		return nil, fmt.Errorf("llm.openrouter.base_url is required")
	}

	if _, err := url.ParseRequestURI(baseURL); err != nil {
		return nil, fmt.Errorf("invalid llm.openrouter.base_url %q; %w", baseURL, err)
	}

	timeoutMS := request.GatewayConfig.RequestTimeoutMS
	if timeoutMS <= 0 {
		timeoutMS = defaultRequestTimeoutMS
	}

	apiKeyEnv := strings.TrimSpace(request.GatewayConfig.APIKeyEnv)
	if apiKeyEnv == "" {
		return nil, fmt.Errorf("llm.openrouter.api_key_env is required")
	}

	return &Adapter{
		baseURL:   strings.TrimRight(baseURL, "/"),
		apiKeyEnv: apiKeyEnv,
		client: &http.Client{
			Timeout: time.Duration(timeoutMS) * time.Millisecond,
		},
	}, nil
}

// Infer executes one non-streaming OpenRouter Responses API call.
func (a *Adapter) Infer(ctx context.Context, request inference.GatewayRequest) (inference.GatewayResponse, error) {
	logger := openrouterLogger().With(
		"provider", request.Provider,
		"model", request.Model,
		"base_url", a.baseURL,
	)
	apiKey := strings.TrimSpace(os.Getenv(a.apiKeyEnv))
	if apiKey == "" {
		logger.Error("openrouter api key is not configured", "api_key_env", a.apiKeyEnv)
		return inference.GatewayResponse{}, fmt.Errorf("openrouter api key environment variable %q is not set", a.apiKeyEnv)
	}

	requestBody, err := a.buildRequestBody(request)
	if err != nil {
		logger.Error("failed to build openrouter request body", "error", err)
		return inference.GatewayResponse{}, err
	}

	encodedBody, err := json.Marshal(requestBody)
	if err != nil {
		logger.Error("failed to encode openrouter request", "error", err)
		return inference.GatewayResponse{}, fmt.Errorf("failed to encode openrouter request payload; %w", err)
	}
	logger.Debug("sending openrouter request",
		"request_bytes", len(encodedBody),
		"reasoning_enabled", request.Reasoning.Enabled,
	)

	httpRequest, err := http.NewRequestWithContext(ctx, http.MethodPost, a.baseURL+"/responses", bytes.NewReader(encodedBody))
	if err != nil {
		logger.Error("failed to construct openrouter http request", "error", err)
		return inference.GatewayResponse{}, fmt.Errorf("failed to create openrouter request; %w", err)
	}

	httpRequest.Header.Set("Authorization", "Bearer "+apiKey)
	httpRequest.Header.Set("Content-Type", "application/json")
	httpRequest.Header.Set("Accept", "application/json")

	httpResponse, err := a.client.Do(httpRequest)
	if err != nil {
		logger.Error("openrouter request transport failure", "error", err)
		return inference.GatewayResponse{}, fmt.Errorf("openrouter request failed; %w", err)
	}
	defer httpResponse.Body.Close()

	bodyBytes, err := io.ReadAll(httpResponse.Body)
	if err != nil {
		logger.Error("failed to read openrouter response body",
			"status_code", httpResponse.StatusCode,
			"error", err,
		)
		return inference.GatewayResponse{}, fmt.Errorf("failed to read openrouter response body; %w", err)
	}

	if httpResponse.StatusCode < 200 || httpResponse.StatusCode >= 300 {
		logger.Warn("openrouter request returned non-success status",
			"status_code", httpResponse.StatusCode,
			"response_bytes", len(bodyBytes),
		)
		httpErr := &inference.GatewayHTTPError{StatusCode: httpResponse.StatusCode, Body: strings.TrimSpace(string(bodyBytes))}
		if request.Reasoning.Enabled && isReasoningCapabilityFailure(httpResponse.StatusCode, bodyBytes) {
			logger.Warn("openrouter reasoning capability mismatch",
				"status_code", httpResponse.StatusCode,
			)
			return inference.GatewayResponse{}, &inference.ReasoningCapabilityError{Message: "configured provider/model does not support required reasoning behavior", Cause: httpErr}
		}

		return inference.GatewayResponse{}, httpErr
	}

	var decoded map[string]any
	if err := json.Unmarshal(bodyBytes, &decoded); err != nil {
		logger.Error("failed to decode openrouter response json", "error", err)
		return inference.GatewayResponse{}, fmt.Errorf("failed to decode openrouter response JSON; %w", err)
	}

	structuredPayload, extractionMetadata, err := extractStructuredPayload(decoded, request.Schema.ID)
	if err != nil {
		logger.Error("failed to extract structured response payload", "error", err)
		return inference.GatewayResponse{}, fmt.Errorf("failed to extract structured response payload; %w", err)
	}

	usage := extractUsage(decoded)
	reasoning := extractReasoning(decoded)

	responseID, _ := decoded["id"].(string)
	responseProvider, _ := decoded["provider"].(string)
	responseModel, _ := decoded["model"].(string)
	status, _ := decoded["status"].(string)
	logger.Info("openrouter request completed",
		"status_code", httpResponse.StatusCode,
		"gateway_response_id", strings.TrimSpace(responseID),
		"response_provider", strings.TrimSpace(responseProvider),
		"response_model", strings.TrimSpace(responseModel),
		"finish_status", strings.TrimSpace(status),
	)

	rawMetadata := cloneMap(decoded)
	if len(extractionMetadata) > 0 {
		rawMetadata["extraction"] = extractionMetadata
	}

	return inference.GatewayResponse{
		GatewayResponseID: strings.TrimSpace(responseID),
		Provider:          strings.TrimSpace(responseProvider),
		Model:             strings.TrimSpace(responseModel),
		FinishStatus:      strings.TrimSpace(status),
		StructuredPayload: structuredPayload,
		Usage:             usage,
		Reasoning:         reasoning,
		RawMetadata:       rawMetadata,
	}, nil
}

func (a *Adapter) buildRequestBody(request inference.GatewayRequest) (map[string]any, error) {
	if len(request.Messages) == 0 {
		return nil, fmt.Errorf("request messages are required")
	}
	provider := strings.TrimSpace(request.Provider)
	if provider == "" {
		return nil, fmt.Errorf("request provider is required")
	}
	model := strings.TrimSpace(request.Model)
	if model == "" {
		return nil, fmt.Errorf("request model is required")
	}

	input := make([]any, 0, len(request.Messages))
	for index, message := range request.Messages {
		role := strings.TrimSpace(string(message.Role))
		content := strings.TrimSpace(message.Content)
		if role == "" {
			return nil, fmt.Errorf("request messages[%d].role is required", index)
		}
		if content == "" {
			return nil, fmt.Errorf("request messages[%d].content is required", index)
		}
		input = append(input, map[string]any{
			"role":    role,
			"content": content,
		})
	}

	requestBody := map[string]any{
		"model":  provider + "/" + model,
		"input":  input,
		"stream": false,
		"response_format": map[string]any{
			"type": "json_schema",
			"json_schema": map[string]any{
				"name":   request.Schema.Name,
				"strict": true,
				"schema": request.Schema.JSONSchema,
			},
		},
		"plugins": []any{
			map[string]any{"id": "response-healing"},
		},
	}

	if request.Reasoning.Enabled {
		requestBody["reasoning"] = map[string]any{
			"effort": request.Reasoning.Effort,
		}
	}

	return requestBody, nil
}

func isReasoningCapabilityFailure(statusCode int, body []byte) bool {
	if statusCode < 400 || statusCode >= 500 {
		return false
	}

	text := strings.ToLower(strings.TrimSpace(string(body)))
	if text == "" {
		return false
	}

	if strings.Contains(text, "reasoning") && (strings.Contains(text, "unsupported") || strings.Contains(text, "not supported")) {
		return true
	}

	var decoded map[string]any
	if err := json.Unmarshal(body, &decoded); err != nil {
		return false
	}

	errorMap := asMap(decoded["error"])
	code, _ := errorMap["code"].(string)
	code = strings.ToLower(code)
	if strings.Contains(code, "reasoning") && (strings.Contains(code, "unsupported") || strings.Contains(code, "not_supported")) {
		return true
	}

	return false
}

func extractStructuredPayload(decoded map[string]any, schemaID string) (map[string]any, map[string]any, error) {
	if payload := asMap(decoded["validated_payload"]); payload != nil {
		return payload, nil, nil
	}

	trimmedSchemaID := strings.TrimSpace(schemaID)
	rawFallbackCandidate := ""
	rawFallbackSource := ""

	if text, ok := decoded["output_text"].(string); ok {
		payload, err := decodePayloadText(text)
		if err == nil {
			return payload, nil, nil
		}
		if trimmedSchemaID == schema.SigilLLMAnswerV1SchemaID && strings.TrimSpace(text) != "" {
			rawFallbackCandidate = strings.TrimSpace(text)
			rawFallbackSource = "output_text"
		} else {
			return nil, nil, err
		}
	}

	outputArray, ok := decoded["output"].([]any)
	if !ok {
		if rawFallbackCandidate != "" {
			return map[string]any{"answer": rawFallbackCandidate}, map[string]any{
				"mode":      "raw_text_fallback",
				"source":    rawFallbackSource,
				"schema_id": trimmedSchemaID,
			}, nil
		}
		return nil, nil, fmt.Errorf("missing output payload")
	}

	for outputIndex, outputItem := range outputArray {
		outputMap := asMap(outputItem)
		contentArray, ok := outputMap["content"].([]any)
		if !ok {
			continue
		}

		for contentIndex, contentItem := range contentArray {
			contentMap := asMap(contentItem)
			contentType, _ := contentMap["type"].(string)
			if strings.TrimSpace(contentType) != "output_text" {
				continue
			}

			text, _ := contentMap["text"].(string)
			if strings.TrimSpace(text) == "" {
				if rawFallbackCandidate == "" {
					rawFallbackSource = fmt.Sprintf("output[%d].content[%d].text", outputIndex, contentIndex)
				}
				continue
			}
			payload, err := decodePayloadText(text)
			if err == nil {
				return payload, nil, nil
			}
			if trimmedSchemaID == schema.SigilLLMAnswerV1SchemaID && rawFallbackCandidate == "" {
				rawFallbackCandidate = strings.TrimSpace(text)
				rawFallbackSource = fmt.Sprintf("output[%d].content[%d].text", outputIndex, contentIndex)
				continue
			}
			if trimmedSchemaID != schema.SigilLLMAnswerV1SchemaID {
				return nil, nil, err
			}
		}
	}

	if trimmedSchemaID == schema.SigilLLMAnswerV1SchemaID && rawFallbackCandidate != "" {
		return map[string]any{"answer": rawFallbackCandidate}, map[string]any{
			"mode":      "raw_text_fallback",
			"source":    rawFallbackSource,
			"schema_id": trimmedSchemaID,
		}, nil
	}

	return nil, nil, fmt.Errorf("output_text content not found")
}

func decodePayloadText(text string) (map[string]any, error) {
	trimmed := strings.TrimSpace(text)
	if trimmed == "" {
		return nil, fmt.Errorf("structured payload text is empty")
	}

	var payload map[string]any
	decoder := json.NewDecoder(strings.NewReader(trimmed))
	decoder.UseNumber()
	if err := decoder.Decode(&payload); err != nil {
		return nil, fmt.Errorf("failed to parse structured payload JSON; %w", err)
	}
	if payload == nil {
		return nil, fmt.Errorf("structured payload must be a JSON object")
	}

	return payload, nil
}

func extractUsage(decoded map[string]any) inference.Usage {
	usageMap := asMap(decoded["usage"])
	if usageMap == nil {
		return inference.Usage{}
	}

	usage := inference.Usage{
		InputTokens:  int64FromAny(usageMap["input_tokens"]),
		OutputTokens: int64FromAny(usageMap["output_tokens"]),
		TotalTokens:  int64FromAny(usageMap["total_tokens"]),
	}

	reasoningTokens := int64FromAny(usageMap["reasoning_tokens"])
	if reasoningTokens == 0 {
		outputDetails := asMap(usageMap["output_tokens_details"])
		reasoningTokens = int64FromAny(outputDetails["reasoning_tokens"])
	}
	if reasoningTokens > 0 {
		usage.ReasoningTokens = &reasoningTokens
	}

	return usage
}

func extractReasoning(decoded map[string]any) inference.Reasoning {
	reasoningMap := asMap(decoded["reasoning"])
	if reasoningMap == nil {
		return inference.Reasoning{}
	}

	reasoning := inference.Reasoning{Artifacts: cloneMap(reasoningMap)}
	if effort, ok := reasoningMap["effort"].(string); ok {
		effort = strings.TrimSpace(effort)
		reasoning.Effort = &effort
	}

	return reasoning
}

func asMap(value any) map[string]any {
	if value == nil {
		return nil
	}

	mapValue, ok := value.(map[string]any)
	if !ok {
		return nil
	}

	return mapValue
}

func openrouterLogger() *slog.Logger {
	return slog.Default().With("component", "inference.openrouter")
}

func int64FromAny(value any) int64 {
	switch typed := value.(type) {
	case nil:
		return 0
	case int:
		return int64(typed)
	case int32:
		return int64(typed)
	case int64:
		return typed
	case float64:
		return int64(typed)
	case json.Number:
		parsed, err := typed.Int64()
		if err == nil {
			return parsed
		}
	case string:
		parsed, err := strconv.ParseInt(strings.TrimSpace(typed), 10, 64)
		if err == nil {
			return parsed
		}
	}

	return 0
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
