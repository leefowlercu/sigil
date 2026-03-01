package config

import (
	"os"
	"path/filepath"
	"testing"
)

func TestNewDefaultConfig(t *testing.T) {
	defaults := NewDefaultConfig()
	if defaults.LogLevel != DefaultLogLevel {
		t.Fatalf("expected default log level %q, got %q", DefaultLogLevel, defaults.LogLevel)
	}

	if defaults.LogDir != DefaultLogDir {
		t.Fatalf("expected default log dir %q, got %q", DefaultLogDir, defaults.LogDir)
	}
}

func TestInitUsesDefaultsWhenConfigFileIsMissing(t *testing.T) {
	clearActiveConfig()
	unsetEnv(t, "SIGIL_LOG_LEVEL", "SIGIL_LOG_DIR")
	chdir(t, t.TempDir())

	if err := Init(); err != nil {
		t.Fatalf("expected defaults-only init success, got %v", err)
	}

	cfg, err := Get()
	if err != nil {
		t.Fatalf("expected active config, got %v", err)
	}

	if cfg.LogLevel != DefaultLogLevel {
		t.Fatalf("expected log level %q, got %q", DefaultLogLevel, cfg.LogLevel)
	}

	expectedLogDir := mustExpandPath(t, DefaultLogDir)
	if cfg.LogDir != expectedLogDir {
		t.Fatalf("expected log dir %q, got %q", expectedLogDir, cfg.LogDir)
	}
}

func TestInitAppliesEnvironmentOverrides(t *testing.T) {
	clearActiveConfig()
	unsetEnv(t, "SIGIL_LOG_LEVEL", "SIGIL_LOG_DIR")
	chdir(t, t.TempDir())
	t.Setenv("SIGIL_LOG_LEVEL", "warn")
	t.Setenv("SIGIL_LOG_DIR", "./env-logs")

	if err := Init(); err != nil {
		t.Fatalf("expected init success, got %v", err)
	}

	cfg := MustGet()
	if cfg.LogLevel != "warn" {
		t.Fatalf("expected log level override warn, got %q", cfg.LogLevel)
	}

	expectedLogDir := mustExpandPath(t, "./env-logs")
	if cfg.LogDir != expectedLogDir {
		t.Fatalf("expected log dir override %q, got %q", expectedLogDir, cfg.LogDir)
	}
}

func TestInitFromPathUsesEnvironmentPrecedenceOverFile(t *testing.T) {
	clearActiveConfig()
	unsetEnv(t, "SIGIL_LOG_LEVEL", "SIGIL_LOG_DIR")
	workDir := t.TempDir()
	chdir(t, workDir)

	configPath := filepath.Join(workDir, "sigil.yaml")
	if err := os.WriteFile(configPath, []byte("log_level: debug\nlog_dir: ./file-logs\n"), 0o644); err != nil {
		t.Fatalf("failed to write config file: %v", err)
	}

	t.Setenv("SIGIL_LOG_DIR", "./env-logs")

	if err := InitFromPath(configPath); err != nil {
		t.Fatalf("expected init success, got %v", err)
	}

	cfg := MustGet()
	if cfg.LogLevel != "debug" {
		t.Fatalf("expected file log level debug, got %q", cfg.LogLevel)
	}

	expectedLogDir := mustExpandPath(t, "./env-logs")
	if cfg.LogDir != expectedLogDir {
		t.Fatalf("expected env log dir override %q, got %q", expectedLogDir, cfg.LogDir)
	}
}

func TestInitResolvesRelativeLogDirFromWorkingDirectory(t *testing.T) {
	clearActiveConfig()
	unsetEnv(t, "SIGIL_LOG_LEVEL", "SIGIL_LOG_DIR")
	workDir := t.TempDir()
	chdir(t, workDir)

	if err := Init(); err != nil {
		t.Fatalf("expected init success, got %v", err)
	}

	cfg := MustGet()
	expected := mustExpandPath(t, DefaultLogDir)
	if cfg.LogDir != expected {
		t.Fatalf("expected resolved log dir %q, got %q", expected, cfg.LogDir)
	}
}

func TestInitRejectsUnsupportedLogLevel(t *testing.T) {
	clearActiveConfig()
	unsetEnv(t, "SIGIL_LOG_LEVEL", "SIGIL_LOG_DIR")
	workDir := t.TempDir()
	chdir(t, workDir)

	configPath := filepath.Join(workDir, "sigil.yaml")
	if err := os.WriteFile(configPath, []byte("log_level: trace\n"), 0o644); err != nil {
		t.Fatalf("failed to write config file: %v", err)
	}

	if err := InitFromPath(configPath); err == nil {
		t.Fatal("expected unsupported log_level validation error")
	}
}

func TestInitFromPathClearsActiveConfigOnValidationFailure(t *testing.T) {
	clearActiveConfig()
	unsetEnv(t, "SIGIL_LOG_LEVEL", "SIGIL_LOG_DIR")
	workDir := t.TempDir()
	chdir(t, workDir)

	validPath := filepath.Join(workDir, "valid.yaml")
	if err := os.WriteFile(validPath, []byte("log_level: info\nlog_dir: ./valid-logs\n"), 0o644); err != nil {
		t.Fatalf("failed to write valid config file: %v", err)
	}

	if err := InitFromPath(validPath); err != nil {
		t.Fatalf("expected initial init success, got %v", err)
	}

	invalidPath := filepath.Join(workDir, "invalid.yaml")
	if err := os.WriteFile(invalidPath, []byte("log_level: trace\n"), 0o644); err != nil {
		t.Fatalf("failed to write invalid config file: %v", err)
	}

	if err := InitFromPath(invalidPath); err == nil {
		t.Fatal("expected validation failure for invalid config")
	}

	if _, err := Get(); err == nil {
		t.Fatal("expected config to be cleared after failed initialization")
	}
}

func TestExpandPathResolvesRelativePathsFromWorkingDirectory(t *testing.T) {
	workDir := t.TempDir()
	chdir(t, workDir)

	expanded, err := ExpandPath("./sigil/logs")
	if err != nil {
		t.Fatalf("expected expansion success, got %v", err)
	}

	currentDir, err := os.Getwd()
	if err != nil {
		t.Fatalf("failed to get working directory: %v", err)
	}
	expected := filepath.Clean(filepath.Join(currentDir, "./sigil/logs"))
	if expanded != expected {
		t.Fatalf("expected expanded path %q, got %q", expected, expanded)
	}
}

func TestExpandPathReturnsAbsolutePathAsIs(t *testing.T) {
	absolutePath := filepath.Join(t.TempDir(), "logs")

	expanded, err := ExpandPath(absolutePath)
	if err != nil {
		t.Fatalf("expected expansion success, got %v", err)
	}

	if expanded != filepath.Clean(absolutePath) {
		t.Fatalf("expected absolute path %q, got %q", absolutePath, expanded)
	}
}

func chdir(t *testing.T, dir string) {
	t.Helper()

	originalDir, err := os.Getwd()
	if err != nil {
		t.Fatalf("failed to get working directory: %v", err)
	}

	if err := os.Chdir(dir); err != nil {
		t.Fatalf("failed to change working directory: %v", err)
	}

	t.Cleanup(func() {
		_ = os.Chdir(originalDir)
	})
}

func unsetEnv(t *testing.T, keys ...string) {
	t.Helper()

	originalValues := make(map[string]*string, len(keys))
	for _, key := range keys {
		if value, ok := os.LookupEnv(key); ok {
			copied := value
			originalValues[key] = &copied
		} else {
			originalValues[key] = nil
		}

		if err := os.Unsetenv(key); err != nil {
			t.Fatalf("failed to unset env %s: %v", key, err)
		}
	}

	t.Cleanup(func() {
		for key, value := range originalValues {
			if value == nil {
				_ = os.Unsetenv(key)
				continue
			}

			_ = os.Setenv(key, *value)
		}
	})
}

func mustExpandPath(t *testing.T, path string) string {
	t.Helper()

	expanded, err := ExpandPath(path)
	if err != nil {
		t.Fatalf("failed to expand %q: %v", path, err)
	}

	return expanded
}
