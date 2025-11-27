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

package acm

import (
	"context"
	"sync"

	apiextensionsv1 "k8s.io/apiextensions-apiserver/pkg/apis/apiextensions/v1"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/util/sets"
	ctrl "sigs.k8s.io/controller-runtime"
	"sigs.k8s.io/controller-runtime/pkg/builder"
	"sigs.k8s.io/controller-runtime/pkg/event"
	"sigs.k8s.io/controller-runtime/pkg/handler"
	"sigs.k8s.io/controller-runtime/pkg/manager"
	"sigs.k8s.io/controller-runtime/pkg/predicate"

	"github.com/stolostron/multicluster-global-hub/operator/api/operator/v1alpha4"
	"github.com/stolostron/multicluster-global-hub/operator/pkg/config"
	"github.com/stolostron/multicluster-global-hub/pkg/logger"
)

// +kubebuilder:rbac:groups=operator.open-cluster-management.io,resources=multiclusterglobalhubs,verbs=get;list;watch;patch;update
// +kubebuilder:rbac:groups=apiextensions.k8s.io,resources=customresourcedefinitions,verbs=get;list;watch
// +kubebuilder:rbac:groups=operator.open-cluster-management.io,resources=multiclusterhubs;clustermanagers,verbs=get;list;patch;update;watch

var ACMResources = sets.NewString(
	"multiclusterhubs.operator.open-cluster-management.io",
	"clustermanagers.operator.open-cluster-management.io",
)

var (
	log                   = logger.DefaultZapLogger()
	acmResourceController *ACMResourceController
)

type ACMResourceController struct {
	manager.Manager
	Resources   map[string]bool
	resourcesMu sync.RWMutex
}

func (r *ACMResourceController) IsResourceRemoved() bool {
	return true
}

func (r *ACMResourceController) Reconcile(ctx context.Context, req ctrl.Request) (ctrl.Result, error) {
	log.Debugf("reconcile acm controller: %v", req)

	r.resourcesMu.Lock()
	r.Resources[req.Name] = true
	r.resourcesMu.Unlock()

	if !r.readyToWatchACMResources() {
		log.Debugf("ACM Resources is not ready")
		return ctrl.Result{}, nil
	}

	config.SetACMResourceReady(true)
	err := config.UpdateCondition(ctx, r.GetClient(), config.GetMGHNamespacedName(),
		metav1.Condition{
			Type:    config.CONDITION_TYPE_ACM_RESOURCE_READY,
			Status:  config.CONDITION_STATUS_TRUE,
			Reason:  config.CONDITION_REASON_ACM_RESOURCE_READY,
			Message: config.CONDITION_MESSAGE_ACM_RESOURCE_READY,
		}, v1alpha4.GlobalHubError)
	if err != nil {
		return ctrl.Result{}, err
	}
	return ctrl.Result{}, nil
}

func (r *ACMResourceController) readyToWatchACMResources() bool {
	r.resourcesMu.RLock()
	defer r.resourcesMu.RUnlock()

	for val := range ACMResources {
		if ready := r.Resources[val]; !ready {
			return false
		}
	}
	return true
}

func StartController(opts config.ControllerOption) (config.ControllerInterface, error) {
	if acmResourceController != nil {
		return acmResourceController, nil
	}
	log.Info("start acm controller")

	acmController := &ACMResourceController{
		Manager:   opts.Manager,
		Resources: make(map[string]bool),
	}
	err := ctrl.NewControllerManagedBy(opts.Manager).Named("acm-controller").
		WatchesMetadata(
			&apiextensionsv1.CustomResourceDefinition{},
			&handler.EnqueueRequestForObject{},
			builder.WithPredicates(predicate.Funcs{
				CreateFunc: func(e event.CreateEvent) bool {
					return ACMResources.Has(e.Object.GetName())
				},
				UpdateFunc: func(e event.UpdateEvent) bool {
					return false
				},
				DeleteFunc: func(e event.DeleteEvent) bool {
					return false
				},
			}),
		).Complete(acmController)
	if err != nil {
		return nil, err
	}
	acmResourceController = acmController
	log.Infof("inited acm controller")
	return acmResourceController, nil
}
