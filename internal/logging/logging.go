package logging

import (
	"fmt"
	"log/slog"
	"os"
	"path/filepath"
	"sync"

	"github.com/leefowlercu/sigil/internal/config"
)

const logFileName = "sigil.log"

var (
	logMu          sync.RWMutex
	logSink        *os.File
	logFilePath    string
	logInitialized bool
	baseLogger     = slog.Default()
)

// DeriveLogFilePath returns the effective application log file path.
func DeriveLogFilePath(logDir string) (string, error) {
	expandedDir, err := config.ExpandPath(logDir)
	if err != nil {
		return "", fmt.Errorf("failed to resolve log_dir; %w", err)
	}

	return filepath.Join(expandedDir, logFileName), nil
}

// Init configures structured JSON file logging as the application default logger.
func Init(cfg config.Config) error {
	derivedPath, err := DeriveLogFilePath(cfg.LogDir)
	if err != nil {
		return fmt.Errorf("failed to derive log file path; %w", err)
	}

	level, err := resolveLogLevel(cfg.LogLevel)
	if err != nil {
		return fmt.Errorf("invalid log level; %w", err)
	}

	logDir := filepath.Dir(derivedPath)
	if err := os.MkdirAll(logDir, 0o755); err != nil {
		return fmt.Errorf("failed to create log directory %q; %w", logDir, err)
	}

	fileSink, err := os.OpenFile(derivedPath, os.O_APPEND|os.O_CREATE|os.O_WRONLY, 0o644)
	if err != nil {
		return fmt.Errorf("failed to open log file %q; %w", derivedPath, err)
	}

	handler := slog.NewJSONHandler(fileSink, &slog.HandlerOptions{
		Level: level,
	})
	logger := slog.New(handler)

	logMu.Lock()
	defer logMu.Unlock()

	if err := closeLocked(); err != nil {
		_ = fileSink.Close()
		return err
	}

	logSink = fileSink
	logFilePath = derivedPath
	logInitialized = true
	slog.SetDefault(logger)

	return nil
}

// ActiveLogFilePath returns the currently active log file path.
func ActiveLogFilePath() (string, error) {
	logMu.RLock()
	defer logMu.RUnlock()

	if !logInitialized {
		return "", fmt.Errorf("logging is not initialized")
	}

	return logFilePath, nil
}

// Close closes the active log sink and clears logging state.
func Close() error {
	logMu.Lock()
	defer logMu.Unlock()

	return closeLocked()
}

func closeLocked() error {
	if logSink == nil {
		logFilePath = ""
		logInitialized = false
		slog.SetDefault(baseLogger)
		return nil
	}

	if err := logSink.Close(); err != nil {
		return fmt.Errorf("failed to close active log sink; %w", err)
	}

	logSink = nil
	logFilePath = ""
	logInitialized = false
	slog.SetDefault(baseLogger)

	return nil
}

func resolveLogLevel(logLevel string) (slog.Level, error) {
	switch logLevel {
	case "debug":
		return slog.LevelDebug, nil
	case "info":
		return slog.LevelInfo, nil
	case "warn":
		return slog.LevelWarn, nil
	case "error":
		return slog.LevelError, nil
	default:
		return 0, fmt.Errorf("unsupported log_level %q", logLevel)
	}
}
