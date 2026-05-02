package inference

import (
	"context"
	"fmt"
	"strings"
	"testing"
	"time"

	"github.com/leefowlercu/sigil/internal/accounting"
	"github.com/leefowlercu/sigil/internal/inference/schema"
)

type gatewayResult struct {
	response GatewayResponse
	err      error
}

type sequenceGateway struct {
	calls   int
	results []gatewayResult
}

func (g *sequenceGateway) Infer(_ context.Context, _ GatewayRequest) (GatewayResponse, error) {
	g.calls++
	index := g.calls - 1
	if index >= len(g.results) {
		index = len(g.results) - 1
	}
	if index < 0 {
		return GatewayResponse{}, fmt.Errorf("gateway has no configured results")
	}

	return g.results[index].response, g.results[index].err
}

func makeService(t *testing.T, gateway Gateway, options ...ServiceOption) *Service {
	t.Helper()
	registry := NewRegistry()
	if err := registry.Register("openrouter", func(_ Request) (Gateway, error) {
		return gateway, nil
	}); err != nil {
		t.Fatalf("failed to register gateway: %v", err)
	}

	schemaRegistry := schema.NewRegistry()
	service, err := NewService(registry, schemaRegistry, options...)
	if err != nil {
		t.Fatalf("failed to create service: %v", err)
	}

	return service
}

func baseRequest() Request {
	return Request{
		Messages: []Message{
			{Role: MessageRoleSystem, Content: "system"},
			{Role: MessageRoleUser, Content: "{\"query\":\"prompt\"}"},
		},
		SchemaID: schema.SigilRLMResponseV1SchemaID,
		Gateway:  "openrouter",
		Provider: "openai",
		Model:    "gpt-5.1",
		Reasoning: ReasoningConfig{
			Enabled: true,
			Effort:  "medium",
		},
	}
}

func TestInferRejectsInvalidMessagesWithTypedGatewayResolutionError(t *testing.T) {
	gateway := &sequenceGateway{results: []gatewayResult{{response: GatewayResponse{StructuredPayload: validFinalPayload()}}}}
	service := makeService(t, gateway)

	request := baseRequest()
	request.Messages = nil
	_, err := service.Infer(context.Background(), request)
	if !IsCode(err, ErrorCodeGatewayResolution) {
		t.Fatalf("expected gateway resolution error code for invalid messages, got %v", err)
	}

	request = baseRequest()
	request.Messages = []Message{{Role: "invalid", Content: "hello"}}
	_, err = service.Infer(context.Background(), request)
	if !IsCode(err, ErrorCodeGatewayResolution) {
		t.Fatalf("expected gateway resolution error code for invalid role, got %v", err)
	}
}

func validFinalPayload() map[string]any {
	return map[string]any{
		"decision": "continue",
		"continuation": map[string]any{
			"repl_code":            "fmt.Println(\"FINAL_ANSWER_START\")\nfmt.Println(\"done\")\nfmt.Println(\"FINAL_ANSWER_END\")",
			"intent":               "Emit verified final answer.",
			"expected_observation": "A final-answer block containing done.",
		},
	}
}

func TestInferFailsWithTypedGatewayResolutionError(t *testing.T) {
	service, err := NewService(NewRegistry(), schema.NewRegistry())
	if err != nil {
		t.Fatalf("expected service construction success, got %v", err)
	}

	_, runErr := service.Infer(context.Background(), baseRequest())
	if !IsCode(runErr, ErrorCodeGatewayResolution) {
		t.Fatalf("expected gateway resolution error code, got %v", runErr)
	}
}

func TestInferFailsWithTypedSchemaLookupError(t *testing.T) {
	gateway := &sequenceGateway{results: []gatewayResult{{response: GatewayResponse{StructuredPayload: validFinalPayload()}}}}
	service := makeService(t, gateway)

	request := baseRequest()
	request.SchemaID = "sigil.unknown.v1"

	_, runErr := service.Infer(context.Background(), request)
	if !IsCode(runErr, ErrorCodeSchemaLookup) {
		t.Fatalf("expected schema lookup error code, got %v", runErr)
	}
	if gateway.calls != 0 {
		t.Fatalf("expected zero gateway calls for schema lookup failure, got %d", gateway.calls)
	}
}

func TestInferFailsWithTypedOutputValidationError(t *testing.T) {
	gateway := &sequenceGateway{results: []gatewayResult{{response: GatewayResponse{StructuredPayload: map[string]any{"decision": "final"}}}}}
	service := makeService(t, gateway)

	_, runErr := service.Infer(context.Background(), baseRequest())
	if !IsCode(runErr, ErrorCodeOutputValidation) {
		t.Fatalf("expected output validation error code, got %v", runErr)
	}
}

func TestInferFailsWithTypedReasoningCapabilityError(t *testing.T) {
	gateway := &sequenceGateway{results: []gatewayResult{{err: &ReasoningCapabilityError{Message: "unsupported reasoning"}}}}
	service := makeService(t, gateway)

	_, runErr := service.Infer(context.Background(), baseRequest())
	if !IsCode(runErr, ErrorCodeReasoningCapability) {
		t.Fatalf("expected reasoning capability error code, got %v", runErr)
	}
}

func TestInferRetriesTransientGatewayFailuresWithBoundedPolicy(t *testing.T) {
	gateway := &sequenceGateway{results: []gatewayResult{
		{err: &GatewayHTTPError{StatusCode: 429, Body: "rate limit"}},
		{err: &GatewayHTTPError{StatusCode: 500, Body: "upstream failure"}},
		{response: GatewayResponse{StructuredPayload: validFinalPayload()}},
	}}

	recordedDelays := make([]time.Duration, 0, 2)
	service := makeService(
		t,
		gateway,
		WithRetryPolicy(RetryPolicy{
			MaxAttempts: 3,
			BaseDelay:   250 * time.Millisecond,
			MaxDelay:    2 * time.Second,
			Jitter: func(_ int, delay time.Duration) time.Duration {
				return delay
			},
			Sleep: func(_ context.Context, delay time.Duration) error {
				recordedDelays = append(recordedDelays, delay)
				return nil
			},
		}),
	)

	result, runErr := service.Infer(context.Background(), baseRequest())
	if runErr != nil {
		t.Fatalf("expected inference success after retries, got %v", runErr)
	}
	if result.ValidatedPayload["decision"] != "continue" {
		t.Fatalf("expected action-only decision payload, got %+v", result.ValidatedPayload)
	}
	if gateway.calls != 3 {
		t.Fatalf("expected 3 gateway calls, got %d", gateway.calls)
	}
	if len(recordedDelays) != 2 {
		t.Fatalf("expected 2 retry delays, got %d", len(recordedDelays))
	}
	if recordedDelays[0] != 250*time.Millisecond {
		t.Fatalf("expected first retry delay 250ms, got %s", recordedDelays[0])
	}
	if recordedDelays[1] != 500*time.Millisecond {
		t.Fatalf("expected second retry delay 500ms, got %s", recordedDelays[1])
	}
}

func TestInferFailsWithTypedGatewayFailureAfterRetryExhaustion(t *testing.T) {
	gateway := &sequenceGateway{results: []gatewayResult{
		{err: &GatewayHTTPError{StatusCode: 429, Body: "rate limit"}},
		{err: &GatewayHTTPError{StatusCode: 500, Body: "upstream failure"}},
		{err: &GatewayHTTPError{StatusCode: 503, Body: "service unavailable"}},
	}}

	service := makeService(
		t,
		gateway,
		WithRetryPolicy(RetryPolicy{
			MaxAttempts: 3,
			BaseDelay:   250 * time.Millisecond,
			MaxDelay:    2 * time.Second,
			Jitter: func(_ int, delay time.Duration) time.Duration {
				return delay
			},
			Sleep: func(_ context.Context, _ time.Duration) error { return nil },
		}),
	)

	_, runErr := service.Infer(context.Background(), baseRequest())
	if !IsCode(runErr, ErrorCodeGatewayFailure) {
		t.Fatalf("expected gateway failure error code, got %v", runErr)
	}
	if gateway.calls != 3 {
		t.Fatalf("expected 3 gateway calls, got %d", gateway.calls)
	}
}

func TestInferDoesNotRetryNonTransientGatewayFailures(t *testing.T) {
	gateway := &sequenceGateway{results: []gatewayResult{{err: &GatewayHTTPError{StatusCode: 400, Body: "bad request"}}}}
	service := makeService(t, gateway)

	_, runErr := service.Infer(context.Background(), baseRequest())
	if !IsCode(runErr, ErrorCodeGatewayFailure) {
		t.Fatalf("expected gateway failure error code, got %v", runErr)
	}
	if gateway.calls != 1 {
		t.Fatalf("expected one gateway call for non-transient failure, got %d", gateway.calls)
	}
}

func TestInferReturnsCanonicalNormalizedResultShape(t *testing.T) {
	reasoningTokens := int64(7)
	gateway := &sequenceGateway{results: []gatewayResult{{response: GatewayResponse{
		GatewayResponseID: "resp_123",
		Provider:          "openai",
		Model:             "gpt-5.1",
		FinishStatus:      "completed",
		StructuredPayload: validFinalPayload(),
		Usage: Usage{
			InputTokens:     10,
			OutputTokens:    5,
			TotalTokens:     15,
			ReasoningTokens: &reasoningTokens,
		},
		Reasoning:   Reasoning{Artifacts: map[string]any{"summary": "reasoned"}},
		RawMetadata: map[string]any{"id": "resp_123"},
	}}}}
	service := makeService(t, gateway)

	result, runErr := service.Infer(context.Background(), baseRequest())
	if runErr != nil {
		t.Fatalf("expected inference success, got %v", runErr)
	}

	requiredStringFields := []string{
		result.SchemaID,
		result.Gateway,
		result.Provider,
		result.Model,
		result.GatewayResponseID,
		result.FinishStatus,
	}
	for index, value := range requiredStringFields {
		if strings.TrimSpace(value) == "" {
			t.Fatalf("required canonical string field index %d is empty", index)
		}
	}
	if result.ValidatedPayload == nil {
		t.Fatal("expected validated_payload in canonical result")
	}
	if result.RawMetadata == nil {
		t.Fatal("expected raw_metadata in canonical result")
	}
	if result.Usage.ReasoningTokens == nil || *result.Usage.ReasoningTokens != 7 {
		t.Fatalf("expected usage.reasoning_tokens=7, got %+v", result.Usage)
	}
	if !result.Reasoning.Enabled {
		t.Fatal("expected reasoning.enabled=true")
	}
	if result.Reasoning.Effort == nil || *result.Reasoning.Effort != "medium" {
		t.Fatalf("expected reasoning.effort medium, got %+v", result.Reasoning)
	}
	if result.Reasoning.Artifacts["summary"] != "reasoned" {
		t.Fatalf("expected reasoning artifact summary, got %+v", result.Reasoning.Artifacts)
	}
}

func TestInferSetsReasoningEffortToNilWhenDisabled(t *testing.T) {
	gateway := &sequenceGateway{results: []gatewayResult{{response: GatewayResponse{StructuredPayload: validFinalPayload()}}}}
	service := makeService(t, gateway)

	request := baseRequest()
	request.Reasoning = ReasoningConfig{Enabled: false, Effort: "high"}

	result, runErr := service.Infer(context.Background(), request)
	if runErr != nil {
		t.Fatalf("expected inference success, got %v", runErr)
	}
	if result.Reasoning.Enabled {
		t.Fatal("expected reasoning.enabled=false")
	}
	if result.Reasoning.Effort != nil {
		t.Fatalf("expected reasoning.effort=nil when disabled, got %+v", result.Reasoning)
	}
}

func TestInferDerivesFallbackCostWhenGatewayReportsUsageOnly(t *testing.T) {
	gateway := &sequenceGateway{results: []gatewayResult{{response: GatewayResponse{
		Provider:          "openai",
		Model:             "gpt-5.1",
		FinishStatus:      "completed",
		StructuredPayload: validFinalPayload(),
		UsageReported:     true,
		Usage: Usage{
			InputTokens:  1_000_000,
			OutputTokens: 1_000_000,
			TotalTokens:  2_000_000,
		},
	}}}}
	service := makeService(t, gateway)

	request := baseRequest()
	request.Accounting = AccountingConfig{
		PricingVersion: "v1",
		FallbackPricing: &accounting.FallbackPricing{
			InputMicrousdPerMillionTokens:  100,
			OutputMicrousdPerMillionTokens: 200,
		},
	}

	result, runErr := service.Infer(context.Background(), request)
	if runErr != nil {
		t.Fatalf("expected inference success, got %v", runErr)
	}
	if result.Accounting.CostSource != accounting.SourceFallbackPricing {
		t.Fatalf("expected fallback pricing cost source, got %q", result.Accounting.CostSource)
	}
	if result.Accounting.KnownTotalCostMicrousd == nil || *result.Accounting.KnownTotalCostMicrousd != 300 {
		t.Fatalf("expected known_total_cost_microusd=300, got %+v", result.Accounting.KnownTotalCostMicrousd)
	}
}

func TestInferUsesMixedGatewayAndFallbackCostAccounting(t *testing.T) {
	inputCost := int64(100)
	gateway := &sequenceGateway{results: []gatewayResult{{response: GatewayResponse{
		Provider:          "openai",
		Model:             "gpt-5.1",
		FinishStatus:      "completed",
		StructuredPayload: validFinalPayload(),
		UsageReported:     true,
		Usage: Usage{
			InputTokens:  1_000_000,
			OutputTokens: 1_000_000,
			TotalTokens:  2_000_000,
		},
		Accounting: accounting.Summary{
			Currency:               accounting.CurrencyUSD,
			KnownInputCostMicrousd: &inputCost,
			TokenSource:            accounting.SourceGatewayReported,
			TokenStatus:            accounting.StatusComplete,
			CostSource:             accounting.SourceGatewayReported,
			CostStatus:             accounting.StatusPartial,
			PricingKey:             accounting.PricingKey{Provider: "openai", Model: "gpt-5.1"},
			MissingTokenItemCount:  0,
			MissingCostItemCount:   1,
		},
	}}}}
	service := makeService(t, gateway)

	request := baseRequest()
	request.Accounting = AccountingConfig{
		PricingVersion: "v1",
		FallbackPricing: &accounting.FallbackPricing{
			InputMicrousdPerMillionTokens:  100,
			OutputMicrousdPerMillionTokens: 200,
		},
	}

	result, runErr := service.Infer(context.Background(), request)
	if runErr != nil {
		t.Fatalf("expected inference success, got %v", runErr)
	}
	if result.Accounting.CostSource != accounting.SourceMixed {
		t.Fatalf("expected mixed cost source, got %q", result.Accounting.CostSource)
	}
	if result.Accounting.KnownInputCostMicrousd == nil || *result.Accounting.KnownInputCostMicrousd != 100 {
		t.Fatalf("expected gateway input cost 100, got %+v", result.Accounting.KnownInputCostMicrousd)
	}
	if result.Accounting.KnownOutputCostMicrousd == nil || *result.Accounting.KnownOutputCostMicrousd != 200 {
		t.Fatalf("expected fallback output cost 200, got %+v", result.Accounting.KnownOutputCostMicrousd)
	}
	if result.Accounting.KnownTotalCostMicrousd == nil || *result.Accounting.KnownTotalCostMicrousd != 300 {
		t.Fatalf("expected combined total cost 300, got %+v", result.Accounting.KnownTotalCostMicrousd)
	}
}

func TestInferPreservesMissingUsageCountersWhenApplyingAccountingNormalization(t *testing.T) {
	inputTokens := int64(1_000_000)
	gateway := &sequenceGateway{results: []gatewayResult{{response: GatewayResponse{
		Provider:          "openai",
		Model:             "gpt-5.1",
		FinishStatus:      "completed",
		StructuredPayload: validFinalPayload(),
		UsageReported:     true,
		Usage: Usage{
			InputTokens: 1_000_000,
		},
		Accounting: accounting.BuildLeafSummary(accounting.LeafInput{
			Provider:    "openai",
			Model:       "gpt-5.1",
			InputTokens: &inputTokens,
		}),
	}}}}
	service := makeService(t, gateway)

	request := baseRequest()
	request.Accounting = AccountingConfig{
		PricingVersion: "v1",
		FallbackPricing: &accounting.FallbackPricing{
			InputMicrousdPerMillionTokens:  100,
			OutputMicrousdPerMillionTokens: 200,
		},
	}

	result, runErr := service.Infer(context.Background(), request)
	if runErr != nil {
		t.Fatalf("expected inference success, got %v", runErr)
	}
	if result.Accounting.InputTokens == nil || *result.Accounting.InputTokens != 1_000_000 {
		t.Fatalf("expected input_tokens=1000000, got %+v", result.Accounting.InputTokens)
	}
	if result.Accounting.OutputTokens != nil {
		t.Fatalf("expected output_tokens to remain unknown, got %+v", result.Accounting.OutputTokens)
	}
	if result.Accounting.TotalTokens != nil {
		t.Fatalf("expected total_tokens to remain unknown, got %+v", result.Accounting.TotalTokens)
	}
	if result.Accounting.KnownOutputCostMicrousd != nil {
		t.Fatalf("expected output fallback cost to remain unknown, got %+v", result.Accounting.KnownOutputCostMicrousd)
	}
	if result.Accounting.CostStatus != accounting.StatusPartial {
		t.Fatalf("expected partial cost status, got %q", result.Accounting.CostStatus)
	}
	if result.Accounting.KnownTotalCostMicrousd == nil || *result.Accounting.KnownTotalCostMicrousd != 100 {
		t.Fatalf("expected fallback subtotal cost 100, got %+v", result.Accounting.KnownTotalCostMicrousd)
	}
}

func TestInferBackfillsDerivedTotalTokensWhenGatewayAccountingIsPartial(t *testing.T) {
	inputTokens := int64(1_000_000)
	outputTokens := int64(500_000)

	gateway := &sequenceGateway{results: []gatewayResult{{response: GatewayResponse{
		Provider:          "openai",
		Model:             "gpt-5.1",
		FinishStatus:      "completed",
		StructuredPayload: validFinalPayload(),
		UsageReported:     true,
		Usage: Usage{
			InputTokens:  inputTokens,
			OutputTokens: outputTokens,
			TotalTokens:  inputTokens + outputTokens,
		},
		Accounting: accounting.BuildLeafSummary(accounting.LeafInput{
			Provider:       "openai",
			Model:          "gpt-5.1",
			InputTokens:    &inputTokens,
			OutputTokens:   &outputTokens,
			PricingVersion: "v1",
		}),
	}}}}
	service := makeService(t, gateway)

	request := baseRequest()
	request.Accounting = AccountingConfig{
		PricingVersion: "v1",
		FallbackPricing: &accounting.FallbackPricing{
			InputMicrousdPerMillionTokens:  100,
			OutputMicrousdPerMillionTokens: 200,
		},
	}

	result, runErr := service.Infer(context.Background(), request)
	if runErr != nil {
		t.Fatalf("expected inference success, got %v", runErr)
	}
	if result.Accounting.TotalTokens == nil || *result.Accounting.TotalTokens != 1_500_000 {
		t.Fatalf("expected total_tokens=1500000, got %+v", result.Accounting.TotalTokens)
	}
	if result.Accounting.TokenStatus != accounting.StatusComplete {
		t.Fatalf("expected complete token status, got %q", result.Accounting.TokenStatus)
	}
	if result.Accounting.KnownTotalCostMicrousd == nil || *result.Accounting.KnownTotalCostMicrousd != 200 {
		t.Fatalf("expected known_total_cost_microusd=200, got %+v", result.Accounting.KnownTotalCostMicrousd)
	}
}

func TestInferPreservesReportedZeroUsageCountersDuringAccountingNormalization(t *testing.T) {
	gateway := &sequenceGateway{results: []gatewayResult{{response: GatewayResponse{
		Provider:          "openai",
		Model:             "gpt-5.1",
		FinishStatus:      "completed",
		StructuredPayload: validFinalPayload(),
		UsageReported:     true,
		Usage: Usage{
			InputTokens:  42,
			OutputTokens: 0,
			TotalTokens:  42,
		},
	}}}}
	service := makeService(t, gateway)

	result, runErr := service.Infer(context.Background(), baseRequest())
	if runErr != nil {
		t.Fatalf("expected inference success, got %v", runErr)
	}
	if result.Accounting.InputTokens == nil || *result.Accounting.InputTokens != 42 {
		t.Fatalf("expected input_tokens=42, got %+v", result.Accounting.InputTokens)
	}
	if result.Accounting.OutputTokens == nil || *result.Accounting.OutputTokens != 0 {
		t.Fatalf("expected output_tokens=0, got %+v", result.Accounting.OutputTokens)
	}
	if result.Accounting.TotalTokens == nil || *result.Accounting.TotalTokens != 42 {
		t.Fatalf("expected total_tokens=42, got %+v", result.Accounting.TotalTokens)
	}
	if result.Accounting.TokenStatus != accounting.StatusComplete {
		t.Fatalf("expected complete token status, got %q", result.Accounting.TokenStatus)
	}
}

func TestInferKeepsFallbackPricingKeyOnRequestedModelWhenGatewayAliasesResponseModel(t *testing.T) {
	gateway := &sequenceGateway{results: []gatewayResult{{response: GatewayResponse{
		Provider:          "openai",
		Model:             "gpt-5.1-alias",
		FinishStatus:      "completed",
		StructuredPayload: validFinalPayload(),
		UsageReported:     true,
		Usage: Usage{
			InputTokens:  1_000_000,
			OutputTokens: 1_000_000,
			TotalTokens:  2_000_000,
		},
	}}}}
	service := makeService(t, gateway)

	request := baseRequest()
	request.Accounting = AccountingConfig{
		PricingVersion: "v1",
		FallbackPricing: &accounting.FallbackPricing{
			InputMicrousdPerMillionTokens:  100,
			OutputMicrousdPerMillionTokens: 200,
		},
	}

	result, runErr := service.Infer(context.Background(), request)
	if runErr != nil {
		t.Fatalf("expected inference success, got %v", runErr)
	}
	if result.Accounting.CostSource != accounting.SourceFallbackPricing {
		t.Fatalf("expected fallback pricing cost source, got %q", result.Accounting.CostSource)
	}
	if result.Accounting.PricingKey.Provider != request.Provider {
		t.Fatalf("expected pricing key provider %q, got %q", request.Provider, result.Accounting.PricingKey.Provider)
	}
	if result.Accounting.PricingKey.Model != request.Model {
		t.Fatalf("expected pricing key model %q, got %q", request.Model, result.Accounting.PricingKey.Model)
	}
}
