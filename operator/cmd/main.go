/*
Copyright 2022.

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

package main

import (
	"context"
	"crypto/tls"
	"fmt"
	"os"
	"time"

	imagev1client "github.com/openshift/client-go/image/clientset/versioned/typed/image/v1"
	"github.com/spf13/pflag"
	apiextensionsv1 "k8s.io/apiextensions-apiserver/pkg/apis/apiextensions/v1"
	"k8s.io/apimachinery/pkg/api/errors"
	"k8s.io/apimachinery/pkg/types"
	"k8s.io/client-go/kubernetes"
	_ "k8s.io/client-go/plugin/pkg/client/auth"
	"k8s.io/client-go/rest"
	ctrl "sigs.k8s.io/controller-runtime"
	"sigs.k8s.io/controller-runtime/pkg/client"
	"sigs.k8s.io/controller-runtime/pkg/healthz"
	metricsserver "sigs.k8s.io/controller-runtime/pkg/metrics/server"
	"sigs.k8s.io/controller-runtime/pkg/webhook"

	"github.com/stolostron/multicluster-global-hub/operator/pkg/config"
	"github.com/stolostron/multicluster-global-hub/operator/pkg/controllers"
	globalhubwebhook "github.com/stolostron/multicluster-global-hub/operator/pkg/webhook"
	"github.com/stolostron/multicluster-global-hub/pkg/logger"
	"github.com/stolostron/multicluster-global-hub/pkg/utils"
)

var setupLog = logger.DefaultZapLogger()

const (
	webhookPort    = 9443
	webhookCertDir = "/webhook-certs"
)

func main() {
	if err := doMain(ctrl.SetupSignalHandler(), ctrl.GetConfigOrDie()); err != nil {
		setupLog.Error(err)
		os.Exit(1)
	}
}

func doMain(ctx context.Context, cfg *rest.Config) error {
	operatorConfig := parseFlags()
	utils.PrintRuntimeInfo()

	if operatorConfig.EnablePprof {
		go utils.StartDefaultPprofServer()
	}

	kubeClient, err := kubernetes.NewForConfig(cfg)
	if err != nil {
		return fmt.Errorf("failed to create the kubeclient: %w", err)
	}

	err = config.LoadControllerConfig(ctx, kubeClient)
	if err != nil {
		return fmt.Errorf("failed to load controller config: %w", err)
	}

	mgr, err := getManager(cfg, operatorConfig)
	if err != nil {
		return fmt.Errorf("unable to start manager: %w", err)
	}

	olmVersion, err := detectOLMVersion(ctx, mgr.GetAPIReader())
	if err != nil {
		return fmt.Errorf("failed to detect OLM version: %w", err)
	}
	operatorConfig.OLMVersion = olmVersion
	setupLog.Infof("detected OLM version: %q", operatorConfig.OLMVersion)

	imageClient, err := imagev1client.NewForConfig(cfg)
	if err != nil {
		return fmt.Errorf("failed to create openshift image client: %w", err)
	}

	if err := logger.AddLogConfigController(ctx, mgr); err != nil {
		return fmt.Errorf("failed to add the logLevel controller: %w", err)
	}

	err = controllers.NewMetaController(mgr, kubeClient, operatorConfig, imageClient).SetupWithManager(mgr)
	if err != nil {
		return fmt.Errorf("unable to create meta controller: %w", err)
	}

	if err := mgr.AddHealthzCheck("healthz", healthz.Ping); err != nil {
		return fmt.Errorf("unable to set up health check: %w", err)
	}
	if err := mgr.AddReadyzCheck("readyz", healthz.Ping); err != nil {
		return fmt.Errorf("unable to set up ready check: %w", err)
	}

	hookServer := mgr.GetWebhookServer()
	setupLog.Info("registering webhooks to the webhook server")
	hookServer.Register("/mutating", &webhook.Admission{
		Handler: globalhubwebhook.NewAdmissionHandler(mgr.GetClient(), mgr.GetScheme()),
	})

	setupLog.Info("starting manager")
	if err := mgr.Start(ctx); err != nil {
		return fmt.Errorf("failed to run the manager: %w", err)
	}

	return nil
}

func parseFlags() *config.OperatorConfig {
	config := &config.OperatorConfig{
		PodNamespace: utils.GetDefaultNamespace(),
	}

	pflag.StringVar(&config.MetricsAddress, "metrics-bind-address", ":8080",
		"The address the metric endpoint binds to.")
	pflag.StringVar(&config.ProbeAddress, "health-probe-bind-address", ":8081",
		"The address the probe endpoint binds to.")
	pflag.BoolVar(&config.LeaderElection, "leader-election", false,
		"Enable leader election for controller manager. ")
	pflag.BoolVar(&config.EnablePprof, "enable-pprof", false, "Enable the pprof tool.")
	pflag.IntVar(&config.TransportFailureThreshold, "transport-failure-threshold", 10,
		"Restart the pod if the transport error count exceeds the transport-failure-threshold within 5 minutes.")

	pflag.Parse()

	return config
}

func detectOLMVersion(ctx context.Context, r client.Reader) (string, error) {
	if os.Getenv("OPERATOR_CONDITION_NAME") != "" {
		return config.OLMVersionV0, nil
	}
	crd := &apiextensionsv1.CustomResourceDefinition{}
	err := r.Get(ctx, types.NamespacedName{Name: "clusterextensions.olm.operatorframework.io"}, crd)
	switch {
	case err == nil:
		return config.OLMVersionV1, nil
	case errors.IsNotFound(err):
		// OLMv1 CRD not found — fall back to checking for OLMv0 Subscription CRD
		err = r.Get(ctx, types.NamespacedName{Name: "subscriptions.operators.coreos.com"}, crd)
		if err == nil {
			return config.OLMVersionV0, nil
		} else if errors.IsNotFound(err) {
			return "", nil
		}
		return "", fmt.Errorf("failed to check for OLMv0 CRD: %w", err)
	default:
		return "", fmt.Errorf("failed to check for OLMv1 CRD: %w", err)
	}
}

func getManager(restConfig *rest.Config, operatorConfig *config.OperatorConfig) (ctrl.Manager, error) {
	electionConfig, err := config.GetElectionConfig()
	if err != nil {
		return nil, err
	}
	leaseDuration := time.Duration(electionConfig.LeaseDuration) * time.Second
	renewDeadline := time.Duration(electionConfig.RenewDeadline) * time.Second
	retryPeriod := time.Duration(electionConfig.RetryPeriod) * time.Second

	tlsCtx, tlsCancel := context.WithTimeout(context.Background(), 10*time.Second)
	defer tlsCancel()
	tlsConfigFunc, profileType, err := utils.BuildMetricsTLSConfigFunc(tlsCtx, restConfig)
	if err != nil {
		return nil, err
	}
	if profileType != "" {
		setupLog.Info("Configuring webhook server TLS from cluster APIServer profile", "profileType", profileType)
	} else {
		setupLog.Info("Using TLS 1.3 for webhook server (cluster APIServer profile unavailable)")
	}

	mgr, err := ctrl.NewManager(restConfig, ctrl.Options{
		Scheme: config.GetRuntimeScheme(),
		Metrics: metricsserver.Options{
			BindAddress: operatorConfig.MetricsAddress,
		},
		WebhookServer: &webhook.DefaultServer{
			Options: webhook.Options{
				Port:    webhookPort,
				TLSOpts: []func(*tls.Config){tlsConfigFunc},
			},
		},
		HealthProbeBindAddress:  operatorConfig.ProbeAddress,
		LeaderElection:          operatorConfig.LeaderElection,
		LeaderElectionID:        "multicluster-global-hub-operator-lock",
		LeaderElectionNamespace: operatorConfig.PodNamespace,
		LeaseDuration:           &leaseDuration,
		RenewDeadline:           &renewDeadline,
		RetryPeriod:             &retryPeriod,
		NewCache:                config.InitCache,
	})

	return mgr, err
}
