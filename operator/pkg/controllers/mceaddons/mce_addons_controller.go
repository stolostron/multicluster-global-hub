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

package mceaddons

import (
	"context"
	"reflect"
	"time"

	"k8s.io/apimachinery/pkg/api/errors"
	"k8s.io/apimachinery/pkg/api/meta"
	"k8s.io/apimachinery/pkg/types"
	addonv1alpha1 "open-cluster-management.io/api/addon/v1alpha1"
	ctrl "sigs.k8s.io/controller-runtime"
	"sigs.k8s.io/controller-runtime/pkg/builder"
	"sigs.k8s.io/controller-runtime/pkg/client"
	"sigs.k8s.io/controller-runtime/pkg/event"
	"sigs.k8s.io/controller-runtime/pkg/handler"
	"sigs.k8s.io/controller-runtime/pkg/predicate"
	"sigs.k8s.io/controller-runtime/pkg/reconcile"

	globalhubv1alpha4 "github.com/stolostron/multicluster-global-hub/operator/api/operator/v1alpha4"
	"github.com/stolostron/multicluster-global-hub/operator/pkg/config"
	"github.com/stolostron/multicluster-global-hub/pkg/logger"
)

// +kubebuilder:rbac:groups=operator.open-cluster-management.io,resources=multiclusterglobalhubs,verbs=get;list;watch;
// +kubebuilder:rbac:groups=addon.open-cluster-management.io,resources=clustermanagementaddons,verbs=create;delete;get;list;patch;update;watch
// +kubebuilder:rbac:groups=addon.open-cluster-management.io,resources=addondeploymentconfigs,verbs=get;list;watch

// MceAddonsController reconciles the MCE related addon(work-manager/managedservice-account/cluster-proxy) ClusterManagementAddOn resources
// It will add the placement strategy to the ClusterManagementAddOn resources, and select hosted related clusters and
// make sure the addon is installed on the <open-cluster-management-global-hub-agent-addon> namespace.
type MceAddonsController struct {
	c client.Client
}

var log = logger.DefaultZapLogger()

var mceAddonsController *MceAddonsController

func StartMceAddonsController(initOption config.ControllerOption) (config.ControllerInterface, error) {
	if mceAddonsController != nil {
		return mceAddonsController, nil
	}
	log.Info("start mce addons controller")

	if !config.IsACMResourceReady() {
		return nil, nil
	}
	if !meta.IsStatusConditionTrue(initOption.MulticlusterGlobalHub.Status.Conditions, config.CONDITION_TYPE_GLOBALHUB_READY) {
		return nil, nil
	}

	mceAddonsController = NewMceAddonsController(initOption.Manager)

	if err := mceAddonsController.SetupWithManager(initOption.Manager); err != nil {
		mceAddonsController = nil
		return nil, err
	}
	log.Info("inited mce addons controller")
	return mceAddonsController, nil
}

func (c *MceAddonsController) IsResourceRemoved() bool {
	return true
}

func NewMceAddonsController(mgr ctrl.Manager) *MceAddonsController {
	return &MceAddonsController{c: mgr.GetClient()}
}

// SetupWithManager sets up the controller with the Manager.
func (r *MceAddonsController) SetupWithManager(mgr ctrl.Manager) error {
	return ctrl.NewControllerManagedBy(mgr).Named("AddonsController").
		For(&addonv1alpha1.ClusterManagementAddOn{},
			builder.WithPredicates(addonPred)).
		// requeue all cma when mgh updated
		Watches(&globalhubv1alpha4.MulticlusterGlobalHub{},
			handler.EnqueueRequestsFromMapFunc(func(ctx context.Context, obj client.Object) []reconcile.Request {
				var requests []reconcile.Request
				for v := range config.HostedAddonList {
					request := reconcile.Request{
						NamespacedName: types.NamespacedName{
							Name: v,
						},
					}
					requests = append(requests, request)
				}
				return requests
			}), builder.WithPredicates(mghPred)).
		Complete(r)
}

var addonPred = predicate.Funcs{
	CreateFunc: func(e event.CreateEvent) bool {
		return config.HostedAddonList.Has(e.Object.GetName())
	},
	UpdateFunc: func(e event.UpdateEvent) bool {
		return config.HostedAddonList.Has(e.ObjectNew.GetName())
	},
	DeleteFunc: func(e event.DeleteEvent) bool {
		return false
	},
}

var mghPred = predicate.Funcs{
	CreateFunc: func(e event.CreateEvent) bool {
		return true
	},
	UpdateFunc: func(e event.UpdateEvent) bool {
		return e.ObjectNew.GetDeletionTimestamp() != nil
	},
	DeleteFunc: func(e event.DeleteEvent) bool {
		return false
	},
}

func (r *MceAddonsController) Reconcile(ctx context.Context, req ctrl.Request) (ctrl.Result, error) {
	log.Debugf("reconcile ClusterManagementAddOn: %v", req.NamespacedName)
	mgh, err := config.GetMulticlusterGlobalHub(ctx, r.c)
	if err != nil {
		log.Error(err)
		return ctrl.Result{RequeueAfter: 5 * time.Second}, nil
	}
	if mgh == nil || config.IsPaused(mgh) {
		return ctrl.Result{}, nil
	}

	cma := &addonv1alpha1.ClusterManagementAddOn{}
	err = r.c.Get(ctx, req.NamespacedName, cma)
	if err != nil {
		if errors.IsNotFound(err) {
			log.Infof("ClusterManagementAddOn %s not found, skip reconcile", req.NamespacedName)
			return ctrl.Result{}, nil
		}
		log.Errorf("Failed to get ClusterManagementAddOn %s, err: %v", req.NamespacedName, err)
		return ctrl.Result{}, err
	}
	if mgh.DeletionTimestamp != nil {
		needUpdate := removeAddonConfig(cma)
		if !needUpdate {
			return ctrl.Result{}, nil
		}
	} else {
		needUpdate := addAddonConfig(cma)
		if !needUpdate {
			return ctrl.Result{}, nil
		}
	}
	log.Infof("Update ClusterManagementAddOn %s", req.NamespacedName)
	err = r.c.Update(ctx, cma)
	if err != nil {
		log.Errorf("Failed to update cma, err:%v", err)
		return ctrl.Result{}, err
	}
	return ctrl.Result{}, nil
}

// addAddonConfig add the config to cma, will return true if the cma updated
func addAddonConfig(cma *addonv1alpha1.ClusterManagementAddOn) bool {
	if len(cma.Spec.InstallStrategy.Placements) == 0 {
		cma.Spec.InstallStrategy.Placements = append(cma.Spec.InstallStrategy.Placements,
			config.GlobalHubHostedAddonPlacementStrategy)
		return true
	}
	for _, pl := range cma.Spec.InstallStrategy.Placements {
		if isGlobalhubPlaceStrategy(pl, config.GlobalHubHostedAddonPlacementStrategy) {
			return false
		}
	}
	cma.Spec.InstallStrategy.Placements = append(cma.Spec.InstallStrategy.Placements,
		config.GlobalHubHostedAddonPlacementStrategy)
	return true
}

func isGlobalhubPlaceStrategy(ps, globalPs addonv1alpha1.PlacementStrategy) bool {
	if ps.Name != globalPs.Name || ps.Namespace != globalPs.Namespace {
		return false
	}

	if !reflect.DeepEqual(ps.Configs, globalPs.Configs) {
		return false
	}
	log.Debugf("found placement, placement strategy %s/%s is globalhub placement strategy",
		ps.Namespace, ps.Name)
	return true
}

func removeAddonConfig(cma *addonv1alpha1.ClusterManagementAddOn) bool {
	var newPlacements []addonv1alpha1.PlacementStrategy
	updated := false
	for _, pl := range cma.Spec.InstallStrategy.Placements {
		if !isGlobalhubPlaceStrategy(pl, config.GlobalHubHostedAddonPlacementStrategy) {
			newPlacements = append(newPlacements, pl)
		} else {
			updated = true
		}
	}
	if updated {
		cma.Spec.InstallStrategy.Placements = newPlacements
	}
	return updated
}
