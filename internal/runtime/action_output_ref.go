package runtime

import (
	"fmt"
	"regexp"
	"strconv"
	"strings"
)

const (
	// ActionOutputRefPrefix is the canonical v1 prefix for action artifact references.
	ActionOutputRefPrefix = "run-artifact://"
)

var actionOutputRefPattern = regexp.MustCompile(`^run-artifact://node/([^/]+)/step/([^/]+)/action-([1-9][0-9]*)\.json$`)

// ActionOutputRef describes the canonical action artifact output reference identity.
type ActionOutputRef struct {
	NodeID      string
	StepID      string
	ActionIndex int
}

// BuildActionOutputRef builds canonical v1 action artifact output references.
func BuildActionOutputRef(nodeID string, stepID string, actionIndex int) (string, error) {
	if err := validateUUIDv7String(nodeID); err != nil {
		return "", fmt.Errorf("node_id must be UUIDv7; %w", ErrInvalidEvent)
	}
	if err := validateUUIDv7String(stepID); err != nil {
		return "", fmt.Errorf("step_id must be UUIDv7; %w", ErrInvalidEvent)
	}
	if actionIndex < 1 {
		return "", fmt.Errorf("action_index must be >= 1; %w", ErrInvalidEvent)
	}

	return fmt.Sprintf("run-artifact://node/%s/step/%s/action-%d.json", nodeID, stepID, actionIndex), nil
}

// ParseActionOutputRef parses canonical v1 action artifact output references.
func ParseActionOutputRef(outputRef string) (ActionOutputRef, error) {
	trimmed := strings.TrimSpace(outputRef)
	if trimmed == "" {
		return ActionOutputRef{}, fmt.Errorf("output_ref is required; %w", ErrInvalidEvent)
	}

	matches := actionOutputRefPattern.FindStringSubmatch(trimmed)
	if len(matches) != 4 {
		return ActionOutputRef{}, fmt.Errorf("output_ref %q does not match canonical action artifact reference format; %w", outputRef, ErrInvalidEvent)
	}

	actionIndex, err := strconv.Atoi(matches[3])
	if err != nil {
		return ActionOutputRef{}, fmt.Errorf("output_ref action index is invalid; %w", ErrInvalidEvent)
	}

	parsed := ActionOutputRef{
		NodeID:      matches[1],
		StepID:      matches[2],
		ActionIndex: actionIndex,
	}

	if err := validateUUIDv7String(parsed.NodeID); err != nil {
		return ActionOutputRef{}, fmt.Errorf("output_ref node id must be UUIDv7; %w", ErrInvalidEvent)
	}
	if err := validateUUIDv7String(parsed.StepID); err != nil {
		return ActionOutputRef{}, fmt.Errorf("output_ref step id must be UUIDv7; %w", ErrInvalidEvent)
	}

	return parsed, nil
}
