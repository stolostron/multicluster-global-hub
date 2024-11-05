/*
Copyright 2024.

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

package controllers

import (
	"context"
	"fmt"
	"time"

	imagev1client "github.com/openshift/client-go/image/clientset/versioned/typed/image/v1"
	promv1 "github.com/prometheus-operator/prometheus-operator/pkg/apis/monitoring/v1"
	admissionregistrationv1 "k8s.io/api/admissionregistration/v1"
	rbacv1 "k8s.io/api/rbac/v1"
	"k8s.io/apimachinery/pkg/api/equality"
	"k8s.io/apimachinery/pkg/api/errors"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/types"
	"k8s.io/apimachinery/pkg/util/sets"
	"k8s.io/client-go/kubernetes"
	"k8s.io/client-go/util/retry"
	"k8s.io/klog"
	clusterv1 "open-cluster-management.io/api/cluster/v1"
	ctrl "sigs.k8s.io/controller-runtime"
	"sigs.k8s.io/controller-runtime/pkg/builder"
	"sigs.k8s.io/controller-runtime/pkg/client"
	"sigs.k8s.io/controller-runtime/pkg/controller/controllerutil"
	"sigs.k8s.io/controller-runtime/pkg/manager"

	"github.com/stolostron/multicluster-global-hub/operator/api/operator/v1alpha4"
	"github.com/stolostron/multicluster-global-hub/operator/pkg/config"
	operatorconstants "github.com/stolostron/multicluster-global-hub/operator/pkg/constants"
	"github.com/stolostron/multicluster-global-hub/operator/pkg/controllers/grafana"
	"github.com/stolostron/multicluster-global-hub/operator/pkg/controllers/inventory"
	globalhubmanager "github.com/stolostron/multicluster-global-hub/operator/pkg/controllers/manager"
	"github.com/stolostron/multicluster-global-hub/operator/pkg/controllers/storage"
	"github.com/stolostron/multicluster-global-hub/operator/pkg/controllers/transporter"
	"github.com/stolostron/multicluster-global-hub/operator/pkg/utils"
	"github.com/stolostron/multicluster-global-hub/pkg/constants"
	"github.com/stolostron/multicluster-global-hub/pkg/jobs"
	commonutils "github.com/stolostron/multicluster-global-hub/pkg/utils"
)

// IsResourceRemoved is used for controller which do not need to cleanup resources
func IsResourceRemoved() bool {
	return true
}

// controllerFuncList store all the controllers that need started and isResourecesRemoved
var controllerFuncList = []config.ControllerFunc{
	{
		StartController:   transporter.StartController,
		IsResourceRemoved: transporter.IsResourceRemoved,
	},
	{
		StartController:   globalhubmanager.StartController,
		IsResourceRemoved: IsResourceRemoved,
	},
	{
		StartController:   storage.StartController,
		IsResourceRemoved: storage.IsResourceRemoved,
	},
	{
		StartController:   grafana.StartController,
		IsResourceRemoved: IsResourceRemoved,
	},
	{
		StartController:   inventory.StartController,
		IsResourceRemoved: IsResourceRemoved,
	},
	{
		StartController:   globalhubmanager.StartController,
		IsResourceRemoved: IsResourceRemoved,
	},
}

type MetaController struct {
	client         client.Client
	kubeClient     kubernetes.Interface
	imageClient    *imagev1client.ImageV1Client
	mgr            manager.Manager
	operatorConfig *config.OperatorConfig
	upgraded       bool
}

// +kubebuilder:rbac:groups=operator.open-cluster-management.io,resources=multiclusterglobalhubs,verbs=get;list;watch;create;update;patch;delete
// +kubebuilder:rbac:groups=operator.open-cluster-management.io,resources=multiclusterglobalhubs/status,verbs=get;update;patch
// +kubebuilder:rbac:groups=operator.open-cluster-management.io,resources=multiclusterglobalhubs/finalizers,verbs=update
// +kubebuilder:rbac:groups="",resources=namespaces,verbs=get;list;watch;create;update;delete
// +kubebuilder:rbac:groups="",resources=services,verbs=get;list;watch;create;update;delete
// +kubebuilder:rbac:groups="",resources=serviceaccounts,verbs=get;list;watch;create;update;delete
// +kubebuilder:rbac:groups="",resources=secrets,verbs=get;list;watch;create;update;delete;patch
// +kubebuilder:rbac:groups="",resources=configmaps,verbs=get;list;watch;create;update;delete
// +kubebuilder:rbac:groups="apps",resources=deployments,verbs=get;list;watch;create;update;delete
// +kubebuilder:rbac:groups="rbac.authorization.k8s.io",resources=roles,verbs=get;list;watch;create;update;delete
// +kubebuilder:rbac:groups="rbac.authorization.k8s.io",resources=rolebindings,verbs=get;list;watch;create;update;delete
// +kubebuilder:rbac:groups="rbac.authorization.k8s.io",resources=clusterroles,verbs=get;list;watch;create;update;delete
// +kubebuilder:rbac:groups="rbac.authorization.k8s.io",resources=clusterrolebindings,verbs=get;list;watch;create;update;delete
func (r *MetaController) Reconcile(ctx context.Context, req ctrl.Request) (ctrl.Result, error) {
	// Check if mgh exist or deleting
	mgh, err := config.GetMulticlusterGlobalHub(ctx, r.client)
	if err != nil {
		return ctrl.Result{}, err
	}

	if mgh == nil {
		return ctrl.Result{}, nil
	}
	if config.IsPaused(mgh) {
		klog.Info("mgh controller is paused, nothing more to do")
		return ctrl.Result{}, nil
	}
	if mgh.DeletionTimestamp != nil {
		klog.V(2).Info("mgh instance is deleting")

		err = config.UpdateCondition(ctx, r.client, types.NamespacedName{
			Namespace: mgh.Namespace,
			Name:      mgh.Name,
		}, metav1.Condition{
			Type:    config.CONDITION_TYPE_GLOBALHUB_READY,
			Status:  config.CONDITION_STATUS_FALSE,
			Reason:  config.CONDITION_REASON_GLOBALHUB_UNINSTALL,
			Message: config.CONDITION_MESSAGE_GLOBALHUB_UNINSTALL,
		}, v1alpha4.GlobalHubUninstalling)
		_, err = r.pruneGlobalHubResources(ctx)
		if err != nil {
			return ctrl.Result{}, err
		}

		for _, controllerFunc := range controllerFuncList {
			removed := controllerFunc.IsResourceRemoved()
			if !removed {
				return ctrl.Result{RequeueAfter: 5 * time.Second}, nil
			}
		}
		mgh.SetFinalizers(utils.Remove(mgh.GetFinalizers(), constants.GlobalHubCleanupFinalizer))
		if err := utils.UpdateObject(ctx, r.client, mgh); err != nil {
			return ctrl.Result{}, err
		}
		return ctrl.Result{}, nil
	}

	var reconcileErr error
	defer func() {
		err = updateMGHReadyStatus(ctx, r.client, mgh, reconcileErr)
		if err != nil {
			klog.Errorf("failed to update the instance condition, err: %v", err)
		}
	}()

	controllerOption := config.ControllerOption{
		KubeClient:            r.kubeClient,
		Ctx:                   ctx,
		OperatorConfig:        r.operatorConfig,
		Manager:               r.mgr,
		MulticlusterGlobalHub: mgh,
	}
	reconcileErr = config.SetMulticlusterGlobalHubConfig(ctx, mgh, r.client, r.imageClient)
	if reconcileErr != nil {
		return ctrl.Result{}, reconcileErr
	}

	if controllerutil.AddFinalizer(mgh, constants.GlobalHubCleanupFinalizer) {
		if reconcileErr := r.client.Update(ctx, mgh, &client.UpdateOptions{}); reconcileErr != nil {
			if errors.IsConflict(reconcileErr) {
				klog.Errorf("conflict when adding finalizer to mgh instance, error: %v", reconcileErr)
				return ctrl.Result{Requeue: true}, nil
			}
			return ctrl.Result{}, err
		}
	}

	for _, controllerFunc := range controllerFuncList {
		reconcileErr = controllerFunc.StartController(controllerOption)
		if reconcileErr != nil {
			return ctrl.Result{}, err
		}
	}
	if config.IsBYOKafka() && config.IsBYOPostgres() && mgh.Spec.EnableMetrics {
		mgh.Spec.EnableMetrics = false
		klog.Info("Kafka and Postgres are provided by customer, disable metrics")
		if reconcileErr := r.client.Update(ctx, mgh, &client.UpdateOptions{}); reconcileErr != nil {
			if errors.IsConflict(reconcileErr) {
				klog.Errorf("conflict when adding finalizer to mgh instance, error: %v", reconcileErr)
				return ctrl.Result{Requeue: true}, nil
			}
			return ctrl.Result{}, err
		}
	}
	if config.IsACMResourceReady() {
		if config.GetAddonManager() != nil {
			if reconcileErr = utils.TriggerManagedHubAddons(ctx, r.client, config.GetAddonManager()); reconcileErr != nil {
				return ctrl.Result{}, reconcileErr
			}
		}
	}
	return ctrl.Result{}, nil
}

func NewMetaController(mgr manager.Manager, kubeClient kubernetes.Interface,
	operatorConfig *config.OperatorConfig, imageClient *imagev1client.ImageV1Client,
) *MetaController {
	r := &MetaController{
		client:         mgr.GetClient(),
		mgr:            mgr,
		kubeClient:     kubeClient,
		operatorConfig: operatorConfig,
		imageClient:    imageClient,
	}
	return r
}

// SetupWithManager sets up the controller with the Manager.
func (r *MetaController) SetupWithManager(mgr ctrl.Manager) error {
	return ctrl.NewControllerManagedBy(mgr).Named("MetaController").
		For(&v1alpha4.MulticlusterGlobalHub{},
			builder.WithPredicates(config.MGHPred)).
		Complete(r)
}

func updateMGHReadyStatus(ctx context.Context,
	c client.Client, mgh *v1alpha4.MulticlusterGlobalHub, reconcileErr error,
) error {
	if reconcileErr != nil {
		return config.UpdateCondition(ctx, c, types.NamespacedName{
			Namespace: mgh.Namespace,
			Name:      mgh.Name,
		}, metav1.Condition{
			Type:    config.CONDITION_TYPE_GLOBALHUB_READY,
			Status:  config.CONDITION_STATUS_FALSE,
			Reason:  config.CONDITION_REASON_GLOBALHUB_NOT_READY,
			Message: reconcileErr.Error(),
		}, v1alpha4.GlobalHubError)
	}
	return retry.RetryOnConflict(retry.DefaultBackoff, func() error {
		curmgh := &v1alpha4.MulticlusterGlobalHub{}

		err := c.Get(ctx, types.NamespacedName{
			Namespace: mgh.GetNamespace(),
			Name:      mgh.GetName(),
		}, curmgh)
		if err != nil {
			return err
		}

		// update phase
		updatedPhase, desiredPhase := needUpdatePhase(curmgh)

		// update ready condition
		updatedReadyCond, desiredConds := updateReadyConditions(mgh.Status.Conditions, desiredPhase)

		if !updatedPhase && !updatedReadyCond {
			return nil
		}

		curmgh.Status.Phase = desiredPhase
		curmgh.Status.Conditions = desiredConds
		err = c.Status().Update(ctx, curmgh)
		return err
	})
}

func updateReadyConditions(conds []metav1.Condition, phase v1alpha4.GlobalHubPhaseType) (bool, []metav1.Condition) {
	if phase == v1alpha4.GlobalHubRunning {
		return config.NeedUpdateConditions(conds, metav1.Condition{
			Type:    config.CONDITION_TYPE_GLOBALHUB_READY,
			Status:  config.CONDITION_STATUS_TRUE,
			Reason:  config.CONDITION_REASON_GLOBALHUB_READY,
			Message: config.CONDITION_MESSAGE_GLOBALHUB_READY,
		})
	}
	return config.NeedUpdateConditions(conds, metav1.Condition{
		Type:    config.CONDITION_TYPE_GLOBALHUB_READY,
		Status:  config.CONDITION_STATUS_FALSE,
		Reason:  config.CONDITION_REASON_GLOBALHUB_NOT_READY,
		Message: config.CONDITION_MESSAGE_GLOBALHUB_NOT_READY,
	})
}

// needUpdatePhase check if the phase need updated. phase is running only when all the components available
func needUpdatePhase(mgh *v1alpha4.MulticlusterGlobalHub) (bool, v1alpha4.GlobalHubPhaseType) {
	phase := v1alpha4.GlobalHubRunning
	desiredComponents := CheckDesiredComponent(mgh)
	if len(mgh.Status.Components) != desiredComponents.Len() {
		phase = v1alpha4.GlobalHubProgressing
		return phase != mgh.Status.Phase, phase
	}
	for _, dcs := range mgh.Status.Components {
		if !desiredComponents.Has(dcs.Name) {
			phase = v1alpha4.GlobalHubProgressing
		}
		if dcs.Type == config.COMPONENTS_AVAILABLE {
			if dcs.Status == config.RECONCILE_ERROR {
				phase = v1alpha4.GlobalHubError
			}
			if dcs.Status != config.CONDITION_STATUS_TRUE {
				phase = v1alpha4.GlobalHubProgressing
			}
		}
	}
	return phase != mgh.Status.Phase, phase
}

func CheckDesiredComponent(mgh *v1alpha4.MulticlusterGlobalHub) sets.String {
	desiredComponents := sets.NewString(
		config.COMPONENTS_MANAGER_NAME,
		config.COMPONENTS_POSTGRES_NAME,
		config.COMPONENTS_KAFKA_NAME,
	)

	if config.IsACMResourceReady() {
		desiredComponents.Insert(config.COMPONENTS_GRAFANA_NAME)
	}
	if config.WithInventory(mgh) {
		desiredComponents.Insert(config.COMPONENTS_INVENTORY_API_NAME)
	}
	return desiredComponents
}

func (r *MetaController) pruneGlobalHubResources(ctx context.Context,
) (ctrl.Result, error) {
	if config.IsACMResourceReady() {
		if err := r.pruneManagedHubs(ctx); err != nil {
			return ctrl.Result{}, fmt.Errorf("failed to delete annotation from the managed cluster: %w", err)
		}
	}

	// clean up namesapced resources, eg. mgh system namespace, etc
	if err := r.pruneNamespacedResources(ctx); err != nil {
		return ctrl.Result{}, err
	}

	// clean up the cluster resources, eg. clusterrole, clusterrolebinding, etc
	if err := r.pruneGlobalResources(ctx); err != nil {
		return ctrl.Result{}, err
	}

	if config.IsACMResourceReady() {
		// remove finalizer from app, policy and placement.
		// the finalizer is added by the global hub manager. ideally, they should be pruned by manager
		// But currently, we do not have a channel from operator to let manager knows when to start pruning.
		if err := jobs.NewPruneFinalizer(ctx, r.client).Run(); err != nil {
			return ctrl.Result{}, err
		}
		klog.Info("removed finalizer from mgh, app, policy, placement and etc")
	}

	return ctrl.Result{}, nil
}

// pruneGlobalResources deletes the cluster scoped resources created by the multicluster-global-hub-operator
// cluster scoped resources need to be deleted manually because they don't have ownerrefenence set
func (r *MetaController) pruneGlobalResources(ctx context.Context) error {
	listOpts := []client.ListOption{
		client.MatchingLabels(map[string]string{
			constants.GlobalHubOwnerLabelKey: constants.GHOperatorOwnerLabelVal,
		}),
	}

	clusterRoleList := &rbacv1.ClusterRoleList{}
	if err := r.client.List(ctx, clusterRoleList, listOpts...); err != nil {
		return err
	}
	for idx := range clusterRoleList.Items {
		if err := r.client.Delete(ctx, &clusterRoleList.Items[idx]); err != nil && !errors.IsNotFound(err) {
			return err
		}
	}

	clusterRoleBindingList := &rbacv1.ClusterRoleBindingList{}
	if err := r.client.List(ctx, clusterRoleBindingList, listOpts...); err != nil {
		return err
	}
	for idx := range clusterRoleBindingList.Items {
		if err := r.client.Delete(ctx, &clusterRoleBindingList.Items[idx]); err != nil && !errors.IsNotFound(err) {
			return err
		}
	}

	webhookList := &admissionregistrationv1.MutatingWebhookConfigurationList{}
	if err := r.client.List(ctx, webhookList, listOpts...); err != nil {
		return err
	}
	for idx := range webhookList.Items {
		if err := r.client.Delete(ctx, &webhookList.Items[idx]); err != nil && !errors.IsNotFound(err) {
			return err
		}
	}

	return nil
}

// pruneNamespacedResources tries to delete mgh resources
func (r *MetaController) pruneNamespacedResources(ctx context.Context) error {
	mghServiceMonitor := &promv1.ServiceMonitor{
		ObjectMeta: metav1.ObjectMeta{
			Name:      operatorconstants.GHServiceMonitorName,
			Namespace: commonutils.GetDefaultNamespace(),
			Labels: map[string]string{
				constants.GlobalHubOwnerLabelKey: constants.GHOperatorOwnerLabelVal,
			},
		},
	}
	if err := r.client.Delete(ctx, mghServiceMonitor); err != nil && !errors.IsNotFound(err) {
		return err
	}

	return nil
}

func (r *MetaController) pruneManagedHubs(ctx context.Context) error {
	clusters := &clusterv1.ManagedClusterList{}
	if err := r.client.List(ctx, clusters, &client.ListOptions{}); err != nil {
		return err
	}

	for idx, managedHub := range clusters.Items {
		if managedHub.Name == constants.LocalClusterName {
			continue
		}
		orgAnnotations := managedHub.GetAnnotations()
		if orgAnnotations == nil {
			continue
		}
		annotations := make(map[string]string, len(orgAnnotations))
		utils.CopyMap(annotations, managedHub.GetAnnotations())

		delete(orgAnnotations, operatorconstants.AnnotationONMulticlusterHub)
		delete(orgAnnotations, operatorconstants.AnnotationPolicyONMulticlusterHub)
		_ = controllerutil.RemoveFinalizer(&clusters.Items[idx], constants.GlobalHubCleanupFinalizer)
		if !equality.Semantic.DeepEqual(annotations, orgAnnotations) {
			if err := r.client.Update(ctx, &clusters.Items[idx], &client.UpdateOptions{}); err != nil {
				return err
			}
		}
	}

	return nil
}
