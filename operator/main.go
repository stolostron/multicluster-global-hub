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
	"strconv"
	"time"

	routev1 "github.com/openshift/api/route/v1"
	operatorsv1 "github.com/operator-framework/operator-lifecycle-manager/pkg/package-server/apis/operators/v1"

	// Import all Kubernetes client auth plugins (e.g. Azure, GCP, OIDC, etc.)
	// to ensure that exec-entrypoint and run can make use of them.
	promv1 "github.com/prometheus-operator/prometheus-operator/pkg/apis/monitoring/v1"
	"github.com/spf13/pflag"
	hypershiftdeploymentv1alpha1 "github.com/stolostron/hypershift-deployment-controller/api/v1alpha1"
	agentv1 "github.com/stolostron/klusterlet-addon-controller/pkg/apis/agent/v1"
	mchv1 "github.com/stolostron/multiclusterhub-operator/api/v1"
	admissionregistrationv1 "k8s.io/api/admissionregistration/v1"
	appsv1 "k8s.io/api/apps/v1"
	batchv1 "k8s.io/api/batch/v1"
	corev1 "k8s.io/api/core/v1"
	rbacv1 "k8s.io/api/rbac/v1"
	"k8s.io/apimachinery/pkg/api/errors"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/fields"
	"k8s.io/apimachinery/pkg/labels"
	"k8s.io/apimachinery/pkg/runtime"
	utilruntime "k8s.io/apimachinery/pkg/util/runtime"
	"k8s.io/client-go/kubernetes"
	clientgoscheme "k8s.io/client-go/kubernetes/scheme"

	// Import all Kubernetes client auth plugins (e.g. Azure, GCP, OIDC, etc.)
	// to ensure that exec-entrypoint and run can make use of them.
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
	"sigs.k8s.io/controller-runtime/pkg/healthz"
	"sigs.k8s.io/controller-runtime/pkg/log/zap"

	globalhubv1alpha4 "github.com/stolostron/multicluster-global-hub/operator/apis/v1alpha4"
	hubofhubsconfig "github.com/stolostron/multicluster-global-hub/operator/pkg/config"
	operatorconstants "github.com/stolostron/multicluster-global-hub/operator/pkg/constants"
	hubofhubsaddon "github.com/stolostron/multicluster-global-hub/operator/pkg/controllers/addon"
	hubofhubscontrollers "github.com/stolostron/multicluster-global-hub/operator/pkg/controllers/hubofhubs"
	"github.com/stolostron/multicluster-global-hub/pkg/constants"
	commonobjects "github.com/stolostron/multicluster-global-hub/pkg/objects"
	"github.com/stolostron/multicluster-global-hub/pkg/utils"
)

var (
	scheme        = runtime.NewScheme()
	setupLog      = ctrl.Log.WithName("setup")
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
	utilruntime.Must(hypershiftdeploymentv1alpha1.AddToScheme(scheme))
	utilruntime.Must(globalhubv1alpha4.AddToScheme(scheme))
	utilruntime.Must(appsubv1.SchemeBuilder.AddToScheme(scheme))
	utilruntime.Must(appsubV1alpha1.AddToScheme(scheme))
	utilruntime.Must(chnv1.AddToScheme(scheme))
	utilruntime.Must(placementrulesv1.AddToScheme(scheme))
	utilruntime.Must(policyv1.AddToScheme(scheme))
	utilruntime.Must(applicationv1beta1.AddToScheme(scheme))
	utilruntime.Must(admissionregistrationv1.AddToScheme(scheme))
	utilruntime.Must(mchv1.AddToScheme(scheme))
	utilruntime.Must(agentv1.SchemeBuilder.AddToScheme(scheme))
	utilruntime.Must(promv1.AddToScheme(scheme))
	// +kubebuilder:scaffold:scheme
}

type operatorConfig struct {
	MetricsAddress string
	ProbeAddress   string
	PodNamespace   string
	LeaderElection bool
}

func main() {
	os.Exit(doMain(ctrl.SetupSignalHandler(), ctrl.GetConfigOrDie()))
}

func doMain(ctx context.Context, cfg *rest.Config) int {
	operatorConfig := parseFlags()
	utils.PrintVersion(setupLog)

	// build filtered resource map
	newCacheFunc := buildResourceFilterMap()

	kubeClient, err := kubernetes.NewForConfig(cfg)
	if err != nil {
		setupLog.Error(err, "failed to create kube client")
		return 1
	}

	electionConfig, err := getElectionConfig(kubeClient)
	if err != nil {
		setupLog.Error(err, "failed to get election config")
		return 1
	}

	mgr, err := getManager(cfg, electionConfig, newCacheFunc, operatorConfig)
	if err != nil {
		setupLog.Error(err, "unable to start manager")
		return 1
	}

	// start addon controller
	if err = (&hubofhubsaddon.HoHAddonInstallReconciler{
		Client: mgr.GetClient(),
		Log:    ctrl.Log.WithName("addon-reconciler"),
	}).SetupWithManager(ctx, mgr); err != nil {
		setupLog.Error(err, "unable to create addon reconciler")
		return 1
	}

	addonController, err := hubofhubsaddon.NewHoHAddonController(mgr.GetConfig(), mgr.GetClient(), electionConfig)
	if err != nil {
		setupLog.Error(err, "unable to create addon controller")
		return 1
	}
	if err = mgr.Add(addonController); err != nil {
		setupLog.Error(err, "unable to add addon controller to manager")
		return 1
	}

	if err = (&hubofhubscontrollers.GlobalHubConditionReconciler{
		Client: mgr.GetClient(),
		Log:    ctrl.Log.WithName("condition-reconciler"),
	}).SetupWithManager(mgr); err != nil {
		setupLog.Error(err, "unable to create GlobalHubStatusReconciler")
		return 1
	}

	if err = (&hubofhubscontrollers.MulticlusterGlobalHubReconciler{
		Manager:        mgr,
		Client:         mgr.GetClient(),
		AddonManager:   addonController.AddonManager(),
		KubeClient:     kubeClient,
		Scheme:         mgr.GetScheme(),
		LeaderElection: electionConfig,
		Log:            ctrl.Log.WithName("global-hub-reconciler"),
	}).SetupWithManager(mgr); err != nil {
		setupLog.Error(err, "unable to create MulticlusterGlobalHubReconciler")
		return 1
	}

	// +kubebuilder:scaffold:builder

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

func parseFlags() *operatorConfig {
	config := &operatorConfig{
		PodNamespace: hubofhubsconfig.GetDefaultNamespace(),
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
	pflag.Parse()

	// set zap logger
	ctrl.SetLogger(zap.New(zap.UseFlagOptions(&opts)))
	return config
}

func buildResourceFilterMap() cache.NewCacheFunc {
	return cache.BuilderWithOptions(cache.Options{
		SelectorsByObject: cache.SelectorsByObject{
			&corev1.Secret{}: {
				Field: fields.SelectorFromSet(fields.Set{"metadata.namespace": hubofhubsconfig.GetDefaultNamespace()}),
			},
			&corev1.ConfigMap{}: {
				Label: labelSelector,
			},
			&corev1.ServiceAccount{}: {
				Label: labelSelector,
			},
			&corev1.Service{}: {
				Label: labelSelector,
			},
			&appsv1.Deployment{}: {
				Label: labelSelector,
			},
			&batchv1.Job{}: {
				Label: labelSelector,
			},
			&rbacv1.Role{}: {
				Label: labelSelector,
			},
			&rbacv1.RoleBinding{}: {
				Label: labelSelector,
			},
			&rbacv1.ClusterRole{}: {
				Label: labelSelector,
			},
			&rbacv1.ClusterRoleBinding{}: {
				Label: labelSelector,
			},
			&routev1.Route{}: {
				Label: labelSelector,
			},
			&clusterv1.ManagedCluster{}: {
				Label: labels.SelectorFromSet(labels.Set{"vendor": "OpenShift"}),
			},
			&workv1.ManifestWork{}: {
				Label: labelSelector,
			},
			&addonv1alpha1.ClusterManagementAddOn{}: {
				Label: labelSelector,
			},
			&addonv1alpha1.ManagedClusterAddOn{}: {
				Label: labelSelector,
			},
			&admissionregistrationv1.MutatingWebhookConfiguration{}: {
				Label: labelSelector,
			},
			&promv1.ServiceMonitor{}: {
				Label: labelSelector,
			},
		},
	})
}

func getManager(restConfig *rest.Config, electionConfig *commonobjects.LeaderElectionConfig,
	newCacheFunc cache.NewCacheFunc, operatorConfig *operatorConfig,
) (ctrl.Manager, error) {
	leaseDuration := time.Duration(electionConfig.LeaseDuration) * time.Second
	renewDeadline := time.Duration(electionConfig.RenewDeadline) * time.Second
	retryPeriod := time.Duration(electionConfig.RetryPeriod) * time.Second

	mgr, err := ctrl.NewManager(restConfig, ctrl.Options{
		Scheme:                  scheme,
		MetricsBindAddress:      operatorConfig.MetricsAddress,
		Port:                    9443,
		HealthProbeBindAddress:  operatorConfig.ProbeAddress,
		LeaderElection:          operatorConfig.LeaderElection,
		LeaderElectionID:        "549a8919.open-cluster-management.io",
		LeaderElectionNamespace: operatorConfig.PodNamespace,
		LeaseDuration:           &leaseDuration,
		RenewDeadline:           &renewDeadline,
		RetryPeriod:             &retryPeriod,
		NewCache:                newCacheFunc,
	})
	return mgr, err
}

func getElectionConfig(kubeClient *kubernetes.Clientset) (*commonobjects.LeaderElectionConfig, error) {
	config := &commonobjects.LeaderElectionConfig{
		LeaseDuration: 137,
		RenewDeadline: 107,
		RetryPeriod:   26,
	}

	configMap, err := kubeClient.CoreV1().ConfigMaps(constants.GHDefaultNamespace).Get(
		context.TODO(), operatorconstants.ControllerLeaderElectionConfig, metav1.GetOptions{})
	if errors.IsNotFound(err) {
		return config, nil
	}
	if err != nil {
		return nil, err
	}

	leaseDurationSec, err := strconv.Atoi(configMap.Data["leaseDuration"])
	if err != nil {
		return nil, err
	}

	renewDeadlineSec, err := strconv.Atoi(configMap.Data["renewDeadline"])
	if err != nil {
		return nil, err
	}

	retryPeriodSec, err := strconv.Atoi(configMap.Data["retryPeriod"])
	if err != nil {
		return nil, err
	}

	config.LeaseDuration = leaseDurationSec
	config.RenewDeadline = renewDeadlineSec
	config.RetryPeriod = retryPeriodSec
	return config, nil
}
