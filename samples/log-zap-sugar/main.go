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

	log := logger.Sugar().Named("controller1")

	log.Debug("debug message", "world")
	log.Info("info message", "append info1", "append info2")
	log.Infow("info message", "key", "val")

	fmt.Println("=====================================")
	config.Level.SetLevel(zap.InfoLevel)

	log.Debug("debug message", "world")
	log.Info("info message", "append info1", "append info2")
	log.Infow("info message", "key", "val")
	log.Warn("warning info")

	sugar := logger.Sugar()
	config.Level.SetLevel(zap.WarnLevel)
	fmt.Println("=====================================")

	sugar.Debug("debug message", "world")
	sugar.Info("info message", "append info1", "append info2")
	sugar.Infow("info message", "key", "val")
	sugar.Warn("warning info")
	sugar.Error("error message")
	// sugar.Panic("panic message")
	sugar.Fatal("")
	fmt.Println("==============")

	// logger.Debug("Debug message with custom encoder - debug level")
	// logger.Info("Info message - debug level", zap.String("username", "john_doe"))
	// logger.Warn("Warning message - debug level")

	// logger.Debug("Debug message - info level")
	// logger.Info("Info message - info level")
	// logger.Warn("Warning message - info level")
}
