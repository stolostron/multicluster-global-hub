package config

import (
	subv1alpha1 "github.com/operator-framework/api/pkg/operators/v1alpha1"
	// Import all Kubernetes client auth plugins (e.g. Azure, GCP, OIDC, etc.)
	// to ensure that exec-entrypoint and run can make use of them.
	promv1 "github.com/prometheus-operator/prometheus-operator/pkg/apis/monitoring/v1"
	mchv1 "github.com/stolostron/multiclusterhub-operator/api/v1"
	admissionregistrationv1 "k8s.io/api/admissionregistration/v1"
	appsv1 "k8s.io/api/apps/v1"
	corev1 "k8s.io/api/core/v1"
	rbacv1 "k8s.io/api/rbac/v1"
	"k8s.io/apimachinery/pkg/fields"
	"k8s.io/apimachinery/pkg/labels"
	_ "k8s.io/client-go/plugin/pkg/client/auth"
	"k8s.io/client-go/rest"
	addonv1alpha1 "open-cluster-management.io/api/addon/v1alpha1"
	clusterv1 "open-cluster-management.io/api/cluster/v1"
	ctrl "sigs.k8s.io/controller-runtime"
	"sigs.k8s.io/controller-runtime/pkg/cache"
	"sigs.k8s.io/controller-runtime/pkg/client"

	globalhubv1alpha4 "github.com/stolostron/multicluster-global-hub/operator/apis/v1alpha4"
	"github.com/stolostron/multicluster-global-hub/pkg/constants"
	"github.com/stolostron/multicluster-global-hub/pkg/utils"
)

var labelSelector = labels.SelectorFromSet(
	labels.Set{
		constants.GlobalHubOwnerLabelKey: constants.GHOperatorOwnerLabelVal,
	},
)

func InitCache(config *rest.Config, cacheOpts cache.Options) (cache.Cache, error) {
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
	}
	return cache.New(config, cacheOpts)
}

func ACMCache(mgr ctrl.Manager) (cache.Cache, error) {
	cacheOptions := cache.Options{
		Scheme: mgr.GetScheme(),
	}
	cacheOptions.ByObject = map[client.Object]cache.ByObject{
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
	}
	extendCache, err := cache.New(mgr.GetConfig(), cacheOptions)
	if err != nil {
		return nil, err
	}
	return extendCache, err
}

func BackupCache(mgr ctrl.Manager) (cache.Cache, error) {
	cacheOptions := cache.Options{
		Scheme: mgr.GetScheme(),
	}
	cacheOptions.ByObject = map[client.Object]cache.ByObject{
		&mchv1.MultiClusterHub{}: {},
	}
	extendCache, err := cache.New(mgr.GetConfig(), cacheOptions)
	if err != nil {
		return nil, err
	}
	return extendCache, err
}
