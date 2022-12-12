package logger

import (
	"log"

	"go.uber.org/zap/zapcore"

	"go.uber.org/zap"
)

const (
	FieldOperatorVersion = "operator_version"
)

func NewLogger(format string, level zapcore.Level) *zap.SugaredLogger {
	var loggerConfig zap.Config

	if format == "json" {
		loggerConfig = zap.NewProductionConfig()
	} else {
		loggerConfig = zap.NewDevelopmentConfig()
	}
	loggerConfig.DisableStacktrace = true
	loggerConfig.Level = zap.NewAtomicLevelAt(level)
	loggerConfig.OutputPaths = []string{"stdout"}
	loggerConfig.EncoderConfig.EncodeTime = zapcore.ISO8601TimeEncoder

	zaplog, err := loggerConfig.Build()
	if err != nil {
		log.Panicf("Could not initialize zap logger: %v", err)
	}
	logger := zaplog.Sugar()

	return logger
}
