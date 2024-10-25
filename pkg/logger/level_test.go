package logger

import (
	"testing"

	"go.uber.org/zap/zapcore"
)

func resetInternalZapConfig() {
	internalZapConfig = GetDefaultZapConfig()
}

func TestSetLogLevel(t *testing.T) {
	tests := []struct {
		level         LogLevel
		expectedLevel zapcore.Level
	}{
		{Info, zapcore.InfoLevel},
		{Debug, zapcore.DebugLevel},
		{Warn, zapcore.WarnLevel},
		{Error, zapcore.ErrorLevel},
	}

	for _, tt := range tests {
		t.Run(string(tt.level), func(t *testing.T) {
			resetInternalZapConfig()
			SetLogLevel(tt.level)

			if got := internalZapConfig.Level.Level(); got != tt.expectedLevel {
				t.Errorf("SetLogLevel(%s): expected %v, got %v", tt.level, tt.expectedLevel, got)
			}
		})
	}

	t.Run("UnknownLogLevel", func(t *testing.T) {
		resetInternalZapConfig()
		unknownLevel := LogLevel("unknown")
		SetLogLevel(unknownLevel)

		if got := internalZapConfig.Level.Level(); got != zapcore.InfoLevel {
			t.Errorf("SetLogLevel(%s): expected default level %v, got %v", unknownLevel, zapcore.InfoLevel, got)
		}
	})
}
