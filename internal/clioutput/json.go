package clioutput

import (
	"encoding/json"
	"fmt"
	"io"
)

// WriteJSON encodes one JSON value without HTML escaping.
func WriteJSON(writer io.Writer, value any) error {
	if writer == nil {
		return fmt.Errorf("writer is required")
	}

	encoder := json.NewEncoder(writer)
	encoder.SetEscapeHTML(false)
	if err := encoder.Encode(value); err != nil {
		return fmt.Errorf("failed to write JSON output; %w", err)
	}

	return nil
}
