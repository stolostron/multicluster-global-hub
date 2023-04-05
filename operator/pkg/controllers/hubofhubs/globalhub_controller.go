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

package hubofhubs

import (
	"context"
	"embed"
	"fmt"
	"reflect"

	"github.com/go-logr/logr"
	routev1 "github.com/openshift/api/route/v1"
	admissionregistrationv1 "k8s.io/api/admissionregistration/v1"
	appsv1 "k8s.io/api/apps/v1"
	corev1 "k8s.io/api/core/v1"
	rbacv1 "k8s.io/api/rbac/v1"
	"k8s.io/apimachinery/pkg/api/errors"
	"k8s.io/apimachinery/pkg/runtime"
	"k8s.io/apimachinery/pkg/types"
	"k8s.io/client-go/kubernetes"
	addonv1alpha1 "open-cluster-management.io/api/addon/v1alpha1"
	ctrl "sigs.k8s.io/controller-runtime"
	"sigs.k8s.io/controller-runtime/pkg/builder"
	"sigs.k8s.io/controller-runtime/pkg/client"
	"sigs.k8s.io/controller-runtime/pkg/event"
	"sigs.k8s.io/controller-runtime/pkg/handler"
	"sigs.k8s.io/controller-runtime/pkg/manager"
	"sigs.k8s.io/controller-runtime/pkg/predicate"
	"sigs.k8s.io/controller-runtime/pkg/reconcile"
	"sigs.k8s.io/controller-runtime/pkg/source"

	// pmcontroller "github.com/stolostron/multicluster-global-hub/operator/pkg/controllers/packagemanifest"
	operatorv1alpha2 "github.com/stolostron/multicluster-global-hub/operator/apis/v1alpha2"
	"github.com/stolostron/multicluster-global-hub/operator/pkg/condition"
	"github.com/stolostron/multicluster-global-hub/operator/pkg/config"
	"github.com/stolostron/multicluster-global-hub/operator/pkg/utils"
	"github.com/stolostron/multicluster-global-hub/pkg/constants"
	commonobjects "github.com/stolostron/multicluster-global-hub/pkg/objects"
)

//go:embed manifests
var fs embed.FS

// var isPackageManifestControllerRunnning = false

// MulticlusterGlobalHubReconciler reconciles a MulticlusterGlobalHub object
type MulticlusterGlobalHubReconciler struct {
	manager.Manager
	client.Client
	KubeClient     kubernetes.Interface
	Scheme         *runtime.Scheme
	LeaderElection *commonobjects.LeaderElectionConfig
	Log            logr.Logger
}

// +kubebuilder:rbac:groups=operator.open-cluster-management.io,resources=multiclusterglobalhubs,verbs=get;list;watch;create;update;patch;delete
// +kubebuilder:rbac:groups=operator.open-cluster-management.io,resources=multiclusterglobalhubs/status,verbs=get;update;patch
// +kubebuilder:rbac:groups=operator.open-cluster-management.io,resources=multiclusterglobalhubs/finalizers,verbs=update
// +kubebuilder:rbac:groups=cluster.open-cluster-management.io,resources=managedclustersets/join,verbs=create;delete
// +kubebuilder:rbac:groups=cluster.open-cluster-management.io,resources=managedclustersets/bind,verbs=create;delete
// +kubebuilder:rbac:groups=apps.open-cluster-management.io,resources=subscriptions,verbs=get;list;update;patch
// +kubebuilder:rbac:groups=apps.open-cluster-management.io,resources=placementrules,verbs=get;list;update;patch
// +kubebuilder:rbac:groups=apps.open-cluster-management.io,resources=channels,verbs=get;list;update;patch
// +kubebuilder:rbac:groups=policy.open-cluster-management.io,resources=policies,verbs=get;list;patch;update
// +kubebuilder:rbac:groups=policy.open-cluster-management.io,resources=placementbindings,verbs=get;list;patch;update
// +kubebuilder:rbac:groups=app.k8s.io,resources=applications,verbs=get;list;patch;update
// +kubebuilder:rbac:groups=cluster.open-cluster-management.io,resources=placements,verbs=get;list;patch;update
// +kubebuilder:rbac:groups=cluster.open-cluster-management.io,resources=managedclustersets,verbs=get;list;patch;update

// +kubebuilder:rbac:groups="",resources=namespaces,verbs=get;list;watch;create;update;delete
// +kubebuilder:rbac:groups="",resources=services,verbs=get;list;watch;create;update;delete
// +kubebuilder:rbac:groups="",resources=serviceaccounts,verbs=get;list;watch;create;update;delete
// +kubebuilder:rbac:groups="",resources=secrets,verbs=get;list;watch;create;update;delete
// +kubebuilder:rbac:groups="",resources=configmaps,verbs=get;list;watch;create;update;delete
// +kubebuilder:rbac:groups="apps",resources=deployments,verbs=get;list;watch;create;update;delete
// +kubebuilder:rbac:groups="route.openshift.io",resources=routes,verbs=get;list;watch;create;update;delete
// +kubebuilder:rbac:groups="rbac.authorization.k8s.io",resources=roles,verbs=get;list;watch;create;update;delete
// +kubebuilder:rbac:groups="rbac.authorization.k8s.io",resources=rolebindings,verbs=get;list;watch;create;update;delete
// +kubebuilder:rbac:groups="rbac.authorization.k8s.io",resources=clusterroles,verbs=get;list;watch;create;update;delete
// +kubebuilder:rbac:groups="rbac.authorization.k8s.io",resources=clusterrolebindings,verbs=get;list;watch;create;update;delete
// +kubebuilder:rbac:groups="admissionregistration.k8s.io",resources=mutatingwebhookconfigurations,verbs=get;list;watch;create;update;delete
// +kubebuilder:rbac:groups=addon.open-cluster-management.io,resources=clustermanagementaddons,verbs=create;delete;get;list;update;watch
// +kubebuilder:rbac:groups=addon.open-cluster-management.io,resources=clustermanagementaddons/finalizers,verbs=update

// Reconcile is part of the main kubernetes reconciliation loop which aims to
// move the current state of the cluster closer to the desired state.
// TODO(user): Modify the Reconcile function to compare the state specified by
// the MulticlusterGlobalHub object against the actual cluster state, and then
// perform operations to make the cluster state reflect the state specified by
// the user.
//
// For more details, check Reconcile and its Result here:
// - https://pkg.go.dev/sigs.k8s.io/controller-runtime@v0.11.0/pkg/reconcile
func (r *MulticlusterGlobalHubReconciler) Reconcile(ctx context.Context, req ctrl.Request) (ctrl.Result, error) {
	r.Log.Info("reconciling mgh instance", "namespace", req.Namespace, "name", req.Name)

	// Fetch the multiclusterglobalhub instance
	mgh := &operatorv1alpha2.MulticlusterGlobalHub{}
	if err := r.Get(ctx, req.NamespacedName, mgh); err != nil {
		if errors.IsNotFound(err) {
			// Request object not found, could have been deleted after reconcile request.
			// Owned objects are automatically garbage collected. For additional cleanup logic use finalizers.
			// Return and don't requeue
			r.Log.Info("mgh instance not found. Ignoring since object must be deleted")
			return ctrl.Result{}, nil
		}
		// Error reading the object - requeue the request.
		r.Log.Error(err, "Failed to get MulticlusterGlobalHub")
		return ctrl.Result{}, err
	}

	if config.IsPaused(mgh) {
		r.Log.Info("mgh reconciliation is paused, nothing more to do")
		return ctrl.Result{}, nil
	}

	// Deleting the multiclusterglobalhub instance
	if mgh.GetDeletionTimestamp() != nil && utils.Contains(mgh.GetFinalizers(), constants.GlobalHubCleanupFinalizer) {
		if err := r.pruneGlobalHubResources(ctx, mgh); err != nil {
			return ctrl.Result{}, fmt.Errorf("failed to prune Global Hub resources %v", err)
		}
		return ctrl.Result{}, nil
	}

	if mgh.Spec.DataLayer == nil {
		return ctrl.Result{}, fmt.Errorf("empty data layer type")
	}

	switch mgh.Spec.DataLayer.Type {
	case operatorv1alpha2.Native:
		if err := r.reconcileNativeGlobalHub(ctx, mgh); err != nil {
			return ctrl.Result{}, err
		}
	case operatorv1alpha2.LargeScale:
		if err := r.reconcileLargeScaleGlobalHub(ctx, mgh); err != nil {
			return ctrl.Result{}, err
		}
	default:
		return ctrl.Result{}, fmt.Errorf("unsupported data layer type: %s", mgh.Spec.DataLayer.Type)
	}

	// Make sure the reconcile work properly, and then add finalizer to the multiclusterglobalhub instance
	if !utils.Contains(mgh.GetFinalizers(), constants.GlobalHubCleanupFinalizer) {
		mgh.SetFinalizers(append(mgh.GetFinalizers(), constants.GlobalHubCleanupFinalizer))
		r.Log.Info("adding finalizer to mgh instance")
		if err := utils.UpdateObject(ctx, r.Client, mgh); err != nil {
			return ctrl.Result{}, fmt.Errorf("failed to add finalizer to mgh %v", err)
		}
	}

	// // try to start packagemanifest controller if it is not running
	// if !isPackageManifestControllerRunnning {
	// 	if err := (&pmcontroller.PackageManifestReconciler{
	// 		Client: r.Client,
	// 		Scheme: r.Scheme,
	// 	}).SetupWithManager(r.Manager); err != nil {
	// 		log.Error(err, "unable to create controller", "controller", "PackageManifest")
	// 		return ctrl.Result{}, err
	// 	}
	// 	log.Info("packagemanifest controller is started")
	// 	isPackageManifestControllerRunnning = true
	// }

	return ctrl.Result{}, nil
}

func (r *MulticlusterGlobalHubReconciler) reconcileNativeGlobalHub(ctx context.Context,
	mgh *operatorv1alpha2.MulticlusterGlobalHub,
) error {
	return fmt.Errorf("native data layer is not supported yet")
}

func (r *MulticlusterGlobalHubReconciler) reconcileLargeScaleGlobalHub(ctx context.Context,
	mgh *operatorv1alpha2.MulticlusterGlobalHub,
) error {
	// make sure largae scale data type settings are not empty
	if mgh.Spec.DataLayer.LargeScale == nil ||
		mgh.Spec.DataLayer.LargeScale.Postgres.Name == "" ||
		mgh.Spec.DataLayer.LargeScale.Kafka.Name == "" {
		return fmt.Errorf("invalid settings for large scale data layer, " +
			"storage and transport secrets are required")
	}

	// reconcile config: need to be done before reconciling manager and grafana
	// 1. global configMap: open-cluster-management-global-hub-system/multicluster-global-hub-config
	// 2. global image: configMap -> env -> default
	if err := r.reconcileSystemConfig(ctx, mgh); err != nil {
		return err
	}

	// reconcile database
	if err := r.reconcileDatabase(ctx, mgh); err != nil {
		if e := condition.SetConditionDatabaseInit(ctx, r.Client, mgh,
			condition.CONDITION_STATUS_FALSE); e != nil {
			return condition.FailToSetConditionError(condition.CONDITION_STATUS_FALSE, e)
		}
		return err
	}

	// reconcile manager
	if err := r.reconcileManager(ctx, mgh); err != nil {
		return err
	}

	// reconcile grafana
	if err := r.reconcileGrafana(ctx, mgh); err != nil {
		return err
	}

	return nil
}

// SetupWithManager sets up the controller with the Manager.
func (r *MulticlusterGlobalHubReconciler) SetupWithManager(mgr ctrl.Manager) error {
	mghPred := predicate.Funcs{
		CreateFunc: func(e event.CreateEvent) bool {
			// set request name to be used in leafhub controller
			config.SetHoHMGHNamespacedName(types.NamespacedName{
				Namespace: e.Object.GetNamespace(), Name: e.Object.GetName(),
			})
			return true
		},
		UpdateFunc: func(e event.UpdateEvent) bool {
			return e.ObjectOld.GetGeneration() != e.ObjectNew.GetGeneration()
		},
		DeleteFunc: func(e event.DeleteEvent) bool {
			return !e.DeleteStateUnknown
		},
	}

	ownPred := predicate.Funcs{
		CreateFunc: func(e event.CreateEvent) bool {
			return false
		},
		UpdateFunc: func(e event.UpdateEvent) bool {
			return e.ObjectOld.GetGeneration() != e.ObjectNew.GetGeneration() // only requeue when spec change
		},
		DeleteFunc: func(e event.DeleteEvent) bool {
			return true
		},
	}

	resPred := predicate.Funcs{
		CreateFunc: func(e event.CreateEvent) bool {
			return false
		},
		UpdateFunc: func(e event.UpdateEvent) bool {
			if e.ObjectNew.GetLabels()[constants.GlobalHubOwnerLabelKey] ==
				constants.GHOperatorOwnerLabelVal &&
				e.ObjectNew.GetGeneration() != e.ObjectOld.GetGeneration() {
				return true
			}
			return false
		},
		DeleteFunc: func(e event.DeleteEvent) bool {
			return e.Object.GetLabels()[constants.GlobalHubOwnerLabelKey] ==
				constants.GHOperatorOwnerLabelVal
		},
	}

	webhookPred := predicate.Funcs{
		CreateFunc: func(e event.CreateEvent) bool {
			return false
		},
		UpdateFunc: func(e event.UpdateEvent) bool {
			if e.ObjectNew.GetLabels()[constants.GlobalHubOwnerLabelKey] ==
				constants.GHOperatorOwnerLabelVal {
				new := e.ObjectNew.(*admissionregistrationv1.MutatingWebhookConfiguration)
				old := e.ObjectOld.(*admissionregistrationv1.MutatingWebhookConfiguration)
				if len(new.Webhooks) != len(old.Webhooks) ||
					new.Webhooks[0].Name != old.Webhooks[0].Name ||
					!reflect.DeepEqual(new.Webhooks[0].AdmissionReviewVersions,
						old.Webhooks[0].AdmissionReviewVersions) ||
					!reflect.DeepEqual(new.Webhooks[0].Rules, old.Webhooks[0].Rules) ||
					!reflect.DeepEqual(new.Webhooks[0].ClientConfig.Service, old.Webhooks[0].ClientConfig.Service) {
					return e.ObjectNew.GetGeneration() != e.ObjectOld.GetGeneration()
				}
				return false
			}
			return false
		},
		DeleteFunc: func(e event.DeleteEvent) bool {
			return e.Object.GetLabels()[constants.GlobalHubOwnerLabelKey] ==
				constants.GHOperatorOwnerLabelVal
		},
	}

	return ctrl.NewControllerManagedBy(mgr).
		For(&operatorv1alpha2.MulticlusterGlobalHub{}, builder.WithPredicates(mghPred)).
		Owns(&appsv1.Deployment{}, builder.WithPredicates(ownPred)).
		Owns(&corev1.Service{}, builder.WithPredicates(ownPred)).
		Owns(&corev1.ServiceAccount{}, builder.WithPredicates(ownPred)).
		Owns(&corev1.Secret{}, builder.WithPredicates(ownPred)).
		Owns(&rbacv1.Role{}, builder.WithPredicates(ownPred)).
		Owns(&rbacv1.RoleBinding{}, builder.WithPredicates(ownPred)).
		Owns(&routev1.Route{}, builder.WithPredicates(ownPred)).
		Watches(&source.Kind{Type: &admissionregistrationv1.MutatingWebhookConfiguration{}},
			handler.EnqueueRequestsFromMapFunc(func(obj client.Object) []reconcile.Request {
				return []reconcile.Request{
					// trigger MGH instance reconcile
					{NamespacedName: config.GetHoHMGHNamespacedName()},
				}
			}), builder.WithPredicates(webhookPred)).
		// secondary watch for configmap
		Watches(&source.Kind{Type: &corev1.ConfigMap{}},
			handler.EnqueueRequestsFromMapFunc(func(obj client.Object) []reconcile.Request {
				return []reconcile.Request{
					// trigger MGH instance reconcile
					{NamespacedName: config.GetHoHMGHNamespacedName()},
				}
			}), builder.WithPredicates(resPred)).
		// secondary watch for namespace
		Watches(&source.Kind{Type: &corev1.Namespace{}},
			handler.EnqueueRequestsFromMapFunc(func(obj client.Object) []reconcile.Request {
				return []reconcile.Request{
					// trigger MGH instance reconcile
					{NamespacedName: config.GetHoHMGHNamespacedName()},
				}
			}), builder.WithPredicates(resPred)).
		// secondary watch for clusterrole
		Watches(&source.Kind{Type: &rbacv1.ClusterRole{}},
			handler.EnqueueRequestsFromMapFunc(func(obj client.Object) []reconcile.Request {
				return []reconcile.Request{
					// trigger MGH instance reconcile
					{NamespacedName: config.GetHoHMGHNamespacedName()},
				}
			}), builder.WithPredicates(resPred)).
		// secondary watch for clusterrolebinding
		Watches(&source.Kind{Type: &rbacv1.ClusterRoleBinding{}},
			handler.EnqueueRequestsFromMapFunc(func(obj client.Object) []reconcile.Request {
				return []reconcile.Request{
					// trigger MGH instance reconcile
					{NamespacedName: config.GetHoHMGHNamespacedName()},
				}
			}), builder.WithPredicates(resPred)).
		// secondary watch for clustermanagementaddon
		Watches(&source.Kind{Type: &addonv1alpha1.ClusterManagementAddOn{}},
			handler.EnqueueRequestsFromMapFunc(func(obj client.Object) []reconcile.Request {
				return []reconcile.Request{
					// trigger MGH instance reconcile
					{NamespacedName: config.GetHoHMGHNamespacedName()},
				}
			}), builder.WithPredicates(resPred)).
		Complete(r)
}
