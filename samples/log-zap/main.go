package main

import (
	"fmt"

	"go.uber.org/zap"
	"go.uber.org/zap/zapcore"
)

func main() {
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

	config := zap.Config{
		Level:            zap.NewAtomicLevelAt(zap.DebugLevel),
		Development:      true,
		Encoding:         "console",
		EncoderConfig:    encoderConfig,
		OutputPaths:      []string{"stdout"},
		ErrorOutputPaths: []string{"stderr"},
	}

	logger, err := config.Build()
	if err != nil {
		panic(err)
	}
	defer logger.Sync()

	logger.Debug("Debug message with custom encoder - debug level")
	logger.Info("Info message - debug level", zap.String("username", "john_doe"))
	logger.Warn("Warning message - debug level")

	fmt.Println("=============================")

	logger, err = config.Build()
	if err != nil {
		panic(err)
	}
	defer logger.Sync()
	config.Level.SetLevel(zap.InfoLevel)

	logger.Debug("Debug message - info level")
	logger.Info("Info message - info level")
	logger.Warn("Warning message - info level")
	logger.Sugar().Panic("failed message", fmt.Errorf("not found"))
	fmt.Println("=============")
}
