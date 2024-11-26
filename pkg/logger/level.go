package logger

import (
	"go.uber.org/zap/zapcore"
)

type LogLevel string

const (
	Info  LogLevel = "info"
	Debug LogLevel = "debug"
	Warn  LogLevel = "warn"
	Error LogLevel = "error"
)

var logLevel = Info

func SetLogLevel(level LogLevel) {
	if internalZapConfig == nil {
		internalZapConfig = GetDefaultZapConfig()
	}
	log := DefaultZapLogger()

	switch level {
	case Info:
		internalZapConfig.Level.SetLevel(zapcore.InfoLevel)
	case Debug:
		internalZapConfig.Level.SetLevel(zapcore.DebugLevel)
	case Warn:
		internalZapConfig.Level.SetLevel(zapcore.WarnLevel)
	case Error:
		internalZapConfig.Level.SetLevel(zapcore.ErrorLevel)
	default:
		DefaultZapLogger().Warnf("set unknown logLevel: %s", level)
	}
	logLevel = level
	log.Infof("set the logLevel: %s", string(level))
}

func GetLogLevel() LogLevel {
	return logLevel
}
