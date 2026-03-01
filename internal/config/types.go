package config

// Config is the typed application configuration contract for sigil.
type Config struct {
	LogLevel string `yaml:"log_level" mapstructure:"log_level"`
	LogDir   string `yaml:"log_dir" mapstructure:"log_dir"`
}
