package runtime

import (
	"fmt"
	"regexp"
	"strconv"
	"strings"
)

const (
	// ArtifactRefPrefix is the canonical v1 prefix for persisted run artifacts.
	ArtifactRefPrefix = "run-artifact://"
)

var (
	actionArtifactRefPattern         = regexp.MustCompile(`^run-artifact://node/([^/]+)/step/([^/]+)/action-([1-9][0-9]*)\.json$`)
	nodeContextArtifactRefPattern    = regexp.MustCompile(`^run-artifact://node/([^/]+)/context\.json$`)
	nodeFinalArtifactRefPattern      = regexp.MustCompile(`^run-artifact://node/([^/]+)/final-answer\.json$`)
	nodeAccountingArtifactRefPattern = regexp.MustCompile(`^run-artifact://node/([^/]+)/accounting\.json$`)
	turnUserArtifactRefPattern       = regexp.MustCompile(`^run-artifact://node/([^/]+)/step/([^/]+)/turn-user\.json$`)
	turnModelArtifactRefPattern      = regexp.MustCompile(`^run-artifact://node/([^/]+)/step/([^/]+)/turn-model\.json$`)
	stepAccountingRefPattern         = regexp.MustCompile(`^run-artifact://node/([^/]+)/step/([^/]+)/accounting\.json$`)
	subcallAccountingRefPattern      = regexp.MustCompile(`^run-artifact://node/([^/]+)/step/([^/]+)/subcall-([1-9][0-9]*)-accounting\.json$`)
	runAccountingArtifactRefPattern  = regexp.MustCompile(`^run-artifact://run/accounting\.json$`)
	runSubmittedConfigRefPattern     = regexp.MustCompile(`^run-artifact://run/submitted-run-config\.json$`)
)

// ActionArtifactRef describes the canonical action artifact reference identity.
type ActionArtifactRef struct {
	NodeID      string
	StepID      string
	ActionIndex int
}

// BuildActionArtifactRef builds canonical v1 action artifact references.
func BuildActionArtifactRef(nodeID string, stepID string, actionIndex int) (string, error) {
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

// ParseActionArtifactRef parses canonical v1 action artifact references.
func ParseActionArtifactRef(actionRef string) (ActionArtifactRef, error) {
	trimmed := strings.TrimSpace(actionRef)
	if trimmed == "" {
		return ActionArtifactRef{}, fmt.Errorf("action_ref is required; %w", ErrInvalidEvent)
	}

	matches := actionArtifactRefPattern.FindStringSubmatch(trimmed)
	if len(matches) != 4 {
		return ActionArtifactRef{}, fmt.Errorf("action_ref %q does not match canonical action artifact reference format; %w", actionRef, ErrInvalidEvent)
	}

	actionIndex, err := strconv.Atoi(matches[3])
	if err != nil {
		return ActionArtifactRef{}, fmt.Errorf("action_ref action index is invalid; %w", ErrInvalidEvent)
	}

	parsed := ActionArtifactRef{
		NodeID:      matches[1],
		StepID:      matches[2],
		ActionIndex: actionIndex,
	}
	if err := validateUUIDv7String(parsed.NodeID); err != nil {
		return ActionArtifactRef{}, fmt.Errorf("action_ref node id must be UUIDv7; %w", ErrInvalidEvent)
	}
	if err := validateUUIDv7String(parsed.StepID); err != nil {
		return ActionArtifactRef{}, fmt.Errorf("action_ref step id must be UUIDv7; %w", ErrInvalidEvent)
	}

	return parsed, nil
}

// ResolveArtifactRefPath converts one canonical artifact ref into a run-local relative path.
func ResolveArtifactRefPath(artifactRef string) ([]string, error) {
	trimmed := strings.TrimSpace(artifactRef)
	if trimmed == "" {
		return nil, fmt.Errorf("artifact_ref is required; %w", ErrInvalidEvent)
	}
	if !strings.HasPrefix(trimmed, ArtifactRefPrefix) {
		return nil, fmt.Errorf("artifact_ref %q must use %q; %w", artifactRef, ArtifactRefPrefix, ErrInvalidEvent)
	}

	if parsed, err := ParseActionArtifactRef(trimmed); err == nil {
		return []string{
			"node",
			parsed.NodeID,
			"step",
			parsed.StepID,
			fmt.Sprintf("action-%d.json", parsed.ActionIndex),
		}, nil
	}
	if matches := nodeContextArtifactRefPattern.FindStringSubmatch(trimmed); len(matches) == 2 {
		return []string{"node", matches[1], "context.json"}, nil
	}
	if matches := nodeFinalArtifactRefPattern.FindStringSubmatch(trimmed); len(matches) == 2 {
		return []string{"node", matches[1], "final-answer.json"}, nil
	}
	if matches := nodeAccountingArtifactRefPattern.FindStringSubmatch(trimmed); len(matches) == 2 {
		return []string{"node", matches[1], "accounting.json"}, nil
	}
	if matches := turnUserArtifactRefPattern.FindStringSubmatch(trimmed); len(matches) == 3 {
		return []string{"node", matches[1], "step", matches[2], "turn-user.json"}, nil
	}
	if matches := turnModelArtifactRefPattern.FindStringSubmatch(trimmed); len(matches) == 3 {
		return []string{"node", matches[1], "step", matches[2], "turn-model.json"}, nil
	}
	if matches := stepAccountingRefPattern.FindStringSubmatch(trimmed); len(matches) == 3 {
		return []string{"node", matches[1], "step", matches[2], "accounting.json"}, nil
	}
	if matches := subcallAccountingRefPattern.FindStringSubmatch(trimmed); len(matches) == 4 {
		return []string{"node", matches[1], "step", matches[2], fmt.Sprintf("subcall-%s-accounting.json", matches[3])}, nil
	}
	if runAccountingArtifactRefPattern.MatchString(trimmed) {
		return []string{"run", "accounting.json"}, nil
	}
	if runSubmittedConfigRefPattern.MatchString(trimmed) {
		return []string{"run", "submitted-run-config.json"}, nil
	}

	return nil, fmt.Errorf("artifact_ref %q is not a supported canonical artifact reference; %w", artifactRef, ErrInvalidEvent)
}
