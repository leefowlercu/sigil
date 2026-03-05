package openrouter

import (
	"context"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"

	"github.com/leefowlercu/sigil/internal/inference"
	"github.com/leefowlercu/sigil/internal/inference/schema"
)

func TestInferBuildsStrictOpenRouterRequestWithHealingAndReasoning(t *testing.T) {
	var capturedPath string
	var capturedAuth string
	var capturedRequest map[string]any

	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		capturedPath = r.URL.Path
		capturedAuth = r.Header.Get("Authorization")
		if err := json.NewDecoder(r.Body).Decode(&capturedRequest); err != nil {
			t.Fatalf("failed to decode request body: %v", err)
		}

		w.Header().Set("Content-Type", "application/json")
		_, _ = w.Write([]byte(`{"id":"resp_123","status":"completed","provider":"openai","model":"gpt-5.1","output":[{"content":[{"type":"output_text","text":"{\"decision\":\"final\",\"final\":{\"answer\":\"done\"}}"}]}],"usage":{"input_tokens":10,"output_tokens":5,"total_tokens":15}}`))
	}))
	defer server.Close()

	t.Setenv("OPENROUTER_API_KEY", "test-api-key")
	adapter, err := NewGateway(inference.Request{GatewayConfig: inference.GatewayConfig{BaseURL: server.URL, RequestTimeoutMS: 30000, APIKeyEnv: "OPENROUTER_API_KEY"}})
	if err != nil {
		t.Fatalf("expected adapter construction success, got %v", err)
	}

	schemaDefinition, err := schema.NewRegistry().Resolve(schema.SigilRLMResponseV1SchemaID)
	if err != nil {
		t.Fatalf("failed to resolve schema definition: %v", err)
	}

	response, err := adapter.Infer(context.Background(), inference.GatewayRequest{
		Messages: []inference.Message{
			{Role: inference.MessageRoleSystem, Content: "system prompt"},
			{Role: inference.MessageRoleUser, Content: "{\"query\":\"prompt\"}"},
		},
		Provider: "openai",
		Model:    "gpt-5.1",
		Schema:   schemaDefinition,
		Reasoning: inference.ReasoningConfig{
			Enabled: true,
			Effort:  "medium",
		},
	})
	if err != nil {
		t.Fatalf("expected inference success, got %v", err)
	}

	if capturedPath != "/responses" {
		t.Fatalf("expected request path /responses, got %q", capturedPath)
	}
	if capturedAuth != "Bearer test-api-key" {
		t.Fatalf("expected bearer auth header, got %q", capturedAuth)
	}
	if model, _ := capturedRequest["model"].(string); model != "openai/gpt-5.1" {
		t.Fatalf("expected openrouter model openai/gpt-5.1, got %q", model)
	}
	if streamed, _ := capturedRequest["stream"].(bool); streamed {
		t.Fatal("expected stream=false request")
	}
	input, ok := capturedRequest["input"].([]any)
	if !ok || len(input) != 2 {
		t.Fatalf("expected input message array length 2, got %T len=%d", capturedRequest["input"], len(input))
	}
	first := asMap(input[0])
	second := asMap(input[1])
	if role, _ := first["role"].(string); role != "system" {
		t.Fatalf("expected first message role system, got %q", role)
	}
	if role, _ := second["role"].(string); role != "user" {
		t.Fatalf("expected second message role user, got %q", role)
	}

	responseFormat := asMap(capturedRequest["response_format"])
	if responseFormat == nil {
		t.Fatal("expected response_format block")
	}
	if responseType, _ := responseFormat["type"].(string); responseType != "json_schema" {
		t.Fatalf("expected response_format.type=json_schema, got %q", responseType)
	}

	jsonSchema := asMap(responseFormat["json_schema"])
	if jsonSchema == nil {
		t.Fatal("expected response_format.json_schema block")
	}
	if strict, _ := jsonSchema["strict"].(bool); !strict {
		t.Fatal("expected strict json_schema mode")
	}
	if _, ok := jsonSchema["schema"].(map[string]any); !ok {
		t.Fatal("expected json_schema.schema object")
	}

	plugins, ok := capturedRequest["plugins"].([]any)
	if !ok || len(plugins) == 0 {
		t.Fatal("expected response healing plugins block")
	}
	pluginMap := asMap(plugins[0])
	if pluginID, _ := pluginMap["id"].(string); pluginID != "response-healing" {
		t.Fatalf("expected response-healing plugin id, got %q", pluginID)
	}

	reasoningMap := asMap(capturedRequest["reasoning"])
	if reasoningMap == nil {
		t.Fatal("expected reasoning block when enabled")
	}
	if effort, _ := reasoningMap["effort"].(string); effort != "medium" {
		t.Fatalf("expected reasoning.effort medium, got %q", effort)
	}

	if response.StructuredPayload["decision"] != "final" {
		t.Fatalf("expected parsed structured payload, got %+v", response.StructuredPayload)
	}
}

func TestInferOmitsReasoningBlockWhenDisabled(t *testing.T) {
	var capturedRequest map[string]any

	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if err := json.NewDecoder(r.Body).Decode(&capturedRequest); err != nil {
			t.Fatalf("failed to decode request body: %v", err)
		}
		w.Header().Set("Content-Type", "application/json")
		_, _ = w.Write([]byte(`{"id":"resp_omit_reasoning","status":"completed","output":[{"content":[{"type":"output_text","text":"{\"decision\":\"final\",\"final\":{\"answer\":\"done\"}}"}]}]}`))
	}))
	defer server.Close()

	t.Setenv("OPENROUTER_API_KEY", "test-api-key")
	adapter, err := NewGateway(inference.Request{GatewayConfig: inference.GatewayConfig{BaseURL: server.URL, RequestTimeoutMS: 30000, APIKeyEnv: "OPENROUTER_API_KEY"}})
	if err != nil {
		t.Fatalf("expected adapter construction success, got %v", err)
	}

	schemaDefinition, err := schema.NewRegistry().Resolve(schema.SigilRLMResponseV1SchemaID)
	if err != nil {
		t.Fatalf("failed to resolve schema definition: %v", err)
	}

	_, err = adapter.Infer(context.Background(), inference.GatewayRequest{
		Messages:  []inference.Message{{Role: inference.MessageRoleSystem, Content: "system"}, {Role: inference.MessageRoleUser, Content: "{\"query\":\"prompt\"}"}},
		Provider:  "openai",
		Model:     "gpt-5.1",
		Schema:    schemaDefinition,
		Reasoning: inference.ReasoningConfig{Enabled: false, Effort: "high"},
	})
	if err != nil {
		t.Fatalf("expected inference success, got %v", err)
	}

	if _, exists := capturedRequest["reasoning"]; exists {
		t.Fatalf("expected reasoning block to be omitted when disabled, got %+v", capturedRequest["reasoning"])
	}
}

func TestInferMapsReasoningArtifactsAndReasoningTokens(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		_, _ = w.Write([]byte(`{"id":"resp_reasoning","status":"completed","output":[{"content":[{"type":"output_text","text":"{\"decision\":\"continue\",\"continuation\":{\"repl_code\":\"next\"}}"}]}],"reasoning":{"summary":"chain","encrypted_content":"blob"},"usage":{"input_tokens":20,"output_tokens":10,"total_tokens":30,"output_tokens_details":{"reasoning_tokens":7}}}`))
	}))
	defer server.Close()

	t.Setenv("OPENROUTER_API_KEY", "test-api-key")
	adapter, err := NewGateway(inference.Request{GatewayConfig: inference.GatewayConfig{BaseURL: server.URL, RequestTimeoutMS: 30000, APIKeyEnv: "OPENROUTER_API_KEY"}})
	if err != nil {
		t.Fatalf("expected adapter construction success, got %v", err)
	}

	schemaDefinition, err := schema.NewRegistry().Resolve(schema.SigilRLMResponseV1SchemaID)
	if err != nil {
		t.Fatalf("failed to resolve schema definition: %v", err)
	}

	response, err := adapter.Infer(context.Background(), inference.GatewayRequest{
		Messages: []inference.Message{
			{Role: inference.MessageRoleSystem, Content: "system"},
			{Role: inference.MessageRoleUser, Content: "{\"query\":\"prompt\"}"},
		},
		Provider: "openai",
		Model:    "gpt-5.1",
		Schema:   schemaDefinition,
		Reasoning: inference.ReasoningConfig{
			Enabled: true,
			Effort:  "medium",
		},
	})
	if err != nil {
		t.Fatalf("expected inference success, got %v", err)
	}

	if response.Usage.ReasoningTokens == nil || *response.Usage.ReasoningTokens != 7 {
		t.Fatalf("expected reasoning tokens=7, got %+v", response.Usage)
	}
	if response.Reasoning.Artifacts["summary"] != "chain" {
		t.Fatalf("expected reasoning summary artifact, got %+v", response.Reasoning.Artifacts)
	}
}

func TestInferReturnsReasoningCapabilityErrorForUnsupportedReasoning(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusBadRequest)
		_, _ = w.Write([]byte(`{"error":{"code":"reasoning_not_supported","message":"reasoning unsupported for selected model"}}`))
	}))
	defer server.Close()

	t.Setenv("OPENROUTER_API_KEY", "test-api-key")
	adapter, err := NewGateway(inference.Request{GatewayConfig: inference.GatewayConfig{BaseURL: server.URL, RequestTimeoutMS: 30000, APIKeyEnv: "OPENROUTER_API_KEY"}})
	if err != nil {
		t.Fatalf("expected adapter construction success, got %v", err)
	}

	schemaDefinition, err := schema.NewRegistry().Resolve(schema.SigilRLMResponseV1SchemaID)
	if err != nil {
		t.Fatalf("failed to resolve schema definition: %v", err)
	}

	_, runErr := adapter.Infer(context.Background(), inference.GatewayRequest{
		Messages: []inference.Message{
			{Role: inference.MessageRoleSystem, Content: "system"},
			{Role: inference.MessageRoleUser, Content: "{\"query\":\"prompt\"}"},
		},
		Provider: "openai",
		Model:    "gpt-5.1",
		Schema:   schemaDefinition,
		Reasoning: inference.ReasoningConfig{
			Enabled: true,
			Effort:  "medium",
		},
	})
	var reasoningErr *inference.ReasoningCapabilityError
	if runErr == nil || !strings.Contains(runErr.Error(), "reasoning") {
		t.Fatalf("expected reasoning capability error, got %v", runErr)
	}
	if !asReasoningCapabilityError(runErr, &reasoningErr) {
		t.Fatalf("expected *ReasoningCapabilityError, got %T", runErr)
	}
}

func TestInferFailsWhenAPIKeyEnvIsMissingAtCallTime(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusOK)
		_, _ = w.Write([]byte(`{"id":"resp_unused","status":"completed"}`))
	}))
	defer server.Close()

	adapter, err := NewGateway(inference.Request{GatewayConfig: inference.GatewayConfig{BaseURL: server.URL, RequestTimeoutMS: 30000, APIKeyEnv: "MISSING_OPENROUTER_API_KEY"}})
	if err != nil {
		t.Fatalf("expected adapter construction success, got %v", err)
	}

	schemaDefinition, err := schema.NewRegistry().Resolve(schema.SigilRLMResponseV1SchemaID)
	if err != nil {
		t.Fatalf("failed to resolve schema definition: %v", err)
	}

	_, runErr := adapter.Infer(context.Background(), inference.GatewayRequest{
		Messages: []inference.Message{
			{Role: inference.MessageRoleSystem, Content: "system"},
			{Role: inference.MessageRoleUser, Content: "{\"query\":\"prompt\"}"},
		},
		Provider: "openai",
		Model:    "gpt-5.1",
		Schema:   schemaDefinition,
	})
	if runErr == nil || !strings.Contains(runErr.Error(), "MISSING_OPENROUTER_API_KEY") {
		t.Fatalf("expected missing api key env error, got %v", runErr)
	}
}

func TestBuildRequestBodyRejectsMissingProviderOrModel(t *testing.T) {
	schemaDefinition, err := schema.NewRegistry().Resolve(schema.SigilRLMResponseV1SchemaID)
	if err != nil {
		t.Fatalf("failed to resolve schema definition: %v", err)
	}

	testCases := []struct {
		name    string
		request inference.GatewayRequest
		wantErr string
	}{
		{
			name: "missing messages",
			request: inference.GatewayRequest{
				Provider: "openai",
				Model:    "gpt-5.1",
				Schema:   schemaDefinition,
			},
			wantErr: "request messages are required",
		},
		{
			name: "missing provider",
			request: inference.GatewayRequest{
				Messages: []inference.Message{{Role: inference.MessageRoleSystem, Content: "system"}, {Role: inference.MessageRoleUser, Content: "{\"query\":\"prompt\"}"}},
				Provider: "   ",
				Model:    "gpt-5.1",
				Schema:   schemaDefinition,
			},
			wantErr: "request provider is required",
		},
		{
			name: "missing model",
			request: inference.GatewayRequest{
				Messages: []inference.Message{{Role: inference.MessageRoleSystem, Content: "system"}, {Role: inference.MessageRoleUser, Content: "{\"query\":\"prompt\"}"}},
				Provider: "openai",
				Model:    "   ",
				Schema:   schemaDefinition,
			},
			wantErr: "request model is required",
		},
	}

	adapter := &Adapter{}
	for _, testCase := range testCases {
		t.Run(testCase.name, func(t *testing.T) {
			_, runErr := adapter.buildRequestBody(testCase.request)
			if runErr == nil || !strings.Contains(runErr.Error(), testCase.wantErr) {
				t.Fatalf("expected error containing %q, got %v", testCase.wantErr, runErr)
			}
		})
	}
}

func asReasoningCapabilityError(err error, target **inference.ReasoningCapabilityError) bool {
	if err == nil {
		return false
	}

	typed, ok := err.(*inference.ReasoningCapabilityError)
	if !ok {
		return false
	}
	*target = typed
	return true
}

func TestDecodePayloadTextAcceptsTrailingContentAfterFirstJSONObject(t *testing.T) {
	payload, err := decodePayloadText(`{"decision":"final","final":{"answer":"done"}}{"unexpected":"trailing"}`)
	if err != nil {
		t.Fatalf("expected payload decode success, got %v", err)
	}
	if payload["decision"] != "final" {
		t.Fatalf("expected decision final, got %+v", payload)
	}
}

func TestDecodePayloadTextRejectsInvalidJSONPrefix(t *testing.T) {
	_, err := decodePayloadText(`not-json {"decision":"final"}`)
	if err == nil {
		t.Fatal("expected decode failure for invalid json prefix")
	}
	if !strings.Contains(err.Error(), "failed to parse structured payload JSON") {
		t.Fatalf("expected structured payload parse failure, got %v", err)
	}
}

func TestExtractStructuredPayloadUsesRawTextFallbackForLLMAnswerSchema(t *testing.T) {
	decoded := map[string]any{
		"output_text": "  plain fallback answer  ",
	}

	payload, metadata, err := extractStructuredPayload(decoded, schema.SigilLLMAnswerV1SchemaID)
	if err != nil {
		t.Fatalf("expected extraction success, got %v", err)
	}
	if payload["answer"] != "plain fallback answer" {
		t.Fatalf("expected normalized answer payload, got %+v", payload)
	}
	if metadata["mode"] != "raw_text_fallback" {
		t.Fatalf("expected raw_text_fallback mode, got %+v", metadata)
	}
	if metadata["source"] != "output_text" {
		t.Fatalf("expected fallback source output_text, got %+v", metadata)
	}
}

func TestExtractStructuredPayloadRejectsRawTextFallbackForNonLLMAnswerSchemas(t *testing.T) {
	decoded := map[string]any{
		"output_text": "plain fallback answer",
	}

	_, _, err := extractStructuredPayload(decoded, schema.SigilRLMResponseV1SchemaID)
	if err == nil {
		t.Fatal("expected extraction failure for non-llm schema fallback attempt")
	}
}

func TestInferEmitsExtractionMetadataWhenRawTextFallbackApplies(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		_, _ = w.Write([]byte(`{"id":"resp_fallback","status":"completed","provider":"openai","model":"gpt-5.1","output_text":"plain subcall answer","usage":{"input_tokens":2,"output_tokens":3,"total_tokens":5}}`))
	}))
	defer server.Close()

	t.Setenv("OPENROUTER_API_KEY", "test-api-key")
	adapter, err := NewGateway(inference.Request{GatewayConfig: inference.GatewayConfig{BaseURL: server.URL, RequestTimeoutMS: 30000, APIKeyEnv: "OPENROUTER_API_KEY"}})
	if err != nil {
		t.Fatalf("expected adapter construction success, got %v", err)
	}

	schemaDefinition, err := schema.NewRegistry().Resolve(schema.SigilLLMAnswerV1SchemaID)
	if err != nil {
		t.Fatalf("failed to resolve schema definition: %v", err)
	}

	response, err := adapter.Infer(context.Background(), inference.GatewayRequest{
		Messages: []inference.Message{
			{Role: inference.MessageRoleSystem, Content: "system"},
			{Role: inference.MessageRoleUser, Content: `{"prompt":"p","context":"c"}`},
		},
		Provider: "openai",
		Model:    "gpt-5.1",
		Schema:   schemaDefinition,
	})
	if err != nil {
		t.Fatalf("expected inference success, got %v", err)
	}

	if response.StructuredPayload["answer"] != "plain subcall answer" {
		t.Fatalf("expected fallback answer payload, got %+v", response.StructuredPayload)
	}
	extraction := asMap(response.RawMetadata["extraction"])
	if extraction == nil {
		t.Fatalf("expected extraction metadata in raw metadata, got %+v", response.RawMetadata)
	}
	if extraction["mode"] != "raw_text_fallback" {
		t.Fatalf("expected raw_text_fallback mode, got %+v", extraction)
	}
}
