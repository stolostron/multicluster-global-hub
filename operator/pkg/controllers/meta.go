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
	"time"

	imagev1client "github.com/openshift/client-go/image/clientset/versioned/typed/image/v1"
	rbacv1 "k8s.io/api/rbac/v1"
	"k8s.io/apimachinery/pkg/api/errors"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/types"
	"k8s.io/apimachinery/pkg/util/sets"
	"k8s.io/client-go/kubernetes"
	"k8s.io/client-go/util/retry"
	ctrl "sigs.k8s.io/controller-runtime"
	"sigs.k8s.io/controller-runtime/pkg/builder"
	"sigs.k8s.io/controller-runtime/pkg/client"
	"sigs.k8s.io/controller-runtime/pkg/controller/controllerutil"
	"sigs.k8s.io/controller-runtime/pkg/handler"
	"sigs.k8s.io/controller-runtime/pkg/manager"

	"github.com/stolostron/multicluster-global-hub/operator/api/operator/v1alpha1"
	"github.com/stolostron/multicluster-global-hub/operator/api/operator/v1alpha4"
	"github.com/stolostron/multicluster-global-hub/operator/pkg/config"
	"github.com/stolostron/multicluster-global-hub/operator/pkg/controllers/acm"
	"github.com/stolostron/multicluster-global-hub/operator/pkg/controllers/agent"
	addonagent "github.com/stolostron/multicluster-global-hub/operator/pkg/controllers/agent/addon"
	"github.com/stolostron/multicluster-global-hub/operator/pkg/controllers/backup"
	"github.com/stolostron/multicluster-global-hub/operator/pkg/controllers/grafana"
	"github.com/stolostron/multicluster-global-hub/operator/pkg/controllers/inventory"
	"github.com/stolostron/multicluster-global-hub/operator/pkg/controllers/managedhub"
	globalhubmanager "github.com/stolostron/multicluster-global-hub/operator/pkg/controllers/manager"
	"github.com/stolostron/multicluster-global-hub/operator/pkg/controllers/storage"
	"github.com/stolostron/multicluster-global-hub/operator/pkg/controllers/transporter"
	"github.com/stolostron/multicluster-global-hub/operator/pkg/controllers/webhook"
	"github.com/stolostron/multicluster-global-hub/operator/pkg/utils"
	"github.com/stolostron/multicluster-global-hub/pkg/constants"
	"github.com/stolostron/multicluster-global-hub/pkg/jobs"
	"github.com/stolostron/multicluster-global-hub/pkg/logger"
)

type Func func(initOption config.ControllerOption) (config.ControllerInterface, error)

// controllerStartMap store all the controllers that need started
var controllerStartFuncMap = map[string]Func{
	"globalhubManager": globalhubmanager.StartController,
	"grafana":          grafana.StartController,
	"defaultAgent":     addonagent.StartDefaultAgentController,
	"hostedAgent":      addonagent.StartHostedAgentController,
	"addonManager":     addonagent.StartAddonManagerController,
	"localAgent":       agent.StartLocalAgentController,
	"webhook":          webhook.StartController,
	"storage":          storage.StartController,
	"transporter":      transporter.StartController,
	"managedhub":       managedhub.StartController,
	"acm":              acm.StartController,
	"backup":           backup.StartController,
	"postgresUser":     storage.StartPostgresConfigUserController,
	"inventory":        inventory.StartInventoryController,
	"spicedb":          inventory.StartSpiceDBReconciler,
}

var log = logger.DefaultZapLogger()

type MetaController struct {
	client               client.Client
	kubeClient           kubernetes.Interface
	imageClient          *imagev1client.ImageV1Client
	mgr                  manager.Manager
	operatorConfig       *config.OperatorConfig
	startedControllerMap map[string]config.ControllerInterface
}

// +kubebuilder:rbac:groups=operator.open-cluster-management.io,resources=multiclusterglobalhubs;multiclusterglobalhubagents,verbs=get;list;watch;create;update;patch;delete
// +kubebuilder:rbac:groups=operator.open-cluster-management.io,resources=multiclusterglobalhubs/status,verbs=get;update;patch
// +kubebuilder:rbac:groups=operator.open-cluster-management.io,resources=multiclusterglobalhubs/finalizers,verbs=update
// +kubebuilder:rbac:groups="",resources=namespaces,verbs=get;list;watch;create;update;delete
// +kubebuilder:rbac:groups="",resources=services,verbs=get;list;watch;create;update;delete
// +kubebuilder:rbac:groups="",resources=serviceaccounts,verbs=get;list;watch;create;update;delete
// +kubebuilder:rbac:groups="",resources=secrets,verbs=get;list;watch;create;update;delete;patch
// +kubebuilder:rbac:groups="",resources=configmaps,verbs=get;list;watch;create;update;delete
// +kubebuilder:rbac:groups="apps",resources=deployments,verbs=get;list;watch;create;update;delete
// +kubebuilder:rbac:groups="rbac.authorization.k8s.io",resources=roles;rolebindings;clusterroles;clusterrolebindings,verbs=get;list;watch;create;update;delete

func (r *MetaController) Reconcile(ctx context.Context, req ctrl.Request) (ctrl.Result, error) {
	// Check if mgh exist or deleting
	mgh, err := config.GetMulticlusterGlobalHub(ctx, r.client)
	if err != nil {
		return ctrl.Result{}, err
	}

	mgha, err := config.GetMulticlusterGlobalHubAgent(ctx, r.client)
	if err != nil {
		return ctrl.Result{}, err
	}
	if mgha != nil {
		// deploy global hub agent
		if err := agent.StartStandaloneAgentController(ctx, r.mgr); err != nil {
			return ctrl.Result{}, err
		}
		return ctrl.Result{}, nil
	}

	if mgh == nil {
		return ctrl.Result{}, nil
	}
	if config.IsPaused(mgh) {
		log.Info("mgh controller is paused, nothing more to do")
		return ctrl.Result{}, nil
	}
	if mgh.DeletionTimestamp != nil {
		log.Debug("mgh instance is deleting")
		err = config.UpdateCondition(ctx, r.client, types.NamespacedName{
			Namespace: mgh.Namespace,
			Name:      mgh.Name,
		}, metav1.Condition{
			Type:    config.CONDITION_TYPE_GLOBALHUB_READY,
			Status:  config.CONDITION_STATUS_FALSE,
			Reason:  config.CONDITION_REASON_GLOBALHUB_UNINSTALL,
			Message: config.CONDITION_MESSAGE_GLOBALHUB_UNINSTALL,
		}, v1alpha4.GlobalHubUninstalling)
		if err != nil {
			return ctrl.Result{}, err
		}

		requeue, err := r.pruneGlobalHubResources(ctx, mgh)
		if err != nil {
			return ctrl.Result{}, err
		}
		if requeue {
			return ctrl.Result{RequeueAfter: 5 * time.Second}, nil
		}
		for name, c := range r.startedControllerMap {
			removed := c.IsResourceRemoved()
			log.Debugf("removed resources in controller: %v, removed:%v", name, removed)
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
			log.Errorf("failed to update the instance condition, err: %v", err)
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
				log.Errorf("conflict when adding finalizer to mgh instance, error: %v", reconcileErr)
				return ctrl.Result{Requeue: true}, nil
			}
		}
	}

	for controllerName, startController := range controllerStartFuncMap {
		var startedController config.ControllerInterface
		startedController, reconcileErr = startController(controllerOption)
		if reconcileErr == nil && startedController != nil {
			log.Debugf("started controller:%v", controllerName)
			r.startedControllerMap[controllerName] = startedController
		}
	}
	log.Debugf("started controller:%v", r.startedControllerMap)

	if reconcileErr != nil {
		return ctrl.Result{}, err
	}
	if config.IsACMResourceReady() && config.GetAddonManager() != nil {
		if reconcileErr = utils.TriggerManagedHubAddons(ctx, r.client, config.GetAddonManager()); reconcileErr != nil {
			return ctrl.Result{}, reconcileErr
		}
	}

	if config.IsBYOKafka() && config.IsBYOPostgres() && mgh.Spec.EnableMetrics {
		mgh.Spec.EnableMetrics = false
		log.Info("Kafka and Postgres are provided by customer, disable metrics")
		if reconcileErr := r.client.Update(ctx, mgh, &client.UpdateOptions{}); reconcileErr != nil {
			if errors.IsConflict(reconcileErr) {
				log.Errorf("conflict when adding finalizer to mgh instance, error: %v", reconcileErr)
				return ctrl.Result{Requeue: true}, nil
			}
			return ctrl.Result{}, err
		}
	}
	return ctrl.Result{}, nil
}

func NewMetaController(mgr manager.Manager, kubeClient kubernetes.Interface,
	operatorConfig *config.OperatorConfig, imageClient *imagev1client.ImageV1Client,
) *MetaController {
	r := &MetaController{
		client:               mgr.GetClient(),
		mgr:                  mgr,
		kubeClient:           kubeClient,
		operatorConfig:       operatorConfig,
		imageClient:          imageClient,
		startedControllerMap: make(map[string]config.ControllerInterface),
	}
	return r
}

// SetupWithManager sets up the controller with the Manager.
func (r *MetaController) SetupWithManager(mgr ctrl.Manager) error {
	return ctrl.NewControllerManagedBy(mgr).Named("MetaController").
		For(&v1alpha4.MulticlusterGlobalHub{},
			builder.WithPredicates(config.MGHPred)).
		Watches(&v1alpha1.MulticlusterGlobalHubAgent{},
			&handler.EnqueueRequestForObject{}).
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
	componentNum := len(mgh.Status.Components)
	if componentNum < desiredComponents.Len() {
		log.Debugf("processing components: expected at least %d, but got %d", desiredComponents.Len(), componentNum)
		phase = v1alpha4.GlobalHubProgressing
		return phase != mgh.Status.Phase, phase
	}

	// Deprecated: The built-in PostgreSQL name `multicluster-global-hub-postgres` has been changed to
	// `multicluster-global-hub-postgresql` starting from Global Hub release 2.13.
	// If the new instance is not ready, we should keep the phase as progressing.
	newPgInstance := false
	for _, dcs := range mgh.Status.Components {
		// ingore the status of the old pg instance before upgrade
		if dcs.Name == "multicluster-global-hub-postgres" {
			continue
		}
		if dcs.Name == "multicluster-global-hub-postgresql" {
			newPgInstance = true
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
	if !newPgInstance && phase != v1alpha4.GlobalHubError {
		log.Info("upgrading pg component to 'multicluster-global-hub-postgresql'")
		phase = v1alpha4.GlobalHubProgressing
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

func (r *MetaController) pruneGlobalHubResources(ctx context.Context, mgh *v1alpha4.MulticlusterGlobalHub,
) (bool, error) {
	// clean up the cluster resources, eg. clusterrole, clusterrolebinding, etc
	if err := r.pruneGlobalResources(ctx); err != nil {
		return false, err
	}

	if config.IsACMResourceReady() {
		// remove finalizer from app, policy and placement.
		// the finalizer is added by the global hub manager. ideally, they should be pruned by manager
		// But currently, we do not have a channel from operator to let manager knows when to start pruning.
		if err := jobs.NewPruneFinalizer(ctx, r.client).Run(); err != nil {
			return false, err
		}
		log.Info("removed finalizer from mgh, app, policy, placement and etc")
	}

	return false, nil
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

	return nil
}
