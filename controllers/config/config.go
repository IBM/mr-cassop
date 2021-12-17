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

// Config contains the Cassandra Operator configs
type Config struct {
	Namespace             string        `env:"NAMESPACE" envDefault:"default"`
	LeaderElectionEnabled bool          `env:"LEADERELECTION_ENABLED" envDefault:"true"`
	LogLevel              zapcore.Level `env:"LOGLEVEL" envDefault:"info"`
	LogFormat             string        `env:"LOGFORMAT" envDefault:"json"`
	WebhooksEnabled       bool          `env:"WEBHOOKS_ENABLED" envDefault:"true"`
	WebhooksPort          int32         `env:"WEBHOOKS_PORT" envDefault:"9443"`
	MetricsPort           int32         `env:"METRICS_PORT" envDefault:"8329"`
	RetryDelay            time.Duration `env:"RETRY_DELAY" envDefault:"10s"`
	DefaultCassandraImage string        `env:"DEFAULT_CASSANDRA_IMAGE,required"`
	DefaultProberImage    string        `env:"DEFAULT_PROBER_IMAGE,required"`
	DefaultJolokiaImage   string        `env:"DEFAULT_JOLOKIA_IMAGE,required"`
	DefaultReaperImage    string        `env:"DEFAULT_REAPER_IMAGE,required"`
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
