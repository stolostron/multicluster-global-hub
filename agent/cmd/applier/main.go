package main

import (
	"context"
	"fmt"
	"os"
	"runtime"

	"github.com/go-logr/logr"
	"github.com/operator-framework/operator-sdk/pkg/log/zap"
	"github.com/spf13/pflag"
	"k8s.io/client-go/discovery"
	"k8s.io/client-go/discovery/cached/memory"
	"k8s.io/client-go/dynamic"
	"k8s.io/client-go/restmapper"
	"k8s.io/client-go/tools/clientcmd"
	ctrl "sigs.k8s.io/controller-runtime"

	"github.com/stolostron/hub-of-hubs/agent/pkg/applier"
)

var (
	hubVersion string
	kubeconfig string
)

func init() {
	pflag.StringVar(&hubVersion, "hub-version", "", "ACM hub version for the current leaf hub.")
	pflag.StringVar(&kubeconfig, "kubeconfig", "", "kubeconfig for the connected cluster.")
	pflag.Parse()
}

func main() {
	os.Exit(doMain())
}

// function to handle defers with exit, see https://stackoverflow.com/a/27629493/553720.
func doMain() int {
	log := initLog()
	printVersion(log)

	cfg, err := clientcmd.BuildConfigFromFlags("", kubeconfig)
	if err != nil {
		log.Error(err, "failed to build config from kubeconfig")
		return 1
	}

	// create dscovery client
	dc, err := discovery.NewDiscoveryClientForConfig(cfg)
	if err != nil {
		log.Error(err, "failed to create discovery client")
		return 1
	}

	// create rest mapper
	mapper := restmapper.NewDeferredDiscoveryRESTMapper(memory.NewMemCacheClient(dc))

	// create dynamic client
	dyn, err := dynamic.NewForConfig(cfg)
	if err != nil {
		log.Error(err, "failed to create dynamic client")
		return 1
	}

	// check hubversion
	if hubVersion == "" {
		log.Error(err, "empty hub version!")
		return 1
	}

	// apply the manifests
	if err := applier.ApplyManifestsForVersion(context.TODO(), hubVersion, dyn, mapper, log); err != nil {
		log.Error(err, "failed to apply manifests")
		return 1
	}

	return 0
}

func initLog() logr.Logger {
	ctrl.SetLogger(zap.Logger())
	log := ctrl.Log.WithName("cmd")
	return log
}

func printVersion(log logr.Logger) {
	log.Info(fmt.Sprintf("Go Version: %s", runtime.Version()))
	log.Info(fmt.Sprintf("Go OS/Arch: %s/%s", runtime.GOOS, runtime.GOARCH))
}
