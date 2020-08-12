package config

import (
	"github.com/caarlos0/env/v6"
	"github.com/pkg/errors"
	"go.uber.org/zap/zapcore"
)

// Config contains the Varnish Operator configs
type Config struct {
	Namespace             string        `env:"NAMESPACE" envDefault:"default"`
	LeaderElectionEnabled bool          `env:"LEADERELECTION_ENABLED" envDefault:"true"`
	LogLevel              zapcore.Level `env:"LOGLEVEL" envDefault:"info"`
	LogFormat             string        `env:"LOGFORMAT" envDefault:"json"`
	WebhooksEnabled       bool          `env:"WEBHOOKS_ENABLED" envDefault:"true"`
}

func LoadConfig() (*Config, error) {
	c := Config{}
	if err := env.Parse(&c); err != nil {
		return &c, errors.WithStack(err)
	}

	return &c, nil
}
