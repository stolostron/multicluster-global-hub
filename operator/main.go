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
	hypershiftdeploymentv1alpha1 "github.com/stolostron/hypershift-deployment-controller/api/v1alpha1"
	admissionregistrationv1 "k8s.io/api/admissionregistration/v1"
	appsv1 "k8s.io/api/apps/v1"
	batchv1 "k8s.io/api/batch/v1"
	corev1 "k8s.io/api/core/v1"
	rbacv1 "k8s.io/api/rbac/v1"
	"k8s.io/apimachinery/pkg/api/errors"
	"k8s.io/apimachinery/pkg/labels"
	"k8s.io/apimachinery/pkg/runtime"
	utilruntime "k8s.io/apimachinery/pkg/util/runtime"
	"k8s.io/client-go/kubernetes"
	clientgoscheme "k8s.io/client-go/kubernetes/scheme"

	// Import all Kubernetes client auth plugins (e.g. Azure, GCP, OIDC, etc.)
	// to ensure that exec-entrypoint and run can make use of them.
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	_ "k8s.io/client-go/plugin/pkg/client/auth"
	addonv1alpha1 "open-cluster-management.io/api/addon/v1alpha1"
	clusterv1 "open-cluster-management.io/api/cluster/v1"
	clusterv1beta1 "open-cluster-management.io/api/cluster/v1beta1"
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

	operatorv1alpha2 "github.com/stolostron/multicluster-global-hub/operator/apis/v1alpha2"
	"github.com/stolostron/multicluster-global-hub/operator/pkg/constants"
	hubofhubscontrollers "github.com/stolostron/multicluster-global-hub/operator/pkg/controllers/hubofhubs"
	commonconstants "github.com/stolostron/multicluster-global-hub/pkg/constants"
	commonobjects "github.com/stolostron/multicluster-global-hub/pkg/objects"
)

var (
	scheme        = runtime.NewScheme()
	setupLog      = ctrl.Log.WithName("setup")
	labelSelector = labels.SelectorFromSet(
		labels.Set{
			commonconstants.GlobalHubOwnerLabelKey: commonconstants.HoHOperatorOwnerLabelVal,
		},
	)
)

func init() {
	utilruntime.Must(clientgoscheme.AddToScheme(scheme))
	utilruntime.Must(routev1.AddToScheme(scheme))
	utilruntime.Must(operatorsv1.AddToScheme(scheme))
	utilruntime.Must(clusterv1.AddToScheme(scheme))
	utilruntime.Must(clusterv1beta1.AddToScheme(scheme))
	utilruntime.Must(workv1.AddToScheme(scheme))
	utilruntime.Must(addonv1alpha1.AddToScheme(scheme))
	utilruntime.Must(hypershiftdeploymentv1alpha1.AddToScheme(scheme))
	utilruntime.Must(operatorv1alpha2.AddToScheme(scheme))
	utilruntime.Must(appsubv1.SchemeBuilder.AddToScheme(scheme))
	utilruntime.Must(appsubV1alpha1.AddToScheme(scheme))
	utilruntime.Must(chnv1.AddToScheme(scheme))
	utilruntime.Must(placementrulesv1.AddToScheme(scheme))
	utilruntime.Must(policyv1.AddToScheme(scheme))
	utilruntime.Must(applicationv1beta1.AddToScheme(scheme))
	utilruntime.Must(admissionregistrationv1.AddToScheme(scheme))
	//+kubebuilder:scaffold:scheme
}

func main() {
	var metricsAddr string
	var enableLeaderElection bool
	var probeAddr string
	flag.StringVar(&metricsAddr, "metrics-bind-address", ":8080", "The address the metric endpoint binds to.")
	flag.StringVar(&probeAddr, "health-probe-bind-address", ":8081", "The address the probe endpoint binds to.")
	flag.BoolVar(&enableLeaderElection, "leader-elect", false, "Enable leader election for controller manager. ")
	opts := zap.Options{
		Development: true,
	}
	opts.BindFlags(flag.CommandLine)
	flag.Parse()

	ctrl.SetLogger(zap.New(zap.UseFlagOptions(&opts)))

	// build filtered resource map
	newCacheFunc := cache.BuilderWithOptions(cache.Options{
		SelectorsByObject: cache.SelectorsByObject{
			&corev1.Secret{}: {
				Label: labelSelector,
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
		},
	})

	kubeClient, err := kubernetes.NewForConfig(ctrl.GetConfigOrDie())
	if err != nil {
		setupLog.Error(err, "failed to create kube client")
		os.Exit(1)
	}

	electionConfig, err := GetElectionConfig(kubeClient)
	if err != nil {
		setupLog.Error(err, "failed to get election config")
		os.Exit(1)
	}
	leaseDuration := time.Duration(electionConfig.LeaseDuration) * time.Second
	renewDeadline := time.Duration(electionConfig.RenewDeadline) * time.Second
	retryPeriod := time.Duration(electionConfig.RetryPeriod) * time.Second

	mgr, err := ctrl.NewManager(ctrl.GetConfigOrDie(), ctrl.Options{
		Scheme:                 scheme,
		MetricsBindAddress:     metricsAddr,
		Port:                   9443,
		HealthProbeBindAddress: probeAddr,
		LeaderElection:         enableLeaderElection,
		LeaderElectionID:       "549a8919.open-cluster-management.io",
		LeaseDuration:          &leaseDuration,
		RenewDeadline:          &renewDeadline,
		RetryPeriod:            &retryPeriod,
		NewCache:               newCacheFunc,
	})
	if err != nil {
		setupLog.Error(err, "unable to start manager")
		os.Exit(1)
	}

	if err = (&hubofhubscontrollers.MulticlusterGlobalHubReconciler{
		Manager:        mgr,
		Client:         mgr.GetClient(),
		KubeClient:     kubeClient,
		Scheme:         mgr.GetScheme(),
		LeaderElection: electionConfig,
	}).SetupWithManager(mgr); err != nil {
		setupLog.Error(err, "unable to create controller", "controller", "MulticlusterGlobalHub")
		os.Exit(1)
	}
	//+kubebuilder:scaffold:builder

	if err := mgr.AddHealthzCheck("healthz", healthz.Ping); err != nil {
		setupLog.Error(err, "unable to set up health check")
		os.Exit(1)
	}
	if err := mgr.AddReadyzCheck("readyz", healthz.Ping); err != nil {
		setupLog.Error(err, "unable to set up ready check")
		os.Exit(1)
	}

	setupLog.Info("starting manager")
	if err := mgr.Start(ctrl.SetupSignalHandler()); err != nil {
		setupLog.Error(err, "problem running manager")
		os.Exit(1)
	}
}

func GetElectionConfig(kubeClient *kubernetes.Clientset) (*commonobjects.LeaderElectionConfig, error) {
	config := &commonobjects.LeaderElectionConfig{
		LeaseDuration: 137,
		RenewDeadline: 107,
		RetryPeriod:   26,
	}

	configMap, err := kubeClient.CoreV1().ConfigMaps(constants.HOHDefaultNamespace).Get(
		context.TODO(), commonconstants.ControllerLeaderElectionConfig, metav1.GetOptions{})
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
