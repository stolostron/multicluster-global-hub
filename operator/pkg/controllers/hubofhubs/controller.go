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
	"fmt"
	"reflect"
	"sync"

	"github.com/go-logr/logr"
	routev1 "github.com/openshift/api/route/v1"
	subv1alpha1 "github.com/operator-framework/api/pkg/operators/v1alpha1"
	promv1 "github.com/prometheus-operator/prometheus-operator/pkg/apis/monitoring/v1"
	admissionregistrationv1 "k8s.io/api/admissionregistration/v1"
	appsv1 "k8s.io/api/apps/v1"
	corev1 "k8s.io/api/core/v1"
	rbacv1 "k8s.io/api/rbac/v1"
	"k8s.io/apimachinery/pkg/api/errors"
	"k8s.io/apimachinery/pkg/util/sets"
	"k8s.io/client-go/kubernetes"
	"open-cluster-management.io/addon-framework/pkg/addonmanager"
	"open-cluster-management.io/api/addon/v1alpha1"
	clusterv1 "open-cluster-management.io/api/cluster/v1"
	ctrl "sigs.k8s.io/controller-runtime"
	"sigs.k8s.io/controller-runtime/pkg/builder"
	"sigs.k8s.io/controller-runtime/pkg/client"
	"sigs.k8s.io/controller-runtime/pkg/controller/controllerutil"
	"sigs.k8s.io/controller-runtime/pkg/event"
	"sigs.k8s.io/controller-runtime/pkg/handler"
	"sigs.k8s.io/controller-runtime/pkg/predicate"
	"sigs.k8s.io/controller-runtime/pkg/reconcile"

	// pmcontroller "github.com/stolostron/multicluster-global-hub/operator/pkg/controllers/packagemanifest"
	"github.com/stolostron/multicluster-global-hub/operator/apis/v1alpha4"
	"github.com/stolostron/multicluster-global-hub/operator/pkg/config"
	"github.com/stolostron/multicluster-global-hub/operator/pkg/controllers/hubofhubs/grafana"
	"github.com/stolostron/multicluster-global-hub/operator/pkg/controllers/hubofhubs/manager"
	"github.com/stolostron/multicluster-global-hub/operator/pkg/controllers/hubofhubs/metrics"
	"github.com/stolostron/multicluster-global-hub/operator/pkg/controllers/hubofhubs/prune"
	"github.com/stolostron/multicluster-global-hub/operator/pkg/controllers/hubofhubs/status"
	"github.com/stolostron/multicluster-global-hub/operator/pkg/controllers/hubofhubs/storage"
	"github.com/stolostron/multicluster-global-hub/operator/pkg/controllers/hubofhubs/transporter"
	"github.com/stolostron/multicluster-global-hub/operator/pkg/controllers/hubofhubs/transporter/protocol"
	"github.com/stolostron/multicluster-global-hub/operator/pkg/utils"
	"github.com/stolostron/multicluster-global-hub/pkg/constants"
)

// GlobalHubController reconciles a MulticlusterGlobalHub object
type GlobalHubController struct {
	log logr.Logger
	ctrl.Manager
	client.Client
	addonMgr            addonmanager.AddonManager
	upgraded            bool
	operatorConfig      *config.OperatorConfig
	pruneReconciler     *prune.PruneReconciler
	metricsReconciler   *metrics.MetricsReconciler
	storageReconciler   *storage.StorageReconciler
	transportReconciler *transporter.TransportReconciler
	statusReconciler    *status.StatusReconciler
	managerReconciler   *manager.ManagerReconciler
	grafanaReconciler   *grafana.GrafanaReconciler
}

func NewGlobalHubController(mgr ctrl.Manager, addonMgr addonmanager.AddonManager,
	kubeClient kubernetes.Interface, operatorConfig *config.OperatorConfig,
) *GlobalHubController {
	return &GlobalHubController{
		log:                 ctrl.Log.WithName("global-hub-controller"),
		Manager:             mgr,
		Client:              mgr.GetClient(),
		addonMgr:            addonMgr,
		operatorConfig:      operatorConfig,
		pruneReconciler:     prune.NewPruneReconciler(mgr.GetClient()),
		metricsReconciler:   metrics.NewMetricsReconciler(mgr.GetClient()),
		storageReconciler:   storage.NewStorageReconciler(mgr, operatorConfig.GlobalResourceEnabled),
		transportReconciler: transporter.NewTransportReconciler(mgr),
		statusReconciler:    status.NewStatusReconciler(mgr.GetClient()),
		managerReconciler:   manager.NewManagerReconciler(mgr, kubeClient, operatorConfig),
		grafanaReconciler:   grafana.NewGrafanaReconciler(mgr, kubeClient),
	}
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
// +kubebuilder:rbac:groups=cluster.open-cluster-management.io,resources=managedclusters,verbs=get;list;update
// +kubebuilder:rbac:groups="",resources=namespaces,verbs=get;list;watch;create;update;delete
// +kubebuilder:rbac:groups="",resources=services,verbs=get;list;watch;create;update;delete
// +kubebuilder:rbac:groups="",resources=serviceaccounts,verbs=get;list;watch;create;update;delete
// +kubebuilder:rbac:groups="",resources=secrets,verbs=get;list;watch;create;update;delete
// +kubebuilder:rbac:groups="",resources=configmaps,verbs=get;list;watch;create;update;delete
// +kubebuilder:rbac:groups="",resources=persistentvolumeclaims,verbs=get;list;watch;create;update;delete;patch
// +kubebuilder:rbac:groups="apps",resources=deployments,verbs=get;list;watch;create;update;delete
// +kubebuilder:rbac:groups="apps",resources=statefulsets,verbs=get;list;watch;create;update;delete
// +kubebuilder:rbac:groups="route.openshift.io",resources=routes,verbs=get;list;watch;create;update;delete
// +kubebuilder:rbac:groups="rbac.authorization.k8s.io",resources=roles,verbs=get;list;watch;create;update;delete
// +kubebuilder:rbac:groups="rbac.authorization.k8s.io",resources=rolebindings,verbs=get;list;watch;create;update;delete
// +kubebuilder:rbac:groups="rbac.authorization.k8s.io",resources=clusterroles,verbs=get;list;watch;create;update;delete
// +kubebuilder:rbac:groups="rbac.authorization.k8s.io",resources=clusterrolebindings,verbs=get;list;watch;create;update;delete
// +kubebuilder:rbac:groups="admissionregistration.k8s.io",resources=mutatingwebhookconfigurations,verbs=get;list;watch;create;update;delete
// +kubebuilder:rbac:groups=addon.open-cluster-management.io,resources=clustermanagementaddons,verbs=create;delete;get;list;update;watch
// +kubebuilder:rbac:groups=addon.open-cluster-management.io,resources=clustermanagementaddons/finalizers,verbs=update
// +kubebuilder:rbac:groups=operator.open-cluster-management.io,resources=multiclusterhubs,verbs=get;list;patch;update;watch
// +kubebuilder:rbac:groups=monitoring.coreos.com,resources=servicemonitors;prometheusrules;podmonitors,verbs=get;create;delete;update;list;watch
// +kubebuilder:rbac:groups=monitoring.coreos.com,resources=prometheuses/api,resourceNames=k8s,verbs=get;create;update
// +kubebuilder:rbac:groups=operators.coreos.com,resources=subscriptions,verbs=get;create;delete;update;list;watch
// +kubebuilder:rbac:groups=operators.coreos.com,resources=clusterserviceversions,verbs=delete
// +kubebuilder:rbac:groups=postgres-operator.crunchydata.com,resources=postgresclusters,verbs=get;create;list;watch
// +kubebuilder:rbac:groups=kafka.strimzi.io,resources=kafkas;kafkatopics;kafkausers,verbs=get;create;list;watch;update;delete
// +kubebuilder:rbac:groups=apiextensions.k8s.io,resources=customresourcedefinitions,verbs=get;list;watch;update

// Reconcile is part of the main kubernetes reconciliation loop which aims to
// move the current state of the cluster closer to the desired state.
// Modify the Reconcile function to compare the state specified by
// the MulticlusterGlobalHub object against the actual cluster state, and then
// perform operations to make the cluster state reflect the state specified by
// the user.
//
// For more details, check Reconcile and its Result here:
// - https://pkg.go.dev/sigs.k8s.io/controller-runtime@v0.11.0/pkg/reconcile
func (r *GlobalHubController) Reconcile(ctx context.Context, req ctrl.Request) (ctrl.Result, error) {
	r.log.V(2).Info("reconciling mgh instance", "namespace", req.Namespace, "name", req.Name)
	mgh, err := config.GetMulticlusterGlobalHub(ctx, req, r.Client)
	if err != nil {
		if errors.IsNotFound(err) {
			// Request object not found, could have been deleted after reconcile request.
			// Owned objects are automatically garbage collected. For additional cleanup logic use finalizers.
			// Return and don't requeue
			r.log.Info("mgh instance not found", "namespace", req.Namespace, "name", req.Name)
			return ctrl.Result{}, nil
		}
		r.log.Error(err, "failed to get MulticlusterGlobalHub")
		return ctrl.Result{}, err
	}
	if config.IsPaused(mgh) {
		r.log.Info("mgh controller is paused, nothing more to do")
		return ctrl.Result{}, nil
	}

	// update status condition
	defer func() {
		err := r.statusReconciler.Reconcile(ctx, mgh, err)
		if err != nil {
			r.log.Error(err, "failed to update the instance condition")
		}
	}()

	// prune resources if deleting mgh or metrics is disabled
	if err = r.pruneReconciler.Reconcile(ctx, mgh); err != nil {
		return ctrl.Result{}, fmt.Errorf("failed to prune Global Hub resources %v", err)
	}
	if mgh.GetDeletionTimestamp() != nil {
		return ctrl.Result{}, nil
	}

	// update the managed hub clusters
	// only reconcile once: upgrade
	if !r.upgraded {
		if err = utils.RemoveManagedHubClusterFinalizer(ctx, r.Client); err != nil {
			return ctrl.Result{}, fmt.Errorf("failed to upgrade from release-2.10: %v", err)
		}
		r.upgraded = true
	}
	if err := utils.AnnotateManagedHubCluster(ctx, r.Client); err != nil {
		return ctrl.Result{}, err
	}

	// storage and transporter
	if err = r.ReconcileMiddleware(ctx, mgh); err != nil {
		return ctrl.Result{}, err
	}

	// reconcile metrics
	if err := r.metricsReconciler.Reconcile(ctx, mgh); err != nil {
		return ctrl.Result{}, err
	}

	// reconciler manager and manager
	if err := r.managerReconciler.Reconcile(ctx, mgh); err != nil {
		return ctrl.Result{}, err
	}
	if err := r.grafanaReconciler.Reconcile(ctx, mgh); err != nil {
		return ctrl.Result{}, err
	}

	if err := utils.TriggerManagedHubAddons(ctx, r.Client, r.addonMgr); err != nil {
		return ctrl.Result{}, err
	}

	if controllerutil.AddFinalizer(mgh, constants.GlobalHubCleanupFinalizer) {
		if err = r.Client.Update(ctx, mgh, &client.UpdateOptions{}); err != nil {
			if errors.IsConflict(err) {
				r.log.Info("conflict when adding finalizer to mgh instance", "error", err)
				return ctrl.Result{Requeue: true}, nil
			}
		}
	}
	return ctrl.Result{}, nil
}

var mghPred = predicate.Funcs{
	CreateFunc: func(e event.CreateEvent) bool {
		return true
	},
	UpdateFunc: func(e event.UpdateEvent) bool {
		return e.ObjectOld.GetResourceVersion() != e.ObjectNew.GetResourceVersion()
	},
	DeleteFunc: func(e event.DeleteEvent) bool {
		return !e.DeleteStateUnknown
	},
}

var ownPred = predicate.Funcs{
	CreateFunc: func(e event.CreateEvent) bool {
		return false
	},
	UpdateFunc: func(e event.UpdateEvent) bool {
		// only requeue when spec change, if the resource do not have spec field, the generation is always 0
		if e.ObjectNew.GetGeneration() == 0 {
			return true
		}
		return e.ObjectOld.GetGeneration() != e.ObjectNew.GetGeneration()
	},
	DeleteFunc: func(e event.DeleteEvent) bool {
		return true
	},
}

var resPred = predicate.Funcs{
	CreateFunc: func(e event.CreateEvent) bool {
		return false
	},
	UpdateFunc: func(e event.UpdateEvent) bool {
		if e.ObjectNew.GetLabels()[constants.GlobalHubOwnerLabelKey] !=
			constants.GHOperatorOwnerLabelVal {
			return false
		}
		// only requeue when spec change, if the resource do not have spec field, the generation is always 0
		if e.ObjectNew.GetGeneration() == 0 {
			return true
		}
		return e.ObjectOld.GetGeneration() != e.ObjectNew.GetGeneration()
	},
	DeleteFunc: func(e event.DeleteEvent) bool {
		return e.Object.GetLabels()[constants.GlobalHubOwnerLabelKey] ==
			constants.GHOperatorOwnerLabelVal
	},
}

var secretCond = func(obj client.Object) bool {
	if WatchedSecret.Has(obj.GetName()) {
		return true
	}
	if obj.GetLabels()["strimzi.io/cluster"] == protocol.KafkaClusterName &&
		obj.GetLabels()["strimzi.io/kind"] == "KafkaUser" {
		return true
	}
	return false
}

var secretPred = predicate.Funcs{
	CreateFunc: func(e event.CreateEvent) bool {
		return secretCond(e.Object)
	},
	UpdateFunc: func(e event.UpdateEvent) bool {
		return secretCond(e.ObjectNew)
	},
	DeleteFunc: func(e event.DeleteEvent) bool {
		return secretCond(e.Object)
	},
}

var deletePred = predicate.Funcs{
	CreateFunc: func(e event.CreateEvent) bool {
		return false
	},
	UpdateFunc: func(e event.UpdateEvent) bool {
		return false
	},
	DeleteFunc: func(e event.DeleteEvent) bool {
		return true
	},
}

var configmappred = predicate.Funcs{
	CreateFunc: func(e event.CreateEvent) bool {
		return WatchedConfigMap.Has(e.Object.GetName())
	},
	UpdateFunc: func(e event.UpdateEvent) bool {
		if e.ObjectNew.GetLabels()[constants.GlobalHubOwnerLabelKey] ==
			constants.GHOperatorOwnerLabelVal {
			return true
		}
		return WatchedConfigMap.Has(e.ObjectNew.GetName())
	},
	DeleteFunc: func(e event.DeleteEvent) bool {
		if e.Object.GetLabels()[constants.GlobalHubOwnerLabelKey] ==
			constants.GHOperatorOwnerLabelVal {
			return true
		}
		return WatchedConfigMap.Has(e.Object.GetName())
	},
}

var mhPred = predicate.Funcs{
	CreateFunc: func(e event.CreateEvent) bool {
		return true
	},
	UpdateFunc: func(e event.UpdateEvent) bool {
		return true
	},
	DeleteFunc: func(e event.DeleteEvent) bool {
		return false
	},
}

var webhookPred = predicate.Funcs{
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
				return true
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

var namespacePred = predicate.Funcs{
	CreateFunc: func(e event.CreateEvent) bool {
		return e.Object.GetName() == config.GetMGHNamespacedName().Namespace
	},
	UpdateFunc: func(e event.UpdateEvent) bool {
		return e.ObjectNew.GetName() == config.GetMGHNamespacedName().Namespace &&
			e.ObjectNew.GetGeneration() != e.ObjectOld.GetGeneration()
	},
	DeleteFunc: func(e event.DeleteEvent) bool {
		return false
	},
}

var globalHubEventHandler = handler.EnqueueRequestsFromMapFunc(
	func(ctx context.Context, obj client.Object) []reconcile.Request {
		return []reconcile.Request{
			// trigger MGH instance reconcile
			{NamespacedName: config.GetMGHNamespacedName()},
		}
	},
)

// SetupWithManager sets up the controller with the Manager.
func (r *GlobalHubController) SetupWithManager(mgr ctrl.Manager) error {
	return ctrl.NewControllerManagedBy(mgr).
		For(&v1alpha4.MulticlusterGlobalHub{}, builder.WithPredicates(mghPred)).
		Owns(&appsv1.Deployment{}, builder.WithPredicates(ownPred)).
		Owns(&appsv1.StatefulSet{}, builder.WithPredicates(ownPred)).
		Owns(&corev1.Service{}, builder.WithPredicates(ownPred)).
		Owns(&corev1.ServiceAccount{}, builder.WithPredicates(ownPred)).
		Owns(&corev1.Secret{}, builder.WithPredicates(ownPred)).
		Owns(&rbacv1.Role{}, builder.WithPredicates(ownPred)).
		Owns(&rbacv1.RoleBinding{}, builder.WithPredicates(ownPred)).
		Owns(&routev1.Route{}, builder.WithPredicates(ownPred)).
		Watches(&admissionregistrationv1.MutatingWebhookConfiguration{},
			globalHubEventHandler, builder.WithPredicates(webhookPred)).
		// secondary watch for configmap
		Watches(&corev1.ConfigMap{},
			globalHubEventHandler, builder.WithPredicates(configmappred)).
		// secondary watch for namespace
		Watches(&corev1.Namespace{},
			globalHubEventHandler, builder.WithPredicates(namespacePred)).
		// secondary watch for clusterrole
		Watches(&rbacv1.ClusterRole{},
			globalHubEventHandler, builder.WithPredicates(resPred)).
		// secondary watch for clusterrolebinding
		Watches(&rbacv1.ClusterRoleBinding{},
			globalHubEventHandler, builder.WithPredicates(resPred)).
		// secondary watch for Secret
		Watches(&corev1.Secret{},
			globalHubEventHandler, builder.WithPredicates(secretPred)).
		// secondary watch for clustermanagementaddon
		Watches(&v1alpha1.ClusterManagementAddOn{},
			globalHubEventHandler,
			builder.WithPredicates(resPred)).
		Watches(&clusterv1.ManagedCluster{},
			globalHubEventHandler,
			builder.WithPredicates(mhPred)).
		Watches(&promv1.ServiceMonitor{},
			globalHubEventHandler,
			builder.WithPredicates(resPred)).
		Watches(&subv1alpha1.Subscription{},
			globalHubEventHandler,
			builder.WithPredicates(deletePred)).
		Complete(r)
}

// ReconcileMiddleware creates the kafka and postgres if needed.
// 1. create the kafka and postgres subscription at the same time
// 2. then create the kafka and postgres resources at the same time
// 3. wait for kafka and postgres ready
func (r *GlobalHubController) ReconcileMiddleware(ctx context.Context, mgh *v1alpha4.MulticlusterGlobalHub,
) error {
	// initialize postgres and kafka at the same time
	var wg sync.WaitGroup

	errorChan := make(chan error, 2)
	// initialize transport
	wg.Add(1)
	go func() {
		defer wg.Done()
		err := r.transportReconciler.Reconcile(ctx, mgh)
		if err != nil {
			errorChan <- err
			return
		}
	}()

	wg.Add(1)
	go func() {
		defer wg.Done()
		err := r.storageReconciler.Reconcile(ctx, mgh)
		if err != nil {
			errorChan <- err
			return
		}
	}()

	go func() {
		wg.Wait()
		close(errorChan)
	}()

	for err := range errorChan {
		if err != nil {
			return fmt.Errorf("middleware not ready, Error: %v", err)
		}
	}
	return nil
}

var WatchedSecret = sets.NewString(
	constants.GHTransportSecretName,
	constants.GHStorageSecretName,
	constants.GHBuiltInStorageSecretName,
	config.PostgresCertName,
	constants.CustomGrafanaIniName,
	config.GetImagePullSecretName(),
	protocol.DefaultGlobalHubKafkaUser,
)

var WatchedConfigMap = sets.NewString(
	constants.PostgresCAConfigMap,
	constants.CustomAlertName,
)
