package logger

import (
	"go.uber.org/zap"
	"go.uber.org/zap/zapcore"
)

func NewLogger(format string, level zapcore.Level) (*zap.SugaredLogger, error) {
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
		return nil, err
	}
	logger := zaplog.Sugar()

	return logger, nil
}
