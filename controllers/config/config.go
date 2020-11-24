package config

import (
	"github.com/caarlos0/env/v6"
	"github.com/pkg/errors"
	"go.uber.org/zap/zapcore"
	"reflect"
	"time"
)

var (
	levelType    = reflect.TypeOf(zapcore.DebugLevel)
	parseFuncMap = map[reflect.Type]env.ParserFunc{
		levelType: levelParser,
	}
)

// Config contains the Varnish Operator configs
type Config struct {
	Namespace             string        `env:"NAMESPACE" envDefault:"default"`
	LeaderElectionEnabled bool          `env:"LEADERELECTION_ENABLED" envDefault:"true"`
	LogLevel              zapcore.Level `env:"LOGLEVEL" envDefault:"info"`
	LogFormat             string        `env:"LOGFORMAT" envDefault:"json"`
	WebhooksEnabled       bool          `env:"WEBHOOKS_ENABLED" envDefault:"true"`
	MetricsPort           int32         `env:"METRICS_PORT" envDefault:"8329"`
	RetryDelay            time.Duration `env:"RETRY_DELAY" envDefault:"10s"`
}

func LoadConfig() (*Config, error) {
	c := Config{}
	if err := env.ParseWithFuncs(&c, parseFuncMap); err != nil {
		return &c, errors.WithStack(err)
	}

	return &c, nil
}

func levelParser(v string) (interface{}, error) {
	var level zapcore.Level
	err := (&level).UnmarshalText([]byte(v))
	if err != nil {
		return nil, errors.Errorf("%s is not an zapcore.Level", v)
	}
	return level, nil
}
