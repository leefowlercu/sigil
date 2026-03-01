package config

const (
	// DefaultConfigPath is the default application config source.
	DefaultConfigPath = "./sigil.yaml"

	// DefaultLogLevel is the baseline log level.
	DefaultLogLevel = "info"

	// DefaultLogDir is the baseline log directory.
	DefaultLogDir = "./sigil/logs"
)

// NewDefaultConfig returns a fully populated baseline Config.
func NewDefaultConfig() Config {
	return Config{
		LogLevel: DefaultLogLevel,
		LogDir:   DefaultLogDir,
	}
}
