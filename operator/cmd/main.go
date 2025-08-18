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
	"k8s.io/client-go/kubernetes"
	_ "k8s.io/client-go/plugin/pkg/client/auth"
	"k8s.io/client-go/rest"
	ctrl "sigs.k8s.io/controller-runtime"
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

func getManager(restConfig *rest.Config, operatorConfig *config.OperatorConfig) (ctrl.Manager, error) {
	electionConfig, err := config.GetElectionConfig()
	if err != nil {
		return nil, err
	}
	leaseDuration := time.Duration(electionConfig.LeaseDuration) * time.Second
	renewDeadline := time.Duration(electionConfig.RenewDeadline) * time.Second
	retryPeriod := time.Duration(electionConfig.RetryPeriod) * time.Second

	mgr, err := ctrl.NewManager(restConfig, ctrl.Options{
		Scheme: config.GetRuntimeScheme(),
		Metrics: metricsserver.Options{
			BindAddress: operatorConfig.MetricsAddress,
		},
		WebhookServer: &webhook.DefaultServer{
			Options: webhook.Options{
				Port: webhookPort,
				TLSOpts: []func(*tls.Config){
					func(config *tls.Config) {
						config.MinVersion = tls.VersionTLS13
					},
				},
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
