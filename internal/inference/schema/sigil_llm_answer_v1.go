package schema

import (
	"fmt"
	"strings"
)

func newSigilLLMAnswerV1Definition() Definition {
	jsonSchema := map[string]any{
		"type": "object",
		"properties": map[string]any{
			"answer": map[string]any{
				"type":      "string",
				"minLength": 1,
			},
		},
		"required":             []any{"answer"},
		"additionalProperties": false,
	}

	return Definition{
		ID:         SigilLLMAnswerV1SchemaID,
		Name:       "sigil_llm_answer_v1",
		JSONSchema: jsonSchema,
		Validate: func(payload map[string]any) error {
			if payload == nil {
				return fmt.Errorf("payload is required")
			}

			if len(payload) != 1 {
				return fmt.Errorf("payload must contain only answer")
			}

			answerRaw, ok := payload["answer"]
			if !ok {
				return fmt.Errorf("answer is required")
			}
			answer, ok := answerRaw.(string)
			if !ok {
				return fmt.Errorf("answer must be string")
			}
			if strings.TrimSpace(answer) == "" {
				return fmt.Errorf("answer must be non-empty")
			}

			return nil
		},
	}
}
