package config

import (
	"fmt"
	"time"

	"go.uber.org/zap/zapcore"
)

type Config struct {
	ServerPort              int           `env:"SERVER_PORT" envDefault:"8888"`
	PodNamespace            string        `env:"POD_NAMESPACE,required"`
	AdminRoleSecretName     string        `env:"ADMIN_SECRET_NAME,required"`      // Active Admin Secret
	BaseAdminRoleSecretName string        `env:"BASE_ADMIN_SECRET_NAME,required"` // User's Admin Secret
	JmxPollingInterval      time.Duration `env:"JMX_POLLING_INTERVAL" envDefault:"10s"`
	JmxPort                 int           `env:"JMX_PORT" envDefault:"7199"`
	JolokiaPort             int           `env:"JOLOKIA_PORT" envDefault:"8080"`
	LogLevel                zapcore.Level `env:"LOGLEVEL" envDefault:"info"`
	LogFormat               string        `env:"LOGFORMAT" envDefault:"json"`
}

func LevelParser(v string) (interface{}, error) {
	var level zapcore.Level
	err := (&level).UnmarshalText([]byte(v))
	if err != nil {
		return nil, fmt.Errorf("%s is not an zapcore.Level", v)
	}
	return level, nil
}
