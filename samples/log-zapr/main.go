/*
Copyright 2019 The logr Authors.

Licensed under the Apache License, Version 2.0 (the "License");
you may not use this file except in compliance with the License.
You may obtain a copy of the License at

    http://www.apache.org/licenses/LICENSE-2.0

Unless required by applicable law or agreed to in writing, software
distributed under the License is distributed on an "AS IS" BASIS,
WITHOUT WARRANTIES OR CONDITIONS OF ANY KIND, either express or implied.
See the License for the specific language governing permissions and
limitations under the License.
*/

// This example shows how to instantiate a zap logger and what output
// looks like.
package main

import (
	"github.com/go-logr/logr"
	"github.com/go-logr/zapr"
	"go.uber.org/zap"
	"go.uber.org/zap/zapcore"
)

type e struct {
	str string
}

func (e e) Error() string {
	return e.str
}

func helper(log logr.Logger, msg string) {
	helper2(log, msg)
}

func helper2(log logr.Logger, msg string) {
	log.WithCallDepth(2).Info(msg)
}

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

	zc := config
	// zc := zap.NewProductionConfig()
	zc.Level = zap.NewAtomicLevelAt(zap.DebugLevel)
	zc.DisableStacktrace = true
	z, _ := zc.Build()

	log := zapr.NewLogger(z)
	// log = log.WithName("MyName")
	log.V(2).Info("02heloo2", "ni", "hao")
	log.V(1).Info("01heloo1", "ni", "hao")
	log.Info("00heloo", "ni", "hao")

	zc.Level.SetLevel(zap.InfoLevel) // -> 0

	log.V(2).Info("V2 message", "ni", "hao")
	log.V(1).Info("V1 message(Debug)", "ni", "hao")
	log.Info("Info message", "key", "val")
	// log.V(10).Info("-2heloo2", "ni", "hao")
	// log.V(0).Info("-11heloo1", "ni", "hao")
	// log.V(-1).Info("-11heloo1", "ni", "hao")
	// log.V(-2).Info("-11heloo1", "ni", "hao")
	// log.Error(nil, "-Ehello", "ee", "rr")

	// log.Debug

	// example(log.WithValues("module", "example"))
}

// // example only depends on logr except when explicitly breaking the
// // abstraction. Even that part is written so that it works with non-zap
// // loggers.
// func example(log logr.Logger) {
// 	log.Info("marshal", "stringer", v.String(), "raw", v)
// 	log.Info("hello", "val1", 1, "val2", map[string]int{"k": 1})
// 	log.V(1).Info("you should see this")
// 	log.V(1).V(1).Info("you should NOT see this")
// 	log.Error(nil, "uh oh", "trouble", true, "reasons", []float64{0.1, 0.11, 3.14})
// 	log.Error(e{"an error occurred"}, "goodbye", "code", -1)
// 	helper(log, "thru a helper")

// 	if zapLogger, ok := log.GetSink().(zapr.Underlier); ok {
// 		_ = zapLogger.GetUnderlying().Core().Sync()
// 	}
// }
