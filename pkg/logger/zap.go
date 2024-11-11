package logger

import (
	"github.com/go-logr/logr"
	"github.com/go-logr/zapr"
	"go.uber.org/zap"
	"go.uber.org/zap/zapcore"
	ctrl "sigs.k8s.io/controller-runtime"
)

// keep the logger in a singleton mode, allowing dynamic changes to the log level at runtime
var (
	internalZapConfig *zap.Config
	internalZapLogger *zap.Logger
)

// SetZapConfig expose the whole configuration for the zap Logger
func SetZapConfig(config *zap.Config) {
	internalZapConfig = config
}

func SetZapLogLevel(level zapcore.Level) {
	if internalZapConfig == nil {
		SetZapConfig(GetDefaultZapConfig())
	}
	internalZapConfig.Level.SetLevel(level)
}

func CoreZapLogger() *zap.Logger {
	if internalZapConfig == nil {
		SetZapLogLevel(zap.InfoLevel)
	}
	if internalZapLogger == nil {
		internalZapLogger, _ = internalZapConfig.Build()
	}
	// set the controller-runtime logger with the current zap logger
	ctrl.SetLogger(zapr.NewLogger(internalZapLogger))
	return internalZapLogger
}

func ZaprLogger() logr.Logger {
	_ = CoreZapLogger()
	return zapr.NewLogger(internalZapLogger)
}

// DefaultZapLogger is a sugar wraps the Logger to provide a more ergonomic, but slightly slower, API
func DefaultZapLogger() *zap.SugaredLogger {
	logger := CoreZapLogger()
	sugar := logger.Sugar()
	return sugar
}

// ZapLogger is a sugar wraps the Logger to provide a more ergonomic, but slightly slower, API
func ZapLogger(name string) *zap.SugaredLogger {
	logger := CoreZapLogger()
	sugar := logger.Sugar().Named(name)
	return sugar
}

func GetDefaultZapConfig() *zap.Config {
	encoderConfig := zapcore.EncoderConfig{
		TimeKey:        "timestamp",
		LevelKey:       "level",
		NameKey:        "logger",
		CallerKey:      "caller",
		MessageKey:     "message",
		StacktraceKey:  "stacktrace",
		LineEnding:     zapcore.DefaultLineEnding,
		EncodeLevel:    zapcore.CapitalLevelEncoder,
		EncodeTime:     zapcore.ISO8601TimeEncoder,
		EncodeDuration: zapcore.StringDurationEncoder,
		EncodeCaller:   zapcore.ShortCallerEncoder,
	}

	config := &zap.Config{
		Level:            zap.NewAtomicLevelAt(zap.InfoLevel),
		Development:      true,
		Encoding:         "console",
		EncoderConfig:    encoderConfig,
		OutputPaths:      []string{"stdout"},
		ErrorOutputPaths: []string{"stderr"},
	}
	return config
}
