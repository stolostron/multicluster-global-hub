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

package managedhub

import (
	"context"
	"time"

	"k8s.io/klog"
	clusterv1 "open-cluster-management.io/api/cluster/v1"
	ctrl "sigs.k8s.io/controller-runtime"
	"sigs.k8s.io/controller-runtime/pkg/builder"
	"sigs.k8s.io/controller-runtime/pkg/client"
	"sigs.k8s.io/controller-runtime/pkg/event"
	"sigs.k8s.io/controller-runtime/pkg/handler"
	"sigs.k8s.io/controller-runtime/pkg/predicate"

	"github.com/stolostron/multicluster-global-hub/operator/pkg/config"
	"github.com/stolostron/multicluster-global-hub/operator/pkg/utils"
)

// +kubebuilder:rbac:groups=cluster.open-cluster-management.io,resources=managedclusters,verbs=get;list;update;create;delete

type ManagedHubController struct {
	c client.Client
}

var started bool

func StartController(initOption config.ControllerOption) error {
	if started {
		return nil
	}
	if !config.IsACMResourceReady() {
		return nil
	}
	err := NewManagedHubController(initOption.Manager).SetupWithManager(initOption.Manager)
	if err != nil {
		return err
	}
	klog.Infof("inited managedhub controller")
	started = true
	return nil
}

func (r *ManagedHubController) Reconcile(ctx context.Context, req ctrl.Request) (ctrl.Result, error) {
	mgh, err := config.GetMulticlusterGlobalHub(ctx, r.c)
	if err != nil {
		return ctrl.Result{}, nil
	}
	if mgh == nil || config.IsPaused(mgh) || mgh.DeletionTimestamp != nil {
		return ctrl.Result{RequeueAfter: 5 * time.Second}, nil
	}
	var reconcileErr error
	if reconcileErr = utils.AnnotateManagedHubCluster(ctx, r.c); reconcileErr != nil {
		return ctrl.Result{}, reconcileErr
	}
	return ctrl.Result{}, nil
}

func NewManagedHubController(mgr ctrl.Manager) *ManagedHubController {
	return &ManagedHubController{c: mgr.GetClient()}
}

func (r *ManagedHubController) SetupWithManager(mgr ctrl.Manager) error {
	return ctrl.NewControllerManagedBy(mgr).
		Named("ManagedHubController").
		WatchesMetadata(
			&clusterv1.ManagedCluster{},
			&handler.EnqueueRequestForObject{},
			builder.WithPredicates(mcPred),
		).
		Complete(r)
}

var mcPred = predicate.Funcs{
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
