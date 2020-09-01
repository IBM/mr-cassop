package logger

import (
	"context"
	"log"

	"go.uber.org/zap/zapcore"

	"go.uber.org/zap"
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

	zaplog, err := loggerConfig.Build()
	if err != nil {
		log.Panicf("Could not initialize zap logger: %v", err)
	}
	logger := zaplog.Sugar()

	return logger
}

type contextKey int

const loggerCtxKey contextKey = iota

func ToContext(ctx context.Context, l *zap.SugaredLogger) context.Context {
	return context.WithValue(ctx, loggerCtxKey, l)
}

func FromContext(ctx context.Context) *zap.SugaredLogger {
	ctxValue := ctx.Value(loggerCtxKey)
	if ctxValue == nil {
		return zap.NewNop().Sugar()
	}

	logr, ok := ctxValue.(*zap.SugaredLogger)
	if !ok {
		return zap.NewNop().Sugar()
	}

	return logr
}
