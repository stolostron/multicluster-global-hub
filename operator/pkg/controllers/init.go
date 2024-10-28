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
	"k8s.io/apimachinery/pkg/api/errors"
	"k8s.io/apimachinery/pkg/api/meta"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/types"
	"k8s.io/apimachinery/pkg/util/sets"
	"k8s.io/client-go/kubernetes"
	"k8s.io/client-go/util/retry"
	"k8s.io/klog"
	ctrl "sigs.k8s.io/controller-runtime"
	"sigs.k8s.io/controller-runtime/pkg/builder"
	"sigs.k8s.io/controller-runtime/pkg/client"
	"sigs.k8s.io/controller-runtime/pkg/controller/controllerutil"
	"sigs.k8s.io/controller-runtime/pkg/event"
	"sigs.k8s.io/controller-runtime/pkg/manager"
	"sigs.k8s.io/controller-runtime/pkg/predicate"

	"github.com/stolostron/multicluster-global-hub/operator/api/operator/v1alpha4"
	"github.com/stolostron/multicluster-global-hub/operator/pkg/config"
	"github.com/stolostron/multicluster-global-hub/operator/pkg/controllers/grafana"
	"github.com/stolostron/multicluster-global-hub/operator/pkg/controllers/inventory"
	globalhubmanager "github.com/stolostron/multicluster-global-hub/operator/pkg/controllers/manager"
	"github.com/stolostron/multicluster-global-hub/operator/pkg/controllers/storage"
	"github.com/stolostron/multicluster-global-hub/operator/pkg/controllers/transporter"
	"github.com/stolostron/multicluster-global-hub/operator/pkg/utils"
	"github.com/stolostron/multicluster-global-hub/pkg/constants"
	commonutils "github.com/stolostron/multicluster-global-hub/pkg/utils"
)

const (
	transportController = "transportController"
	storageController   = "storageController"
	managerController   = "managerController"
	grafanaController   = "grafanaController"
	inventoryController = "inventoryController"
)

type Func func(initOption config.ControllerOption) (bool, error)

type MetaController struct {
	client            client.Client
	kubeClient        kubernetes.Interface
	imageClient       *imagev1client.ImageV1Client
	mgr               manager.Manager
	initedControllers sets.String
	initFuncMap       map[string]Func
	operatorConfig    *config.OperatorConfig
	upgraded          bool
}

func (r *MetaController) Reconcile(ctx context.Context, req ctrl.Request) (ctrl.Result, error) {
	// Check if mgh exist or deleting
	mgh, err := config.GetMulticlusterGlobalHub(ctx, r.client)
	if err != nil || mgh == nil {
		return ctrl.Result{RequeueAfter: 5 * time.Second}, nil
	}

	if config.IsPaused(mgh) {
		klog.Info("mgh controller is paused, nothing more to do")
		return ctrl.Result{}, nil
	}

	controllerOption := config.ControllerOption{
		KubeClient:            r.kubeClient,
		Ctx:                   ctx,
		OperatorConfig:        r.operatorConfig,
		IsGlobalhubReady:      meta.IsStatusConditionTrue(mgh.Status.Conditions, config.CONDITION_TYPE_GLOBALHUB_READY),
		Manager:               r.mgr,
		MulticlusterGlobalHub: mgh,
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

		return ctrl.Result{}, err
	}

	var reconcileErr error
	defer func() {
		err = updateMghStatus(ctx, r.client, mgh, reconcileErr)
		if err != nil {
			klog.Errorf("failed to update the instance condition, err: %v", err)
		}
	}()

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
		}
	}

	for controllerName, startController := range r.initFuncMap {
		controllerOption.ControllerName = controllerName
		if r.initedControllers.Has(controllerName) {
			continue
		}
		var started bool
		started, reconcileErr = startController(controllerOption)
		if reconcileErr != nil {
			klog.Errorf("failed to init %v, err: %v", controllerName, err)
			return ctrl.Result{}, err
		}
		if started {
			r.initedControllers.Insert(controllerOption.ControllerName)
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
		client:            mgr.GetClient(),
		mgr:               mgr,
		initedControllers: sets.NewString(),
		kubeClient:        kubeClient,
		operatorConfig:    operatorConfig,
		imageClient:       imageClient,
	}
	r.initFuncMap = map[string]Func{
		transportController: transporter.StartController,
		storageController:   storage.StartController,
		managerController:   globalhubmanager.StartController,
		grafanaController:   grafana.StartController,
		inventoryController: inventory.StartController,
	}
	return r
}

// SetupWithManager sets up the controller with the Manager.
func (r *MetaController) SetupWithManager(mgr ctrl.Manager) error {
	return ctrl.NewControllerManagedBy(mgr).Named("MetaController").
		For(&v1alpha4.MulticlusterGlobalHub{},
			builder.WithPredicates(mghPred)).
		Complete(r)
}

var mghPred = predicate.Funcs{
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

func updateMghStatus(ctx context.Context,
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

		// update data retention condition
		updatedRetentionCond, desiredConds := updateRetentionConditions(curmgh)

		// update ready condition
		updatedReadyCond, desiredConds := updateReadyConditions(mgh.Status.Conditions, desiredPhase)

		if !updatedPhase && !updatedReadyCond && !updatedRetentionCond {
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

// dataRetention should at least be 1 month, otherwise it will deleted the current month partitions and records
func updateRetentionConditions(mgh *v1alpha4.MulticlusterGlobalHub) (bool, []metav1.Condition) {
	months, err := commonutils.ParseRetentionMonth(mgh.Spec.DataLayerSpec.Postgres.Retention)
	if err != nil {
		err = fmt.Errorf("failed to parse the retention month, err:%v", err)
		return config.NeedUpdateConditions(mgh.Status.Conditions, metav1.Condition{
			Type:    config.CONDITION_TYPE_DATABASE,
			Status:  config.CONDITION_STATUS_FALSE,
			Reason:  config.CONDITION_REASON_RETENTION_PARSED_FAILED,
			Message: err.Error(),
		})
	}

	if months < 1 {
		months = 1
	}
	msg := fmt.Sprintf("The data will be kept in the database for %d months.", months)
	return config.NeedUpdateConditions(mgh.Status.Conditions, metav1.Condition{
		Type:    config.CONDITION_TYPE_DATABASE,
		Status:  config.CONDITION_STATUS_TRUE,
		Reason:  config.CONDITION_REASON_RETENTION_PARSED,
		Message: msg,
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
