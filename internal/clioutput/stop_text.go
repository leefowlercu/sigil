package clioutput

import (
	"fmt"
	"io"
)

// WriteStopText renders the default human-readable stop result.
func WriteStopText(writer io.Writer, runID string, stopRequested bool, state string, eventsPath string) error {
	if writer == nil {
		return fmt.Errorf("writer is required")
	}

	if _, err := fmt.Fprintf(
		writer,
		"Run stop result\n  Run ID: %s\n  Stop requested: %t\n  State: %s\n  Events path: %s\n",
		runID,
		stopRequested,
		state,
		eventsPath,
	); err != nil {
		return fmt.Errorf("failed to write stop result; %w", err)
	}

	return nil
}
