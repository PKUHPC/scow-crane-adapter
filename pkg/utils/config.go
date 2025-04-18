package utils

type SslConfig struct {
	Enabled               bool   `yaml:"enabled"`
	CaCertPath            string `yaml:"caCertPath"`
	AdapterCertPath       string `yaml:"adapterCertPath"`
	AdapterPrivateKeyPath string `yaml:"adapterPrivateKeyPath"`
}

type Config struct {
	BindPort   int       `mapstructure:"bind-port"`
	LogLevel   string    `mapstructure:"log-level"`
	ModulePath string    `mapstructure:"module-path"`
	Ssl        SslConfig `yaml:"ssl"`
}
