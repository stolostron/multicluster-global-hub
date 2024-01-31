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
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"

	mchv1 "github.com/stolostron/multiclusterhub-operator/api/v1"
	"k8s.io/apimachinery/pkg/api/errors"
	"k8s.io/apimachinery/pkg/api/meta"
	"k8s.io/client-go/kubernetes"
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
func Validate(filePath string) (string, bool) {
	if len(filePath) == 0 {
		return "", false
	}
	content, err := os.ReadFile(filePath) // #nosec G304
	if err != nil {
		log.Printf("failed to read file %s - %v", filePath, err)
		return "", false
	}
	trimmedContent := strings.TrimSpace(string(content))
	return trimmedContent, len(trimmedContent) > 0
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

func RestartPod(ctx context.Context, kubeClient kubernetes.Interface, podNamespace, deploymentName string) error {
	labelSelector := fmt.Sprintf("name=%s", deploymentName)
	poList, err := kubeClient.CoreV1().Pods(podNamespace).List(ctx, metav1.ListOptions{
		LabelSelector: labelSelector,
	})
	if err != nil {
		if errors.IsNotFound(err) {
			return nil
		}
		return err
	}
	for _, po := range poList.Items {
		err := kubeClient.CoreV1().Pods(podNamespace).Delete(ctx, po.Name, metav1.DeleteOptions{})
		if err != nil && !errors.IsNotFound(err) {
			return err
		}
	}
	return nil
}

func IsBackupEnabled(ctx context.Context, client client.Client) (bool, error) {
	mch, err := ListMCH(ctx, client)
	if err != nil {
		return false, err
	}
	if mch == nil {
		return false, nil
	}
	if mch.Spec.Overrides == nil {
		return false, nil
	}
	for _, c := range mch.Spec.Overrides.Components {
		if c.Name == "cluster-backup" && c.Enabled {
			return true, nil
		}
	}
	return false, nil
}
