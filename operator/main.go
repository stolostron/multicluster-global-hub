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
	"flag"
	"os"
	"time"

	// Import all Kubernetes client auth plugins (e.g. Azure, GCP, OIDC, etc.)
	// to ensure that exec-entrypoint and run can make use of them.
	kafkav1beta2 "github.com/RedHatInsights/strimzi-client-go/apis/kafka.strimzi.io/v1beta2"
	routev1 "github.com/openshift/api/route/v1"
	subv1alpha1 "github.com/operator-framework/api/pkg/operators/v1alpha1"
	operatorsv1 "github.com/operator-framework/operator-lifecycle-manager/pkg/package-server/apis/operators/v1"
	// Import all Kubernetes client auth plugins (e.g. Azure, GCP, OIDC, etc.)
	// to ensure that exec-entrypoint and run can make use of them.
	promv1 "github.com/prometheus-operator/prometheus-operator/pkg/apis/monitoring/v1"
	"github.com/spf13/pflag"
	agentv1 "github.com/stolostron/klusterlet-addon-controller/pkg/apis/agent/v1"
	mchv1 "github.com/stolostron/multiclusterhub-operator/api/v1"
	admissionregistrationv1 "k8s.io/api/admissionregistration/v1"
	appsv1 "k8s.io/api/apps/v1"
	corev1 "k8s.io/api/core/v1"
	rbacv1 "k8s.io/api/rbac/v1"
	apiextensionsv1 "k8s.io/apiextensions-apiserver/pkg/apis/apiextensions/v1"
	"k8s.io/apimachinery/pkg/fields"
	"k8s.io/apimachinery/pkg/labels"
	"k8s.io/apimachinery/pkg/runtime"
	utilruntime "k8s.io/apimachinery/pkg/util/runtime"
	"k8s.io/client-go/kubernetes"
	clientgoscheme "k8s.io/client-go/kubernetes/scheme"
	_ "k8s.io/client-go/plugin/pkg/client/auth"
	"k8s.io/client-go/rest"
	addonv1alpha1 "open-cluster-management.io/api/addon/v1alpha1"
	clusterv1 "open-cluster-management.io/api/cluster/v1"
	clusterv1beta1 "open-cluster-management.io/api/cluster/v1beta1"
	clusterv1beta2 "open-cluster-management.io/api/cluster/v1beta2"
	workv1 "open-cluster-management.io/api/work/v1"
	policyv1 "open-cluster-management.io/governance-policy-propagator/api/v1"
	chnv1 "open-cluster-management.io/multicloud-operators-channel/pkg/apis/apps/v1"
	placementrulesv1 "open-cluster-management.io/multicloud-operators-subscription/pkg/apis/apps/placementrule/v1"
	appsubv1 "open-cluster-management.io/multicloud-operators-subscription/pkg/apis/apps/v1"
	appsubV1alpha1 "open-cluster-management.io/multicloud-operators-subscription/pkg/apis/apps/v1alpha1"
	applicationv1beta1 "sigs.k8s.io/application/api/v1beta1"
	ctrl "sigs.k8s.io/controller-runtime"
	"sigs.k8s.io/controller-runtime/pkg/cache"
	"sigs.k8s.io/controller-runtime/pkg/client"
	"sigs.k8s.io/controller-runtime/pkg/healthz"
	"sigs.k8s.io/controller-runtime/pkg/log/zap"
	metricsserver "sigs.k8s.io/controller-runtime/pkg/metrics/server"

	globalhubv1alpha4 "github.com/stolostron/multicluster-global-hub/operator/apis/v1alpha4"
	"github.com/stolostron/multicluster-global-hub/operator/pkg/config"
	"github.com/stolostron/multicluster-global-hub/operator/pkg/controllers/crd"
	"github.com/stolostron/multicluster-global-hub/pkg/constants"
	"github.com/stolostron/multicluster-global-hub/pkg/utils"
)

var (
	scheme   = runtime.NewScheme()
	setupLog = ctrl.Log.WithName("setup")

	labelSelector = labels.SelectorFromSet(
		labels.Set{
			constants.GlobalHubOwnerLabelKey: constants.GHOperatorOwnerLabelVal,
		},
	)
)

func init() {
	utilruntime.Must(clientgoscheme.AddToScheme(scheme))
	utilruntime.Must(routev1.AddToScheme(scheme))
	utilruntime.Must(operatorsv1.AddToScheme(scheme))
	utilruntime.Must(clusterv1.AddToScheme(scheme))
	utilruntime.Must(clusterv1beta1.AddToScheme(scheme))
	utilruntime.Must(clusterv1beta2.AddToScheme(scheme))
	utilruntime.Must(workv1.AddToScheme(scheme))
	utilruntime.Must(addonv1alpha1.AddToScheme(scheme))
	utilruntime.Must(globalhubv1alpha4.AddToScheme(scheme))
	utilruntime.Must(appsubv1.SchemeBuilder.AddToScheme(scheme))
	utilruntime.Must(appsubV1alpha1.AddToScheme(scheme))
	utilruntime.Must(subv1alpha1.AddToScheme(scheme))
	utilruntime.Must(chnv1.AddToScheme(scheme))
	utilruntime.Must(placementrulesv1.AddToScheme(scheme))
	utilruntime.Must(policyv1.AddToScheme(scheme))
	utilruntime.Must(applicationv1beta1.AddToScheme(scheme))
	utilruntime.Must(admissionregistrationv1.AddToScheme(scheme))
	utilruntime.Must(mchv1.AddToScheme(scheme))
	utilruntime.Must(agentv1.SchemeBuilder.AddToScheme(scheme))
	utilruntime.Must(promv1.AddToScheme(scheme))
	utilruntime.Must(apiextensionsv1.AddToScheme(scheme))

	// add Kafka scheme
	utilruntime.Must(kafkav1beta2.AddToScheme(scheme))
	// +kubebuilder:scaffold:scheme
}

func main() {
	os.Exit(doMain(ctrl.SetupSignalHandler(), ctrl.GetConfigOrDie()))
}

func doMain(ctx context.Context, cfg *rest.Config) int {
	operatorConfig := parseFlags()
	utils.PrintVersion(setupLog)

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

	_, err = crd.AddCRDController(mgr, operatorConfig, kubeClient)
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
		Scheme: scheme,
		Metrics: metricsserver.Options{
			BindAddress: operatorConfig.MetricsAddress,
		},
		HealthProbeBindAddress:  operatorConfig.ProbeAddress,
		LeaderElection:          operatorConfig.LeaderElection,
		LeaderElectionID:        "multicluster-global-hub-operator-lock",
		LeaderElectionNamespace: operatorConfig.PodNamespace,
		LeaseDuration:           &leaseDuration,
		RenewDeadline:           &renewDeadline,
		RetryPeriod:             &retryPeriod,
		NewCache:                initCache,
	})

	return mgr, err
}

func initCache(config *rest.Config, cacheOpts cache.Options) (cache.Cache, error) {
	cacheOpts.ByObject = map[client.Object]cache.ByObject{
		// addon installer: transport credentials and image pull secret
		// global hub controller
		&corev1.Secret{}: {
			Namespaces: map[string]cache.Config{
				utils.GetDefaultNamespace(): {},
			},
		},
		// global hub condition controller(status changed)
		// global hub controller(spec changed)
		&appsv1.Deployment{}: {
			Namespaces: map[string]cache.Config{
				utils.GetDefaultNamespace(): {LabelSelector: labelSelector},
			},
		},
		// global hub controller - postgres
		&appsv1.StatefulSet{}: {
			Namespaces: map[string]cache.Config{
				utils.GetDefaultNamespace(): {LabelSelector: labelSelector},
			},
		},
		// global hub controller
		&corev1.Service{}: {
			Namespaces: map[string]cache.Config{
				utils.GetDefaultNamespace(): {LabelSelector: labelSelector},
			},
		},
		// global hub controller
		&corev1.ServiceAccount{}: {
			Namespaces: map[string]cache.Config{
				utils.GetDefaultNamespace(): {LabelSelector: labelSelector},
			},
		},
		// global hub controller: postgresCA and custom alert
		&corev1.ConfigMap{}: {
			Namespaces: map[string]cache.Config{
				utils.GetDefaultNamespace(): {},
			},
		},
		// global hub controller
		&rbacv1.Role{}: {
			Namespaces: map[string]cache.Config{
				utils.GetDefaultNamespace(): {LabelSelector: labelSelector},
			},
		},
		&rbacv1.RoleBinding{}: {
			Namespaces: map[string]cache.Config{
				utils.GetDefaultNamespace(): {LabelSelector: labelSelector},
			},
		},
		&rbacv1.ClusterRole{}: {
			Label: labelSelector,
		},
		&rbacv1.ClusterRoleBinding{}: {
			Label: labelSelector,
		},
		&corev1.Namespace{}: {
			Field: fields.SelectorFromSet(
				fields.Set{
					"metadata.name": utils.GetDefaultNamespace(),
				},
			),
		},
		&corev1.PersistentVolumeClaim{}: {
			Namespaces: map[string]cache.Config{
				utils.GetDefaultNamespace(): {LabelSelector: labelSelector},
			},
		},
		&admissionregistrationv1.MutatingWebhookConfiguration{}: {
			Label: labelSelector,
		},
		&globalhubv1alpha4.MulticlusterGlobalHub{}: {},

		// addon installer, global hub controller
		&clusterv1.ManagedCluster{}: {
			Label: labels.SelectorFromSet(labels.Set{"vendor": "OpenShift"}),
		},
		// addon installer, global hub controller
		&addonv1alpha1.ClusterManagementAddOn{}: {
			Label: labelSelector,
		},
		// addon installer
		&addonv1alpha1.ManagedClusterAddOn{}: {
			Label: labelSelector,
		},
		// global hub controller
		&promv1.ServiceMonitor{}: {
			Label: labelSelector,
		},
		// global hub controller
		&subv1alpha1.Subscription{}: {},
		// backup controller
		&mchv1.MultiClusterHub{}: {},
	}
	return cache.New(config, cacheOpts)
}
