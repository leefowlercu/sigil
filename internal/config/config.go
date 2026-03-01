package config

import (
	"errors"
	"fmt"
	"os"
	"path/filepath"
	"strings"
	"sync"

	"github.com/spf13/viper"
)

const (
	applicationEnvPrefix = "SIGIL"
)

var (
	configMu      sync.RWMutex
	activeConfig  Config
	configLoaded  bool
	validLogLevel = map[string]struct{}{
		"debug": {},
		"info":  {},
		"warn":  {},
		"error": {},
	}
)

// Init initializes configuration using the default config source.
func Init() error {
	return InitFromPath(DefaultConfigPath)
}

// InitFromPath initializes configuration using the provided config path.
func InitFromPath(configPath string) (err error) {
	defer func() {
		if err != nil {
			clearActiveConfig()
		}
	}()

	path := resolveConfigPath(configPath)

	configName, configDir, err := splitConfigPath(path)
	if err != nil {
		return fmt.Errorf("failed to resolve config path; %w", err)
	}

	v := viper.New()
	v.SetConfigName(configName)
	v.SetConfigType("yaml")
	v.AddConfigPath(configDir)
	v.SetEnvPrefix(applicationEnvPrefix)
	v.SetEnvKeyReplacer(strings.NewReplacer(".", "_"))
	v.AutomaticEnv()

	registerDefaults(v)

	if err := readConfig(v); err != nil {
		return err
	}

	var cfg Config
	if err := v.Unmarshal(&cfg); err != nil {
		return fmt.Errorf("failed to unmarshal config; %w", err)
	}

	if err := normalizeConfigPaths(&cfg); err != nil {
		return fmt.Errorf("invalid config; %w", err)
	}

	if err := validateConfig(cfg); err != nil {
		return fmt.Errorf("invalid config; %w", err)
	}

	setActiveConfig(cfg)
	return nil
}

// Get returns the active config.
func Get() (Config, error) {
	configMu.RLock()
	defer configMu.RUnlock()

	if !configLoaded {
		return Config{}, errors.New("config is not initialized")
	}

	return activeConfig, nil
}

// MustGet returns the active config and panics if config was not initialized.
func MustGet() Config {
	cfg, err := Get()
	if err != nil {
		panic(err)
	}

	return cfg
}

// ExpandPath resolves relative paths from the current working directory.
func ExpandPath(path string) (string, error) {
	clean := strings.TrimSpace(path)
	if clean == "" {
		return "", errors.New("path is empty")
	}

	if filepath.IsAbs(clean) {
		return filepath.Clean(clean), nil
	}

	cwd, err := os.Getwd()
	if err != nil {
		return "", fmt.Errorf("failed to resolve working directory; %w", err)
	}

	return filepath.Clean(filepath.Join(cwd, clean)), nil
}

func setActiveConfig(cfg Config) {
	configMu.Lock()
	defer configMu.Unlock()

	activeConfig = cfg
	configLoaded = true
}

func clearActiveConfig() {
	configMu.Lock()
	defer configMu.Unlock()

	activeConfig = Config{}
	configLoaded = false
}

func resolveConfigPath(configPath string) string {
	path := strings.TrimSpace(configPath)
	if path == "" {
		return DefaultConfigPath
	}

	return path
}

func splitConfigPath(configPath string) (string, string, error) {
	cleanPath := filepath.Clean(configPath)
	base := filepath.Base(cleanPath)
	ext := filepath.Ext(base)

	switch ext {
	case "", ".yaml", ".yml":
	default:
		return "", "", fmt.Errorf("config path %q must use a YAML extension", configPath)
	}

	configName := base
	if ext != "" {
		configName = strings.TrimSuffix(base, ext)
	}

	if configName == "" {
		return "", "", fmt.Errorf("config path %q has no file name", configPath)
	}

	configDir := filepath.Dir(cleanPath)
	if configDir == "" {
		configDir = "."
	}

	return configName, configDir, nil
}

func registerDefaults(v *viper.Viper) {
	defaults := NewDefaultConfig()
	v.SetDefault("log_level", defaults.LogLevel)
	v.SetDefault("log_dir", defaults.LogDir)
}

func readConfig(v *viper.Viper) error {
	if err := v.ReadInConfig(); err != nil {
		var notFound viper.ConfigFileNotFoundError
		if errors.As(err, &notFound) {
			return nil
		}

		return fmt.Errorf("failed to read config; %w", err)
	}

	return nil
}

func validateConfig(cfg Config) error {
	if _, ok := validLogLevel[cfg.LogLevel]; !ok {
		return fmt.Errorf("unsupported log_level %q", cfg.LogLevel)
	}

	return nil
}

func normalizeConfigPaths(cfg *Config) error {
	logDir, err := ExpandPath(cfg.LogDir)
	if err != nil {
		return fmt.Errorf("invalid log_dir; %w", err)
	}

	cfg.LogDir = logDir
	return nil
}
