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

package addon

import (
	"context"
	"reflect"

	"k8s.io/apimachinery/pkg/api/errors"
	"k8s.io/apimachinery/pkg/types"
	"open-cluster-management.io/api/addon/v1alpha1"
	addonv1alpha1 "open-cluster-management.io/api/addon/v1alpha1"
	clusterv1 "open-cluster-management.io/api/cluster/v1"
	ctrl "sigs.k8s.io/controller-runtime"
	"sigs.k8s.io/controller-runtime/pkg/builder"
	"sigs.k8s.io/controller-runtime/pkg/client"
	"sigs.k8s.io/controller-runtime/pkg/event"
	"sigs.k8s.io/controller-runtime/pkg/handler"
	"sigs.k8s.io/controller-runtime/pkg/predicate"
	"sigs.k8s.io/controller-runtime/pkg/reconcile"

	"github.com/stolostron/multicluster-global-hub/operator/pkg/config"
	"github.com/stolostron/multicluster-global-hub/pkg/constants"
	"github.com/stolostron/multicluster-global-hub/pkg/utils"
)

// +kubebuilder:rbac:groups=operator.open-cluster-management.io,resources=multiclusterglobalhubs,verbs=get;list;watch;
// +kubebuilder:rbac:groups=addon.open-cluster-management.io,resources=managedclusteraddons,verbs=create;delete;get;list;patch;update;watch
// +kubebuilder:rbac:groups=addon.open-cluster-management.io,resources=addondeploymentconfigs,verbs=get;list;watch
// +kubebuilder:rbac:groups=addon.open-cluster-management.io,resources=managedclusteraddons,verbs=get;list;watch

type HostedAgentController struct {
	c client.Client
}

var GlobalHubHostedAddonConfig = v1alpha1.AddOnConfig{
	ConfigReferent: v1alpha1.ConfigReferent{
		Name:      "global-hub",
		Namespace: utils.GetDefaultNamespace(),
	},
	ConfigGroupResource: v1alpha1.ConfigGroupResource{
		Group:    "addon.open-cluster-management.io",
		Resource: "addondeploymentconfigs",
	},
}

var hostedAgentController *HostedAgentController

// StartHostedAgentController watches CMA resources and updates their configuration.
// Currently, it only adds settings in hosted mode and removes them in non-hosted mode.
func StartHostedAgentController(initOption config.ControllerOption) (config.ControllerInterface, error) {
	if hostedAgentController != nil {
		return hostedAgentController, nil
	}
	if !config.IsACMResourceReady() {
		return nil, nil
	}

	log.Info("start agent(hosted mode) controller")

	if !ReadyToEnableAddonManager(initOption.MulticlusterGlobalHub) {
		return nil, nil
	}

	hostedAgentController = NewHostedAgentController(initOption.Manager)

	if err := hostedAgentController.SetupWithManager(initOption.Manager); err != nil {
		hostedAgentController = nil
		return nil, err
	}
	log.Info("initialized agent(hosted mode) controller")
	return hostedAgentController, nil
}

func (c *HostedAgentController) IsResourceRemoved() bool {
	return true
}

func NewHostedAgentController(mgr ctrl.Manager) *HostedAgentController {
	return &HostedAgentController{c: mgr.GetClient()}
}

// SetupWithManager sets up the controller with the Manager.
func (r *HostedAgentController) SetupWithManager(mgr ctrl.Manager) error {
	return ctrl.NewControllerManagedBy(mgr).Named("AddonsController").
		For(&addonv1alpha1.ManagedClusterAddOn{},
			builder.WithPredicates(predicate.Funcs{
				CreateFunc: func(e event.CreateEvent) bool {
					return config.HostedAddonList.Has(e.Object.GetName())
				},
				UpdateFunc: func(e event.UpdateEvent) bool {
					return config.HostedAddonList.Has(e.ObjectNew.GetName())
				},
				DeleteFunc: func(e event.DeleteEvent) bool {
					return false
				},
			})).
		Watches(&clusterv1.ManagedCluster{},
			handler.EnqueueRequestsFromMapFunc(func(ctx context.Context, obj client.Object) []reconcile.Request {
				var requests []reconcile.Request
				for v := range config.HostedAddonList {
					request := reconcile.Request{
						NamespacedName: types.NamespacedName{
							Namespace: obj.GetName(),
							Name:      v,
						},
					}
					requests = append(requests, request)
				}
				return requests
			}),
			builder.WithPredicates(predicate.Funcs{
				CreateFunc: func(e event.CreateEvent) bool {
					return false
				},
				UpdateFunc: func(e event.UpdateEvent) bool {
					return IfHostedLabelChange(e.ObjectNew.GetLabels(), e.ObjectOld.GetLabels())
				},
				DeleteFunc: func(e event.DeleteEvent) bool {
					return false
				},
			})).
		Complete(r)
}

func IfHostedLabelChange(newLabelMap, oldLabelMap map[string]string) bool {
	newHostedLabel := ""
	oldHostedLabel := ""

	if newLabelMap != nil {
		newHostedLabel = newLabelMap[constants.GHDeployModeLabelKey]
	}
	if oldLabelMap != nil {
		oldHostedLabel = oldLabelMap[constants.GHDeployModeLabelKey]
	}
	if newHostedLabel == oldHostedLabel {
		return false
	}
	return true
}

func (r *HostedAgentController) Reconcile(ctx context.Context, req ctrl.Request) (ctrl.Result, error) {
	log.Debugf("reconcile hosted agent: %v", req.NamespacedName)
	mca := &addonv1alpha1.ManagedClusterAddOn{}
	err := r.c.Get(ctx, req.NamespacedName, mca)
	if err != nil {
		if errors.IsNotFound(err) {
			log.Infof("ManagedClusterAddOn %s not found, skip reconcile", req.NamespacedName)
			return ctrl.Result{}, nil
		}
		return ctrl.Result{}, err
	}

	// Get mcl, check if the cluster is hosted
	mc := &clusterv1.ManagedCluster{}
	err = r.c.Get(ctx, types.NamespacedName{Name: req.NamespacedName.Namespace}, mc)
	if err != nil {
		log.Errorf("Failed to get managed cluster %s, err:%v", req.NamespacedName.Namespace, err)
		return ctrl.Result{}, err
	}

	// If the cluster is local cluster, skip the reconcile
	if mc.Labels[constants.LocalClusterName] == "true" {
		return ctrl.Result{}, nil
	}

	isHosted := false
	needUpdate := false

	if mc.Labels != nil && mc.Labels[constants.GHDeployModeLabelKey] == constants.GHDeployModeHosted {
		isHosted = true
	}
	log.Debugf("ManagedCluster %s is hosted: %v", mc.Name, isHosted)

	if isHosted {
		needUpdate = addAddonConfig(mca)
	} else {
		needUpdate = removeAddonConfig(mca)
	}
	log.Debugf("ManagedClusterAddOn %s need update: %v", req.NamespacedName, needUpdate)
	if !needUpdate {
		return ctrl.Result{}, nil
	}

	err = r.c.Update(ctx, mca)
	if err != nil {
		log.Errorf("Failed to update cma, err:%v", err)
		return ctrl.Result{}, err
	}
	return ctrl.Result{}, nil
}

func removeAddonConfig(mca *addonv1alpha1.ManagedClusterAddOn) bool {
	if len(mca.Spec.Configs) == 0 {
		return false
	}
	for i, addonConfig := range mca.Spec.Configs {
		if reflect.DeepEqual(addonConfig, GlobalHubHostedAddonConfig) {
			mca.Spec.Configs = append(mca.Spec.Configs[:i], mca.Spec.Configs[i+1:]...)
			return true
		}
	}
	return false
}

// addAddonConfig add the config to mca, will return true if the mca updated
func addAddonConfig(mca *addonv1alpha1.ManagedClusterAddOn) bool {
	if len(mca.Spec.Configs) == 0 {
		mca.Spec.Configs = append(mca.Spec.Configs,
			GlobalHubHostedAddonConfig)
		return true
	}
	for _, addonConfig := range mca.Spec.Configs {
		if reflect.DeepEqual(addonConfig, GlobalHubHostedAddonConfig) {
			return false
		}
	}
	mca.Spec.Configs = append(mca.Spec.Configs,
		GlobalHubHostedAddonConfig)
	return true
}
