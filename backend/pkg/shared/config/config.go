package config

import (
	"fmt"

	"github.com/spf13/viper"
)

// Config holds shared configuration for backend services.
// Individual services can embed or extend this struct in their own packages.
type Config struct {
	AppName string `mapstructure:"appName"`

	Server struct {
		Addr string `mapstructure:"addr"`
	} `mapstructure:"server"`

	Database struct {
		URL string `mapstructure:"url"`
	} `mapstructure:"database"`

	Redis struct {
		URL string `mapstructure:"url"`
	} `mapstructure:"redis"`

	Auth struct {
		JWTSecret string `mapstructure:"jwtSecret"`
	} `mapstructure:"auth"`

	Logging struct {
		Level string `mapstructure:"level"`
	} `mapstructure:"logging"`

	// TODO: add Metrics and Tracing configuration sections.
}

// Load loads configuration from configs/{env}.yaml and environment variables.
func Load(serviceName string) (Config, error) {
	v := viper.New()

	env := viper.GetString("APP_ENV")
	if env == "" {
		env = "dev"
	}

	v.SetConfigName(env)
	v.SetConfigType("yaml")
	v.AddConfigPath("configs")

	v.SetEnvPrefix(serviceName)
	v.AutomaticEnv()

	if err := v.ReadInConfig(); err != nil {
		return Config{}, fmt.Errorf("read config: %w", err)
	}

	var cfg Config
	if err := v.Unmarshal(&cfg); err != nil {
		return Config{}, fmt.Errorf("unmarshal config: %w", err)
	}

	if cfg.Server.Addr == "" {
		cfg.Server.Addr = ":8080"
	}

	return cfg, nil
}

