package harness

import (
	"fmt"
	"strings"

	"github.com/leefowlercu/sigil/internal/runtime"
)

const (
	finalAnswerStartMarker = "FINAL_ANSWER_START"
	finalAnswerEndMarker   = "FINAL_ANSWER_END"
)

func normalizeFinalAnswer(answer string) string {
	return strings.TrimSpace(answer)
}

func finalAnswerCandidateFromEvidence(runID string, artifacts *ActionArtifactStore, evidence []FinalEvidence) (string, bool, error) {
	if artifacts == nil {
		return "", false, fmt.Errorf("artifact store is required")
	}
	for _, item := range evidence {
		if _, err := runtime.ParseActionArtifactRef(item.Ref); err != nil {
			continue
		}
		artifact, err := artifacts.Read(runID, item.Ref)
		if err != nil {
			return "", false, err
		}
		if artifact.Status != "completed" {
			continue
		}
		candidate, ok := extractMarkedFinalAnswerCandidate(artifact.Stdout)
		if ok {
			return candidate, true, nil
		}
	}
	return "", false, nil
}

func extractMarkedFinalAnswerCandidate(stdout string) (string, bool) {
	start := strings.Index(stdout, finalAnswerStartMarker)
	if start < 0 {
		return "", false
	}
	afterStart := stdout[start+len(finalAnswerStartMarker):]
	afterStart = strings.TrimPrefix(afterStart, "\r\n")
	afterStart = strings.TrimPrefix(afterStart, "\n")

	end := strings.Index(afterStart, finalAnswerEndMarker)
	if end < 0 {
		return "", false
	}
	candidate := afterStart[:end]
	candidate = strings.TrimSuffix(candidate, "\r\n")
	candidate = strings.TrimSuffix(candidate, "\n")
	if strings.TrimSpace(candidate) == "" {
		return "", false
	}
	return candidate, true
}
