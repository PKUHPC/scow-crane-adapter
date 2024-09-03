package utils

type Config struct {
	BindPort int    `mapstructure:"bind-port"`
	LogLevel string `mapstructure:"log-level"`
}
