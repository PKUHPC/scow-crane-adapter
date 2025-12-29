package utils

type SslConfig struct {
	Enabled               bool   `yaml:"enabled"`
	CaCertPath            string `yaml:"caCertPath"`
	AdapterCertPath       string `yaml:"adapterCertPath"`
	AdapterPrivateKeyPath string `yaml:"adapterPrivateKeyPath"`
}

type MonitorConfig struct {
	Port int `yaml:"port"`
}

type Config struct {
	BindPort int           `mapstructure:"bind-port"`
	LogLevel string        `mapstructure:"log-level"`
	Ssl      SslConfig     `yaml:"ssl"`
	Monitor  MonitorConfig `yaml:"monitor"`
}
