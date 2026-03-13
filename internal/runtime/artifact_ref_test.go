package runtime

import (
	"errors"
	"strings"
	"testing"
)

func TestBuildAndParseActionArtifactRef(t *testing.T) {
	nodeID := mustUUIDv7String(t)
	stepID := mustUUIDv7String(t)

	actionRef, err := BuildActionArtifactRef(nodeID, stepID, 1)
	if err != nil {
		t.Fatalf("expected action ref build success, got %v", err)
	}

	parsed, err := ParseActionArtifactRef(actionRef)
	if err != nil {
		t.Fatalf("expected action ref parse success, got %v", err)
	}

	if parsed.NodeID != nodeID {
		t.Fatalf("expected node id %q, got %q", nodeID, parsed.NodeID)
	}
	if parsed.StepID != stepID {
		t.Fatalf("expected step id %q, got %q", stepID, parsed.StepID)
	}
	if parsed.ActionIndex != 1 {
		t.Fatalf("expected action index 1, got %d", parsed.ActionIndex)
	}
}

func TestParseActionArtifactRefRejectsInvalidValues(t *testing.T) {
	nodeID := mustUUIDv7String(t)
	stepID := mustUUIDv7String(t)

	testCases := []struct {
		name      string
		actionRef string
	}{
		{name: "empty", actionRef: ""},
		{name: "wrong prefix", actionRef: "artifact://node/x/step/y/action-1.json"},
		{name: "bad action index", actionRef: "run-artifact://node/" + nodeID + "/step/" + stepID + "/action-0.json"},
		{name: "bad node id", actionRef: "run-artifact://node/not-a-uuid/step/" + stepID + "/action-1.json"},
		{name: "bad step id", actionRef: "run-artifact://node/" + nodeID + "/step/not-a-uuid/action-1.json"},
	}

	for _, tc := range testCases {
		t.Run(tc.name, func(t *testing.T) {
			_, err := ParseActionArtifactRef(tc.actionRef)
			if !errors.Is(err, ErrInvalidEvent) {
				t.Fatalf("expected ErrInvalidEvent, got %v", err)
			}
		})
	}
}

func TestArtifactRefPrefixConstant(t *testing.T) {
	if !strings.HasPrefix(ArtifactRefPrefix, "run-artifact://") {
		t.Fatalf("expected canonical run-artifact prefix, got %q", ArtifactRefPrefix)
	}
}
