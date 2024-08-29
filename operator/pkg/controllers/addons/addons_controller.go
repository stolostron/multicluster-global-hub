/*
Copyright 2023.

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

package addons

import (
	"context"
	"reflect"

	"k8s.io/apimachinery/pkg/util/sets"
	"k8s.io/klog"
	"open-cluster-management.io/api/addon/v1alpha1"
	ctrl "sigs.k8s.io/controller-runtime"
	"sigs.k8s.io/controller-runtime/pkg/builder"
	"sigs.k8s.io/controller-runtime/pkg/client"
	"sigs.k8s.io/controller-runtime/pkg/event"
	"sigs.k8s.io/controller-runtime/pkg/manager"
	"sigs.k8s.io/controller-runtime/pkg/predicate"

	"github.com/stolostron/multicluster-global-hub/operator/pkg/config"
	"github.com/stolostron/multicluster-global-hub/pkg/constants"
)

var addonList = sets.NewString(
	"work-manager",
	"cluster-proxy",
	"managed-serviceaccount",
)

var newNamespaceConfig = v1alpha1.PlacementStrategy{
	PlacementRef: v1alpha1.PlacementRef{
		Namespace: "open-cluster-management-global-set",
		Name:      "global",
	},
	Configs: []v1alpha1.AddOnConfig{
		{
			ConfigReferent: v1alpha1.ConfigReferent{
				Name:      "global-hub",
				Namespace: constants.GHDefaultNamespace,
			},
			ConfigGroupResource: v1alpha1.ConfigGroupResource{
				Group:    "addon.open-cluster-management.io",
				Resource: "addondeploymentconfigs",
			},
		},
	},
}

// BackupReconciler reconciles a MulticlusterGlobalHub object
type AddonsReconciler struct {
	manager.Manager
	client.Client
}

func NewAddonsReconciler(mgr manager.Manager) *AddonsReconciler {
	return &AddonsReconciler{
		Manager: mgr,
		Client:  mgr.GetClient(),
	}
}

// SetupWithManager sets up the controller with the Manager.
func (r *AddonsReconciler) SetupWithManager(mgr ctrl.Manager) error {
	return ctrl.NewControllerManagedBy(mgr).Named("AddonsController").
		For(&v1alpha1.ClusterManagementAddOn{},
			builder.WithPredicates(addonPred)).
		Complete(r)
}

var addonPred = predicate.Funcs{
	CreateFunc: func(e event.CreateEvent) bool {
		return addonList.Has(e.Object.GetName())
	},
	UpdateFunc: func(e event.UpdateEvent) bool {
		return addonList.Has(e.ObjectNew.GetName())
	},
	DeleteFunc: func(e event.DeleteEvent) bool {
		return false
	},
}

func (r *AddonsReconciler) Reconcile(ctx context.Context, req ctrl.Request) (ctrl.Result, error) {
	klog.V(2).Infof("Reconcile ClusterManagementAddOn: %v", req.NamespacedName)
	if !config.GetImportClusterInHosted() {
		return ctrl.Result{}, nil
	}
	cma := &v1alpha1.ClusterManagementAddOn{}
	err := r.Client.Get(ctx, req.NamespacedName, cma)
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
		cma.Spec.InstallStrategy.Placements = append(cma.Spec.InstallStrategy.Placements, newNamespaceConfig)
		return true
	}
	for _, pl := range cma.Spec.InstallStrategy.Placements {
		if !reflect.DeepEqual(pl.PlacementRef, newNamespaceConfig.PlacementRef) {
			continue
		}
		if reflect.DeepEqual(pl.Configs, newNamespaceConfig.Configs) {
			return false
		}
		pl.Configs = append(pl.Configs, newNamespaceConfig.Configs...)
		return true
	}
	cma.Spec.InstallStrategy.Placements = append(cma.Spec.InstallStrategy.Placements, newNamespaceConfig)
	return true
}
