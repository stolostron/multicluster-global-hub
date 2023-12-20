package utils

import (
	"context"
	"fmt"
	"log"
	"os"
	"runtime"
	"strings"

	"github.com/go-logr/logr"
	"github.com/stolostron/multicluster-global-hub/pkg/constants"
	uberzap "go.uber.org/zap"
	uberzapcore "go.uber.org/zap/zapcore"

	mchv1 "github.com/stolostron/multiclusterhub-operator/api/v1"
	"k8s.io/apimachinery/pkg/api/errors"
	"k8s.io/apimachinery/pkg/api/meta"
	"sigs.k8s.io/controller-runtime/pkg/client"
	"sigs.k8s.io/controller-runtime/pkg/log/zap"
)

func PrintVersion(log logr.Logger) {
	log.Info(fmt.Sprintf("Go Version: %s", runtime.Version()))
	log.Info(fmt.Sprintf("Go OS/Arch: %s/%s", runtime.GOOS, runtime.GOARCH))
}

func CtrlZapOptions() zap.Options {
	encoderConfig := uberzap.NewProductionEncoderConfig()
	encoderConfig.EncodeTime = uberzapcore.ISO8601TimeEncoder
	encoderConfig.EncodeLevel = uberzapcore.CapitalColorLevelEncoder
	opts := zap.Options{
		Encoder: uberzapcore.NewConsoleEncoder(encoderConfig),
		// for development
		// ZapOpts: []uberzap.Option{
		// 	uberzap.AddCaller(),
		// },
	}
	return opts
}

// Validate return true if the file exists and the content is not empty
func Validate(filePath string) bool {
	if len(filePath) == 0 {
		return false
	}
	content, err := os.ReadFile(filePath) // #nosec G304
	if err != nil {
		log.Printf("failed to read file %s - %v", filePath, err)
		return false
	}
	trimmedContent := strings.TrimSpace(string(content))
	return len(trimmedContent) > 0
}

// GetDefaultNamespace returns default installation namespace
func GetDefaultNamespace() string {
	defaultNamespace, _ := os.LookupEnv("POD_NAMESPACE")
	if defaultNamespace == "" {
		defaultNamespace = constants.GHDefaultNamespace
	}
	return defaultNamespace
}

func ListMCH(ctx context.Context, k8sClient client.Client) (*mchv1.MultiClusterHub, error) {
	mch := &mchv1.MultiClusterHubList{}
	err := k8sClient.List(ctx, mch)
	if errors.IsNotFound(err) {
		return nil, nil
	}
	if meta.IsNoMatchError(err) {
		return nil, nil
	}
	if err != nil {
		return nil, err
	}

	if len(mch.Items) == 0 {
		return nil, err
	}

	return &mch.Items[0], nil
}
