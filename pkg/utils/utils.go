package utils

import (
	"fmt"
	"runtime"
	"time"

	"github.com/go-logr/logr"
	"go.uber.org/zap/zapcore"
	"sigs.k8s.io/controller-runtime/pkg/log/zap"
)

func PrintVersion(log logr.Logger) {
	log.Info(fmt.Sprintf("Go Version: %s", runtime.Version()))
	log.Info(fmt.Sprintf("Go OS/Arch: %s/%s", runtime.GOOS, runtime.GOARCH))
}

func CtrlZapOptions() zap.Options {
	opts := zap.Options{
		Development: true,
		TimeEncoder: func(t time.Time, enc zapcore.PrimitiveArrayEncoder) {
			enc.AppendString(t.Format("2006-01-02 15:04:05 MST"))
		},
	}
	return opts
}
