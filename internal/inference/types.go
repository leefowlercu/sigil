package inference

import (
	"context"

	"github.com/leefowlercu/sigil/internal/inference/schema"
)

// MessageRole identifies ordered model-input message roles.
type MessageRole string

const (
	MessageRoleSystem    MessageRole = "system"
	MessageRoleUser      MessageRole = "user"
	MessageRoleAssistant MessageRole = "assistant"
)

// Message is one ordered model-input message.
type Message struct {
	Role    MessageRole `json:"role"`
	Content string      `json:"content"`
}

// Request is the gateway-agnostic inference input contract.
type Request struct {
	Messages      []Message       `json:"messages"`
	SchemaID      string          `json:"schema_id"`
	Gateway       string          `json:"gateway"`
	Provider      string          `json:"provider"`
	Model         string          `json:"model"`
	GatewayConfig GatewayConfig   `json:"gateway_config"`
	Reasoning     ReasoningConfig `json:"reasoning"`
}

// GatewayConfig carries gateway transport configuration values.
type GatewayConfig struct {
	BaseURL          string `json:"base_url"`
	RequestTimeoutMS int    `json:"request_timeout_ms"`
	APIKeyEnv        string `json:"api_key_env"`
}

// ReasoningConfig controls reasoning behavior for inference requests.
type ReasoningConfig struct {
	Enabled bool   `json:"enabled"`
	Effort  string `json:"effort"`
}

// Usage captures normalized token accounting for an inference response.
type Usage struct {
	InputTokens     int64  `json:"input_tokens"`
	OutputTokens    int64  `json:"output_tokens"`
	TotalTokens     int64  `json:"total_tokens"`
	ReasoningTokens *int64 `json:"reasoning_tokens,omitempty"`
}

// Reasoning captures normalized reasoning output fields.
type Reasoning struct {
	Enabled   bool           `json:"enabled"`
	Effort    *string        `json:"effort"`
	Artifacts map[string]any `json:"artifacts,omitempty"`
}

// Result is the canonical normalized successful inference output contract.
type Result struct {
	SchemaID          string         `json:"schema_id"`
	ValidatedPayload  map[string]any `json:"validated_payload"`
	Gateway           string         `json:"gateway"`
	Provider          string         `json:"provider"`
	Model             string         `json:"model"`
	GatewayResponseID string         `json:"gateway_response_id"`
	Usage             Usage          `json:"usage"`
	Reasoning         Reasoning      `json:"reasoning"`
	FinishStatus      string         `json:"finish_status"`
	RawMetadata       map[string]any `json:"raw_metadata"`
}

// GatewayRequest is the normalized request sent from inference core to gateway adapters.
type GatewayRequest struct {
	Messages  []Message
	Provider  string
	Model     string
	Schema    schema.Definition
	Reasoning ReasoningConfig
}

// GatewayResponse is the normalized adapter response consumed by inference core.
type GatewayResponse struct {
	GatewayResponseID string
	Provider          string
	Model             string
	FinishStatus      string
	StructuredPayload map[string]any
	Usage             Usage
	Reasoning         Reasoning
	RawMetadata       map[string]any
}

// Gateway defines a gateway adapter contract for inference execution.
type Gateway interface {
	Infer(ctx context.Context, request GatewayRequest) (GatewayResponse, error)
}
