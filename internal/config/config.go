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
	runEnvPrefix         = "SIGIL_RUN"
)

var (
	configMu     sync.RWMutex
	activeConfig Config
	configLoaded bool

	runConfigMu     sync.RWMutex
	activeRunConfig RunConfig
	runConfigLoaded bool

	validLogLevel = map[string]struct{}{
		"debug": {},
		"info":  {},
		"warn":  {},
		"error": {},
	}

	validRunGateways = map[string]struct{}{
		"openrouter": {},
	}

	validRunProviderModels = map[string]map[string]struct{}{
		"openai": {
			"gpt-5.1":           {},
			"gpt-5.1-codex-max": {},
			"gpt-5.2":           {},
			"gpt-5.2-pro":       {},
			"gpt-5.2-codex":     {},
			"gpt-5.3-codex":     {},
		},
		"anthropic": {
			"claude-sonnet-4":   {},
			"claude-opus-4":     {},
			"claude-sonnet-4.5": {},
			"claude-haiku-4.5":  {},
			"claude-opus-4.5":   {},
			"claude-sonnet-4.6": {},
			"claude-opus-4.6":   {},
		},
	}

	validRunReasoningEfforts = map[string]struct{}{
		"minimal": {},
		"low":     {},
		"medium":  {},
		"high":    {},
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

	path := resolveConfigPath(configPath, DefaultConfigPath)

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

	if err := readConfig(v, true); err != nil {
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

// InitRun initializes run configuration using the default run config source.
func InitRun() error {
	return initRunFromPath(DefaultRunConfigPath, true)
}

// InitRunFromPath initializes run configuration using the provided run config path.
func InitRunFromPath(runConfigPath string) error {
	return initRunFromPath(runConfigPath, false)
}

func initRunFromPath(runConfigPath string, allowMissing bool) (err error) {
	defer func() {
		if err != nil {
			clearActiveRunConfig()
		}
	}()

	path := resolveConfigPath(runConfigPath, DefaultRunConfigPath)

	configName, configDir, err := splitConfigPath(path)
	if err != nil {
		return fmt.Errorf("failed to resolve run config path; %w", err)
	}

	v := viper.New()
	v.SetConfigName(configName)
	v.SetConfigType("yaml")
	v.AddConfigPath(configDir)
	v.SetEnvPrefix(runEnvPrefix)
	v.SetEnvKeyReplacer(strings.NewReplacer(".", "_"))
	v.AutomaticEnv()

	registerRunDefaults(v)

	if err := readConfig(v, allowMissing); err != nil {
		return err
	}

	var cfg RunConfig
	if err := v.Unmarshal(&cfg); err != nil {
		return fmt.Errorf("failed to unmarshal run config; %w", err)
	}

	if err := validateRunConfig(cfg); err != nil {
		return fmt.Errorf("invalid run config; %w", err)
	}

	setActiveRunConfig(cfg)
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

// GetRun returns the active run config.
func GetRun() (RunConfig, error) {
	runConfigMu.RLock()
	defer runConfigMu.RUnlock()

	if !runConfigLoaded {
		return RunConfig{}, errors.New("run config is not initialized")
	}

	return activeRunConfig, nil
}

// MustGetRun returns the active run config and panics if run config was not initialized.
func MustGetRun() RunConfig {
	cfg, err := GetRun()
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

func setActiveRunConfig(cfg RunConfig) {
	runConfigMu.Lock()
	defer runConfigMu.Unlock()

	activeRunConfig = cfg
	runConfigLoaded = true
}

func clearActiveRunConfig() {
	runConfigMu.Lock()
	defer runConfigMu.Unlock()

	activeRunConfig = RunConfig{}
	runConfigLoaded = false
}

func resolveConfigPath(configPath string, fallback string) string {
	path := strings.TrimSpace(configPath)
	if path == "" {
		return fallback
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

func registerRunDefaults(v *viper.Viper) {
	defaults := NewDefaultRunConfig()
	v.SetDefault("system_prompt_append", defaults.SystemPromptAppend)
	v.SetDefault("prompt", defaults.Prompt)
	v.SetDefault("prompt_template", defaults.PromptTemplate)
	v.SetDefault("context", defaults.Context)
	v.SetDefault("context_template", defaults.ContextTemplate)
	v.SetDefault("llm.provider", defaults.LLM.Provider)
	v.SetDefault("llm.model", defaults.LLM.Model)
	v.SetDefault("llm.gateway", defaults.LLM.Gateway)
	v.SetDefault("llm.reasoning.enabled", defaults.LLM.Reasoning.Enabled)
	v.SetDefault("llm.reasoning.effort", defaults.LLM.Reasoning.Effort)
	v.SetDefault("llm.openrouter.base_url", defaults.LLM.OpenRouter.BaseURL)
	v.SetDefault("llm.openrouter.request_timeout_ms", defaults.LLM.OpenRouter.RequestTimeoutMS)
	v.SetDefault("llm.openrouter.api_key_env", defaults.LLM.OpenRouter.APIKeyEnv)
	v.SetDefault("rlm.enabled", defaults.RLM.Enabled)
	v.SetDefault("rlm.max_depth", defaults.RLM.MaxDepth)
}

func readConfig(v *viper.Viper, allowMissing bool) error {
	if err := v.ReadInConfig(); err != nil {
		var notFound viper.ConfigFileNotFoundError
		if allowMissing && errors.As(err, &notFound) {
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

func validateRunConfig(cfg RunConfig) error {
	if !exactlyOneStringSet(cfg.Prompt, cfg.PromptTemplate) {
		return errors.New("exactly one of prompt and prompt_template must be set")
	}

	if !exactlyOneStringSet(cfg.Context, cfg.ContextTemplate) {
		return errors.New("exactly one of context and context_template must be set")
	}

	provider := strings.TrimSpace(cfg.LLM.Provider)
	if provider == "" {
		return errors.New("llm.provider is required")
	}

	model := strings.TrimSpace(cfg.LLM.Model)
	if model == "" {
		return errors.New("llm.model is required")
	}

	if _, ok := validRunGateways[cfg.LLM.Gateway]; !ok {
		return fmt.Errorf("unsupported llm.gateway %q", cfg.LLM.Gateway)
	}

	allowedModels, providerOk := validRunProviderModels[provider]
	if !providerOk {
		return fmt.Errorf("unsupported llm.provider %q", provider)
	}

	if _, modelOk := allowedModels[model]; !modelOk {
		return fmt.Errorf("unsupported llm.model %q for llm.provider %q", model, provider)
	}

	if _, ok := validRunReasoningEfforts[cfg.LLM.Reasoning.Effort]; !ok {
		return fmt.Errorf("unsupported llm.reasoning.effort %q", cfg.LLM.Reasoning.Effort)
	}

	return nil
}

func exactlyOneStringSet(left string, right string) bool {
	leftSet := strings.TrimSpace(left) != ""
	rightSet := strings.TrimSpace(right) != ""

	return (leftSet || rightSet) && !(leftSet && rightSet)
}

func normalizeConfigPaths(cfg *Config) error {
	logDir, err := ExpandPath(cfg.LogDir)
	if err != nil {
		return fmt.Errorf("invalid log_dir; %w", err)
	}

	cfg.LogDir = logDir
	return nil
}
