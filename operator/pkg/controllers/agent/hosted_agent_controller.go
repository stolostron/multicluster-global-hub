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

package agent

import (
	"context"
	"reflect"
	"time"

	"k8s.io/apimachinery/pkg/types"
	"k8s.io/klog"
	"open-cluster-management.io/api/addon/v1alpha1"
	ctrl "sigs.k8s.io/controller-runtime"
	"sigs.k8s.io/controller-runtime/pkg/builder"
	"sigs.k8s.io/controller-runtime/pkg/client"
	"sigs.k8s.io/controller-runtime/pkg/event"
	"sigs.k8s.io/controller-runtime/pkg/handler"
	"sigs.k8s.io/controller-runtime/pkg/manager"
	"sigs.k8s.io/controller-runtime/pkg/predicate"
	"sigs.k8s.io/controller-runtime/pkg/reconcile"

	globalhubv1alpha4 "github.com/stolostron/multicluster-global-hub/operator/api/operator/v1alpha4"
	"github.com/stolostron/multicluster-global-hub/operator/pkg/config"
	operatorconstants "github.com/stolostron/multicluster-global-hub/operator/pkg/constants"
)

type HostedAgentController struct {
	manager.Manager
	client.Client
}

var hostedAddonController *HostedAgentController

func AddHostedAgentController(mgr manager.Manager) (*HostedAgentController, error) {
	if hostedAddonController != nil {
		return hostedAddonController, nil
	}
	r := &HostedAgentController{
		Manager: mgr,
		Client:  mgr.GetClient(),
	}
	if err := r.SetupWithManager(mgr); err != nil {
		return nil, err
	}
	hostedAddonController = r
	return hostedAddonController, nil
}

// SetupWithManager sets up the controller with the Manager.
func (r *HostedAgentController) SetupWithManager(mgr ctrl.Manager) error {
	return ctrl.NewControllerManagedBy(mgr).Named("AddonsController").
		For(&v1alpha1.ClusterManagementAddOn{},
			builder.WithPredicates(addonPred)).
		// requeue all cma when mgh annotation changed.
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
		if reflect.DeepEqual(e.ObjectNew.GetAnnotations(), e.ObjectOld.GetAnnotations()) {
			return false
		}
		if e.ObjectNew.GetAnnotations()[operatorconstants.AnnotationImportClusterInHosted] !=
			e.ObjectOld.GetAnnotations()[operatorconstants.AnnotationImportClusterInHosted] {
			return true
		}
		return false
	},
	DeleteFunc: func(e event.DeleteEvent) bool {
		return false
	},
}

func (r *HostedAgentController) Reconcile(ctx context.Context, req ctrl.Request) (ctrl.Result, error) {
	klog.V(2).Infof("Reconcile ClusterManagementAddOn: %v", req.NamespacedName)
	if !config.GetImportClusterInHosted() {
		return ctrl.Result{}, nil
	}
	mgh, err := config.GetMulticlusterGlobalHub(ctx, r.Client)
	if err != nil {
		klog.Error("error ", err)
		return ctrl.Result{RequeueAfter: 5 * time.Second}, nil
	}
	if mgh == nil || config.IsPaused(mgh) || mgh.DeletionTimestamp != nil {
		return ctrl.Result{}, nil
	}
	cma := &v1alpha1.ClusterManagementAddOn{}
	err = r.Client.Get(ctx, req.NamespacedName, cma)
	if err != nil {
		return ctrl.Result{}, err
	}

	needUpdate := addAddonConfig(cma)
	if !needUpdate {
		return ctrl.Result{}, nil
	}

	err = r.Client.Update(ctx, cma)
	if err != nil {
		klog.Errorf("Failed to update cma, err:%v", err)
		return ctrl.Result{}, err
	}
	return ctrl.Result{}, nil
}

// addAddonConfig add the config to cma, will return true if the cma updated
func addAddonConfig(cma *v1alpha1.ClusterManagementAddOn) bool {
	if len(cma.Spec.InstallStrategy.Placements) == 0 {
		cma.Spec.InstallStrategy.Placements = append(cma.Spec.InstallStrategy.Placements,
			config.GlobalHubHostedAddonPlacementStrategy)
		return true
	}
	for i, pl := range cma.Spec.InstallStrategy.Placements {
		if !reflect.DeepEqual(pl.PlacementRef, config.GlobalHubHostedAddonPlacementStrategy.PlacementRef) {
			continue
		}
		if reflect.DeepEqual(pl.Configs, config.GlobalHubHostedAddonPlacementStrategy.Configs) {
			return false
		}
		cma.Spec.InstallStrategy.Placements[i].Configs = append(pl.Configs,
			config.GlobalHubHostedAddonPlacementStrategy.Configs...)
		return true
	}
	cma.Spec.InstallStrategy.Placements = append(cma.Spec.InstallStrategy.Placements,
		config.GlobalHubHostedAddonPlacementStrategy)
	return true
}
