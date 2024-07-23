package config

import (
	admissionregistrationv1 "k8s.io/api/admissionregistration/v1"
	appsv1 "k8s.io/api/apps/v1"
	corev1 "k8s.io/api/core/v1"
	rbacv1 "k8s.io/api/rbac/v1"
	"k8s.io/apimachinery/pkg/fields"
	"k8s.io/apimachinery/pkg/labels"
	"k8s.io/client-go/rest"
	"sigs.k8s.io/controller-runtime/pkg/cache"
	"sigs.k8s.io/controller-runtime/pkg/client"

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
				utils.GetDefaultNamespace(): {},
			},
		},
		&admissionregistrationv1.MutatingWebhookConfiguration{}: {
			Label: labelSelector,
		},
	}
	return cache.New(config, cacheOpts)
}
