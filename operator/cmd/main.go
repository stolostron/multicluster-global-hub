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
	"flag"
	"os"
	"time"

	imagev1client "github.com/openshift/client-go/image/clientset/versioned/typed/image/v1"
	"github.com/spf13/pflag"
	"k8s.io/client-go/kubernetes"
	_ "k8s.io/client-go/plugin/pkg/client/auth"
	"k8s.io/client-go/rest"
	ctrl "sigs.k8s.io/controller-runtime"
	"sigs.k8s.io/controller-runtime/pkg/healthz"
	"sigs.k8s.io/controller-runtime/pkg/log/zap"
	metricsserver "sigs.k8s.io/controller-runtime/pkg/metrics/server"
	"sigs.k8s.io/controller-runtime/pkg/webhook"

	"github.com/stolostron/multicluster-global-hub/operator/pkg/config"
	"github.com/stolostron/multicluster-global-hub/operator/pkg/controllers"
	"github.com/stolostron/multicluster-global-hub/operator/pkg/controllers/crd"
	"github.com/stolostron/multicluster-global-hub/operator/pkg/controllers/hubofhubs"
	globalhubwebhook "github.com/stolostron/multicluster-global-hub/operator/pkg/webhook"
	"github.com/stolostron/multicluster-global-hub/pkg/utils"
)

var setupLog = ctrl.Log.WithName("setup")

const (
	webhookPort    = 9443
	webhookCertDir = "/webhook-certs"
)

func main() {
	os.Exit(doMain(ctrl.SetupSignalHandler(), ctrl.GetConfigOrDie()))
}

func doMain(ctx context.Context, cfg *rest.Config) int {
	operatorConfig := parseFlags()
	utils.PrintVersion(setupLog)

	if operatorConfig.EnablePprof {
		go utils.StartDefaultPprofServer()
	}

	kubeClient, err := kubernetes.NewForConfig(cfg)
	if err != nil {
		setupLog.Error(err, "failed to create kube client")
		return 1
	}

	err = config.LoadControllerConfig(ctx, kubeClient)
	if err != nil {
		setupLog.Error(err, "failed to load controller config")
		return 1
	}

	mgr, err := getManager(cfg, operatorConfig)
	if err != nil {
		setupLog.Error(err, "unable to start manager")
		return 1
	}

	imageClient, err := imagev1client.NewForConfig(cfg)
	if err != nil {
		setupLog.Error(err, "failed to create openshift image client")
		return 1
	}

	err = controllers.NewMetaController(mgr, kubeClient, operatorConfig, imageClient).SetupWithManager(mgr)
	if err != nil {
		setupLog.Error(err, "unable to create meta controller")
		return 1
	}

	// global hub controller
	globalHubController, err := hubofhubs.NewGlobalHubController(mgr, kubeClient, operatorConfig, imageClient)
	if err != nil {
		setupLog.Error(err, "unable to create crd controller")
		return 1
	}

	_, err = crd.AddCRDController(mgr, operatorConfig, kubeClient, globalHubController)
	if err != nil {
		setupLog.Error(err, "unable to create crd controller")
		return 1
	}

	if err := mgr.AddHealthzCheck("healthz", healthz.Ping); err != nil {
		setupLog.Error(err, "unable to set up health check")
		return 1
	}
	if err := mgr.AddReadyzCheck("readyz", healthz.Ping); err != nil {
		setupLog.Error(err, "unable to set up ready check")
		return 1
	}

	hookServer := mgr.GetWebhookServer()
	setupLog.Info("registering webhooks to the webhook server")
	hookServer.Register("/mutating", &webhook.Admission{
		Handler: globalhubwebhook.NewAdmissionHandler(mgr.GetClient(), mgr.GetScheme()),
	})

	setupLog.Info("starting manager")
	if err := mgr.Start(ctx); err != nil {
		setupLog.Error(err, "problem running manager")
		return 1
	}

	return 0
}

func parseFlags() *config.OperatorConfig {
	config := &config.OperatorConfig{
		PodNamespace: utils.GetDefaultNamespace(),
	}

	// add zap flags
	opts := utils.CtrlZapOptions()
	defaultFlags := flag.CommandLine
	opts.BindFlags(defaultFlags)
	pflag.CommandLine.AddGoFlagSet(defaultFlags)
	pflag.StringVar(&config.MetricsAddress, "metrics-bind-address", ":8080",
		"The address the metric endpoint binds to.")
	pflag.StringVar(&config.ProbeAddress, "health-probe-bind-address", ":8081",
		"The address the probe endpoint binds to.")
	pflag.BoolVar(&config.LeaderElection, "leader-election", false,
		"Enable leader election for controller manager. ")
	pflag.BoolVar(&config.GlobalResourceEnabled, "global-resource-enabled", false,
		"Enable the global resource. It is expermental feature. Do not support upgrade.")
	pflag.BoolVar(&config.EnablePprof, "enable-pprof", false, "Enable the pprof tool.")
	pflag.Parse()

	config.LogLevel = "info"
	if logflag := defaultFlags.Lookup("zap-log-level"); len(logflag.Value.String()) != 0 {
		config.LogLevel = logflag.Value.String()
	}

	// set zap logger
	ctrl.SetLogger(zap.New(zap.UseFlagOptions(&opts)))
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
