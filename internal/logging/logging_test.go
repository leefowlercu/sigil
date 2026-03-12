package logging

import (
	"encoding/json"
	"log/slog"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/leefowlercu/sigil/internal/config"
)

func TestDeriveLogFilePathResolvesFromWorkingDirectory(t *testing.T) {
	resetLogging(t)
	workDir := t.TempDir()
	chdirForLoggingTest(t, workDir)

	derivedPath, err := DeriveLogFilePath("./.sigil/logs")
	if err != nil {
		t.Fatalf("expected path derivation success, got %v", err)
	}

	expectedPath, err := config.ExpandPath("./.sigil/logs/sigil.log")
	if err != nil {
		t.Fatalf("expected expected-path expansion success, got %v", err)
	}
	if derivedPath != expectedPath {
		t.Fatalf("expected derived path %q, got %q", expectedPath, derivedPath)
	}
}

func TestInitCreatesLogFileAndSetsActivePath(t *testing.T) {
	resetLogging(t)
	workDir := t.TempDir()

	cfg := config.Config{
		LogLevel: "info",
		LogDir:   filepath.Join(workDir, "custom-logs"),
	}

	if err := Init(cfg); err != nil {
		t.Fatalf("expected logging init success, got %v", err)
	}

	activePath, err := ActiveLogFilePath()
	if err != nil {
		t.Fatalf("expected active log path, got %v", err)
	}

	expectedPath := filepath.Join(workDir, "custom-logs", "sigil.log")
	if activePath != expectedPath {
		t.Fatalf("expected active path %q, got %q", expectedPath, activePath)
	}

	info, err := os.Stat(activePath)
	if err != nil {
		t.Fatalf("expected log file to exist, got %v", err)
	}
	if !info.Mode().IsRegular() {
		t.Fatalf("expected log path to be a regular file, got mode %s", info.Mode())
	}
}

func TestInitWritesStructuredJSONLogRecords(t *testing.T) {
	resetLogging(t)
	workDir := t.TempDir()

	cfg := config.Config{
		LogLevel: "info",
		LogDir:   filepath.Join(workDir, "json-logs"),
	}
	if err := Init(cfg); err != nil {
		t.Fatalf("expected logging init success, got %v", err)
	}

	slog.Info("record for json validation", "run_id", "run-123")

	activePath, err := ActiveLogFilePath()
	if err != nil {
		t.Fatalf("expected active log path, got %v", err)
	}

	if err := Close(); err != nil {
		t.Fatalf("expected close success, got %v", err)
	}

	records := readLogJSONRecords(t, activePath)
	if len(records) == 0 {
		t.Fatal("expected at least one JSON log record")
	}

	found := false
	for _, record := range records {
		if record["msg"] != "record for json validation" {
			continue
		}
		found = true

		if record["level"] != "INFO" {
			t.Fatalf("expected level INFO, got %v", record["level"])
		}
		if record["run_id"] != "run-123" {
			t.Fatalf("expected run_id run-123, got %v", record["run_id"])
		}
		if _, ok := record["time"]; !ok {
			t.Fatalf("expected structured record to include time field, got %v", record)
		}
	}

	if !found {
		t.Fatal("expected to find emitted JSON record with target message")
	}
}

func TestInitFailsWhenDerivedPathCannotOpenFileSink(t *testing.T) {
	resetLogging(t)
	workDir := t.TempDir()
	blockedPath := filepath.Join(workDir, "blocked-target")
	if err := os.WriteFile(blockedPath, []byte("blocked"), 0o644); err != nil {
		t.Fatalf("failed to create blocked path fixture: %v", err)
	}

	cfg := config.Config{
		LogLevel: "info",
		LogDir:   blockedPath,
	}

	if err := Init(cfg); err == nil {
		t.Fatal("expected logging init error when log sink cannot be opened")
	}

	if _, err := ActiveLogFilePath(); err == nil {
		t.Fatal("expected no active log path after failed initialization")
	}
}

func TestInitReplacesExistingSinkAndPath(t *testing.T) {
	resetLogging(t)
	workDir := t.TempDir()

	firstCfg := config.Config{
		LogLevel: "info",
		LogDir:   filepath.Join(workDir, "logs-one"),
	}
	if err := Init(firstCfg); err != nil {
		t.Fatalf("expected first init success, got %v", err)
	}

	firstPath, err := ActiveLogFilePath()
	if err != nil {
		t.Fatalf("expected first active log path, got %v", err)
	}

	secondCfg := config.Config{
		LogLevel: "info",
		LogDir:   filepath.Join(workDir, "logs-two"),
	}
	if err := Init(secondCfg); err != nil {
		t.Fatalf("expected second init success, got %v", err)
	}

	secondPath, err := ActiveLogFilePath()
	if err != nil {
		t.Fatalf("expected second active log path, got %v", err)
	}

	if firstPath == secondPath {
		t.Fatalf("expected distinct paths across re-init, got %q", secondPath)
	}

	expectedSecondPath := filepath.Join(workDir, "logs-two", "sigil.log")
	if secondPath != expectedSecondPath {
		t.Fatalf("expected second path %q, got %q", expectedSecondPath, secondPath)
	}
}

func TestCloseSucceedsWhenNotInitialized(t *testing.T) {
	resetLogging(t)

	if err := Close(); err != nil {
		t.Fatalf("expected close success on uninitialized logger, got %v", err)
	}
}

func resetLogging(t *testing.T) {
	t.Helper()
	if err := Close(); err != nil {
		t.Fatalf("failed to reset logging before test: %v", err)
	}

	t.Cleanup(func() {
		_ = Close()
	})
}

func chdirForLoggingTest(t *testing.T, dir string) {
	t.Helper()

	originalDir, err := os.Getwd()
	if err != nil {
		t.Fatalf("failed to get current working directory: %v", err)
	}

	if err := os.Chdir(dir); err != nil {
		t.Fatalf("failed to change directory to %q: %v", dir, err)
	}

	t.Cleanup(func() {
		_ = os.Chdir(originalDir)
	})
}

func readLogJSONRecords(t *testing.T, path string) []map[string]any {
	t.Helper()

	content, err := os.ReadFile(path)
	if err != nil {
		t.Fatalf("failed to read log file %q: %v", path, err)
	}

	rawLines := strings.Split(string(content), "\n")
	records := make([]map[string]any, 0, len(rawLines))
	for _, rawLine := range rawLines {
		line := strings.TrimSpace(rawLine)
		if line == "" {
			continue
		}

		record := make(map[string]any)
		if err := json.Unmarshal([]byte(line), &record); err != nil {
			t.Fatalf("expected JSON log line, got parse error for %q: %v", line, err)
		}
		records = append(records, record)
	}

	return records
}
