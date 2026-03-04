package harness

import (
	"fmt"
	"os"
	"path/filepath"
	"regexp"
	"strings"

	"github.com/leefowlercu/sigil/internal/runtime"
)

var (
	runOutputContextRefPattern   = regexp.MustCompile(`^run-output://node/([^/]+)/context\.json$`)
	runOutputFinalRefPattern     = regexp.MustCompile(`^run-output://node/([^/]+)/final-answer\.json$`)
	runOutputTurnUserRefPattern  = regexp.MustCompile(`^run-output://node/([^/]+)/step/([^/]+)/turn-user\.json$`)
	runOutputTurnModelRefPattern = regexp.MustCompile(`^run-output://node/([^/]+)/step/([^/]+)/turn-model\.json$`)
)

func resolveFinalEvidenceRefs(runID string, runsBaseDir string, artifacts *ActionArtifactStore, evidence []FinalEvidence) error {
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
		if err := resolveEvidenceRef(runID, runsBaseDir, artifacts, item.Ref); err != nil {
			return fmt.Errorf("final.evidence[%d] ref resolution failed; %w", index, err)
		}
	}

	return nil
}

func resolveEvidenceRef(runID string, runsBaseDir string, artifacts *ActionArtifactStore, ref string) error {
	trimmed := strings.TrimSpace(ref)
	if trimmed == "" {
		return fmt.Errorf("ref is required")
	}

	if strings.HasPrefix(trimmed, runtime.ActionOutputRefPrefix) {
		if _, err := artifacts.Read(runID, trimmed); err != nil {
			return fmt.Errorf("failed to resolve run-artifact ref %q; %w", trimmed, err)
		}
		return nil
	}

	outputPath, err := resolveRunOutputPath(trimmed)
	if err != nil {
		return err
	}

	pathParts := append([]string{runsBaseDir, runID, "outputs"}, outputPath...)
	path := filepath.Join(pathParts...)
	info, err := os.Stat(path)
	if err != nil {
		return fmt.Errorf("failed to resolve run-output ref %q at %q; %w", trimmed, path, err)
	}
	if info.IsDir() {
		return fmt.Errorf("run-output ref %q resolved to directory %q", trimmed, path)
	}

	return nil
}

func resolveRunOutputPath(ref string) ([]string, error) {
	if matches := runOutputContextRefPattern.FindStringSubmatch(ref); len(matches) == 2 {
		return []string{"node", matches[1], "context.json"}, nil
	}
	if matches := runOutputFinalRefPattern.FindStringSubmatch(ref); len(matches) == 2 {
		return []string{"node", matches[1], "final-answer.json"}, nil
	}
	if matches := runOutputTurnUserRefPattern.FindStringSubmatch(ref); len(matches) == 3 {
		return []string{"node", matches[1], "step", matches[2], "turn-user.json"}, nil
	}
	if matches := runOutputTurnModelRefPattern.FindStringSubmatch(ref); len(matches) == 3 {
		return []string{"node", matches[1], "step", matches[2], "turn-model.json"}, nil
	}

	return nil, fmt.Errorf("unsupported run-output ref %q", ref)
}
