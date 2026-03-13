package harness

import (
	"errors"
	"fmt"
	"os"
	"path/filepath"
	"regexp"
	"strconv"
	"strings"

	"github.com/leefowlercu/sigil/internal/runtime"
)

var (
	malformedActionRefPattern = regexp.MustCompile(`^run-artifact://node/([^/]+)/action-([1-9][0-9]*)\.json$`)
)

func resolveFinalEvidenceRefs(runID string, runsBaseDir string, artifacts *ActionArtifactStore, currentNodeID string, evidence []FinalEvidence) error {
	if strings.TrimSpace(runID) == "" {
		return fmt.Errorf("run id is required")
	}
	if strings.TrimSpace(runsBaseDir) == "" {
		return fmt.Errorf("runs base directory is required")
	}
	if artifacts == nil {
		return fmt.Errorf("artifact store is required")
	}

	for index, item := range evidence {
		resolvedRef, err := resolveEvidenceRef(runID, runsBaseDir, artifacts, currentNodeID, item.Ref)
		if err != nil {
			return fmt.Errorf("final.evidence[%d] ref resolution failed; %w", index, err)
		}
		evidence[index].Ref = resolvedRef
	}

	return nil
}

func resolveEvidenceRef(runID string, runsBaseDir string, artifacts *ActionArtifactStore, currentNodeID string, ref string) (string, error) {
	trimmed := strings.TrimSpace(ref)
	if trimmed == "" {
		return "", fmt.Errorf("ref is required")
	}

	if strings.HasPrefix(trimmed, runtime.ArtifactRefPrefix) {
		if _, err := runtime.ParseActionArtifactRef(trimmed); err == nil {
			if _, readErr := artifacts.Read(runID, trimmed); readErr != nil {
				repairedRef, repaired := normalizeMalformedActionRef(runID, runsBaseDir, currentNodeID, trimmed)
				if repaired {
					if _, repairedErr := artifacts.Read(runID, repairedRef); repairedErr == nil {
						return repairedRef, nil
					}
				}
				return "", fmt.Errorf("failed to resolve action artifact ref %q; %w", trimmed, readErr)
			}
			return trimmed, nil
		}
		if repairedRef, repaired := normalizeMalformedActionRef(runID, runsBaseDir, currentNodeID, trimmed); repaired {
			if _, repairedErr := artifacts.Read(runID, repairedRef); repairedErr == nil {
				return repairedRef, nil
			}
		}

		artifactPath, err := runtime.ResolveArtifactRefPath(trimmed)
		if err != nil {
			return "", err
		}

		pathParts := append([]string{runsBaseDir, runID, "artifacts"}, artifactPath...)
		path := filepath.Join(pathParts...)
		info, err := os.Stat(path)
		if err != nil {
			return "", fmt.Errorf("failed to resolve artifact ref %q at %q; %w", trimmed, path, err)
		}
		if info.IsDir() {
			return "", fmt.Errorf("artifact ref %q resolved to directory %q", trimmed, path)
		}

		return trimmed, nil
	}

	return "", fmt.Errorf("unsupported artifact ref %q", ref)
}

func normalizeMalformedActionRef(runID string, runsBaseDir string, currentNodeID string, ref string) (string, bool) {
	if strings.TrimSpace(currentNodeID) == "" {
		return "", false
	}

	matches := malformedActionRefPattern.FindStringSubmatch(strings.TrimSpace(ref))
	if len(matches) != 3 {
		return "", false
	}

	actionIndex, err := strconv.Atoi(matches[2])
	if err != nil || actionIndex < 1 {
		return "", false
	}

	stepsDir := filepath.Join(runsBaseDir, runID, "artifacts", "node", currentNodeID, "step")
	entries, err := os.ReadDir(stepsDir)
	if err != nil {
		if errors.Is(err, os.ErrNotExist) {
			return "", false
		}
		return "", false
	}

	malformedNodeID := matches[1]
	candidates := make([]string, 0, 1)
	for _, entry := range entries {
		if !entry.IsDir() {
			continue
		}
		stepID := strings.TrimSpace(entry.Name())
		if !matchesHybridActionRefNodeAndStep(malformedNodeID, currentNodeID, stepID) {
			continue
		}
		artifactPath := filepath.Join(stepsDir, stepID, fmt.Sprintf("action-%d.json", actionIndex))
		info, statErr := os.Stat(artifactPath)
		if statErr != nil || info.IsDir() {
			continue
		}
		candidates = append(candidates, stepID)
	}
	if len(candidates) != 1 {
		return "", false
	}

	canonicalRef, err := runtime.BuildActionArtifactRef(currentNodeID, candidates[0], actionIndex)
	if err != nil {
		return "", false
	}
	return canonicalRef, true
}

func matchesHybridActionRefNodeAndStep(malformedNodeID string, currentNodeID string, stepID string) bool {
	trimmedMalformed := strings.TrimSpace(malformedNodeID)
	trimmedNode := strings.TrimSpace(currentNodeID)
	trimmedStep := strings.TrimSpace(stepID)
	if trimmedMalformed == "" || trimmedNode == "" || trimmedStep == "" {
		return false
	}
	if trimmedMalformed == trimmedStep {
		return true
	}

	malformedParts := strings.Split(trimmedMalformed, "-")
	nodeParts := strings.Split(trimmedNode, "-")
	stepParts := strings.Split(trimmedStep, "-")
	if len(malformedParts) != 5 || len(nodeParts) != 5 || len(stepParts) != 5 {
		return false
	}

	return malformedParts[0] == nodeParts[0] &&
		malformedParts[1] == nodeParts[1] &&
		malformedParts[2] == stepParts[2] &&
		malformedParts[3] == stepParts[3] &&
		malformedParts[4] == stepParts[4]
}

func buildMalformedHybridActionRefNodeID(currentNodeID string, stepID string) (string, bool) {
	trimmedNode := strings.TrimSpace(currentNodeID)
	trimmedStep := strings.TrimSpace(stepID)
	if trimmedNode == "" || trimmedStep == "" {
		return "", false
	}

	nodeParts := strings.Split(trimmedNode, "-")
	stepParts := strings.Split(trimmedStep, "-")
	if len(nodeParts) != 5 || len(stepParts) != 5 {
		return "", false
	}

	return nodeParts[0] + "-" + nodeParts[1] + "-" + stepParts[2] + "-" + stepParts[3] + "-" + stepParts[4], true
}
