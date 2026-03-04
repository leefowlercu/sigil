package runtime

import (
	"errors"
	"strings"
	"testing"
)

func TestBuildAndParseActionOutputRef(t *testing.T) {
	nodeID := mustUUIDv7String(t)
	stepID := mustUUIDv7String(t)

	outputRef, err := BuildActionOutputRef(nodeID, stepID, 1)
	if err != nil {
		t.Fatalf("expected output ref build success, got %v", err)
	}

	parsed, err := ParseActionOutputRef(outputRef)
	if err != nil {
		t.Fatalf("expected output ref parse success, got %v", err)
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

func TestParseActionOutputRefRejectsInvalidValues(t *testing.T) {
	nodeID := mustUUIDv7String(t)
	stepID := mustUUIDv7String(t)

	testCases := []struct {
		name      string
		outputRef string
	}{
		{name: "empty", outputRef: ""},
		{name: "wrong prefix", outputRef: "artifact://node/x/step/y/action-1.json"},
		{name: "bad action index", outputRef: "run-artifact://node/" + nodeID + "/step/" + stepID + "/action-0.json"},
		{name: "bad node id", outputRef: "run-artifact://node/not-a-uuid/step/" + stepID + "/action-1.json"},
		{name: "bad step id", outputRef: "run-artifact://node/" + nodeID + "/step/not-a-uuid/action-1.json"},
	}

	for _, tc := range testCases {
		t.Run(tc.name, func(t *testing.T) {
			_, err := ParseActionOutputRef(tc.outputRef)
			if !errors.Is(err, ErrInvalidEvent) {
				t.Fatalf("expected ErrInvalidEvent, got %v", err)
			}
		})
	}
}

func TestActionOutputRefPrefixConstant(t *testing.T) {
	if !strings.HasPrefix(ActionOutputRefPrefix, "run-artifact://") {
		t.Fatalf("expected canonical run-artifact prefix, got %q", ActionOutputRefPrefix)
	}
}
