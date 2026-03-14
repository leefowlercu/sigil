package steps

import (
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"testing"
	"time"
)

func TestCleanupRunControlHelperDirsRemovesStaleDirs(t *testing.T) {
	t.Helper()

	moduleRoot := t.TempDir()
	staleA, err := os.MkdirTemp(moduleRoot, ".acceptance-run-stop-helper-")
	if err != nil {
		t.Fatalf("failed to create stale helper dir A: %v", err)
	}
	staleB, err := os.MkdirTemp(moduleRoot, ".acceptance-run-stop-helper-")
	if err != nil {
		t.Fatalf("failed to create stale helper dir B: %v", err)
	}
	unrelatedDir := filepath.Join(moduleRoot, "keep-me")
	if err := os.Mkdir(unrelatedDir, 0o755); err != nil {
		t.Fatalf("failed to create unrelated dir: %v", err)
	}

	if err := cleanupRunControlHelperDirs(moduleRoot, ""); err != nil {
		t.Fatalf("cleanupRunControlHelperDirs returned error: %v", err)
	}

	for _, dir := range []string{staleA, staleB} {
		if _, err := os.Stat(dir); !os.IsNotExist(err) {
			t.Fatalf("expected helper dir %q to be removed, got err=%v", dir, err)
		}
	}
	if _, err := os.Stat(unrelatedDir); err != nil {
		t.Fatalf("expected unrelated dir to remain, got err=%v", err)
	}
}

func TestCleanupScenarioResourcesRemovesHelperDirWhenHelperStopFails(t *testing.T) {
	if os.Getenv("SIGIL_TEST_HELPER_PROCESS") == "cleanup-stop-helper-exit-2" {
		os.Exit(2)
	}

	helperRoot := t.TempDir()
	helperDir := filepath.Join(helperRoot, ".acceptance-run-stop-helper-test")
	if err := os.Mkdir(helperDir, 0o755); err != nil {
		t.Fatalf("failed to create helper dir: %v", err)
	}
	if err := os.WriteFile(filepath.Join(helperDir, "run-stop-helper"), []byte("helper"), 0o755); err != nil {
		t.Fatalf("failed to seed helper binary: %v", err)
	}

	cmd := exec.Command(os.Args[0], "-test.run=TestCleanupScenarioResourcesRemovesHelperDirWhenHelperStopFails")
	cmd.Env = append(os.Environ(), "SIGIL_TEST_HELPER_PROCESS=cleanup-stop-helper-exit-2")
	if err := cmd.Start(); err != nil {
		t.Fatalf("failed to start helper process: %v", err)
	}
	time.Sleep(100 * time.Millisecond)

	world := &harnessWorld{
		helperDir: helperDir,
		helperCmd: cmd,
	}
	err := world.cleanupScenarioResources()
	if err == nil {
		t.Fatal("expected cleanupScenarioResources to surface helper stop error")
	}
	if !strings.Contains(err.Error(), "failed to stop helper process") {
		t.Fatalf("expected helper stop error, got %v", err)
	}
	if _, statErr := os.Stat(helperDir); !os.IsNotExist(statErr) {
		t.Fatalf("expected helper dir to be removed, got err=%v", statErr)
	}
}
