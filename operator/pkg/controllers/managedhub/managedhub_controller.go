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

	"k8s.io/apimachinery/pkg/api/equality"
	clusterv1 "open-cluster-management.io/api/cluster/v1"
	ctrl "sigs.k8s.io/controller-runtime"
	"sigs.k8s.io/controller-runtime/pkg/builder"
	"sigs.k8s.io/controller-runtime/pkg/client"
	"sigs.k8s.io/controller-runtime/pkg/controller/controllerutil"
	"sigs.k8s.io/controller-runtime/pkg/event"
	"sigs.k8s.io/controller-runtime/pkg/handler"
	"sigs.k8s.io/controller-runtime/pkg/predicate"

	"github.com/stolostron/multicluster-global-hub/operator/api/operator/v1alpha4"
	"github.com/stolostron/multicluster-global-hub/operator/pkg/config"
	operatorconstants "github.com/stolostron/multicluster-global-hub/operator/pkg/constants"
	"github.com/stolostron/multicluster-global-hub/operator/pkg/utils"
	"github.com/stolostron/multicluster-global-hub/pkg/constants"
	"github.com/stolostron/multicluster-global-hub/pkg/logger"
)

// +kubebuilder:rbac:groups=cluster.open-cluster-management.io,resources=managedclusters,verbs=get;list;update;create;delete

type ManagedHubController struct {
	c client.Client
}

var (
	isResourceRemoved    = true
	managedHubController *ManagedHubController
)

var log = logger.DefaultZapLogger()

func (r *ManagedHubController) IsResourceRemoved() bool {
	log.Infof("managedHubController resource removed: %v", isResourceRemoved)
	return isResourceRemoved
}

func StartController(initOption config.ControllerOption) (config.ControllerInterface, error) {
	if managedHubController != nil {
		return managedHubController, nil
	}
	if !config.IsACMResourceReady() {
		return nil, nil
	}
	log.Info("start managedhub controller")

	managedHubController = NewManagedHubController(initOption.Manager)
	err := managedHubController.SetupWithManager(initOption.Manager)
	if err != nil {
		managedHubController = nil
		return nil, err
	}
	log.Infof("inited managedhub controller")
	return managedHubController, nil
}

func (r *ManagedHubController) Reconcile(ctx context.Context, req ctrl.Request) (ctrl.Result, error) {
	log.Debugf("reconcile managedhub controller")

	mgh, err := config.GetMulticlusterGlobalHub(ctx, r.c)
	if err != nil {
		return ctrl.Result{}, nil
	}
	if mgh == nil || config.IsPaused(mgh) {
		return ctrl.Result{}, nil
	}
	if mgh.DeletionTimestamp != nil {
		log.Debugf("deleting mgh in managedhub controller")
		err = utils.HandleMghDelete(ctx, &isResourceRemoved, mgh.Namespace, r.pruneManagedHubs)
		log.Debugf("deleted managedhub resources, isResourceRemoved: %v", isResourceRemoved)
		return ctrl.Result{}, err
	}
	isResourceRemoved = false
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
		For(&v1alpha4.MulticlusterGlobalHub{},
			builder.WithPredicates(config.MGHPred)).
		Watches(&clusterv1.ManagedCluster{},
			&handler.EnqueueRequestForObject{}, builder.WithPredicates(mcPred)).
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

func (r *ManagedHubController) pruneManagedHubs(ctx context.Context, namespace string) error {
	clusters := &clusterv1.ManagedClusterList{}
	if err := r.c.List(ctx, clusters, &client.ListOptions{}); err != nil {
		return err
	}

	for idx, managedHub := range clusters.Items {
		if managedHub.Labels[constants.LocalClusterName] == "true" {
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
			if err := r.c.Update(ctx, &clusters.Items[idx], &client.UpdateOptions{}); err != nil {
				return err
			}
		}
	}

	return nil
}
