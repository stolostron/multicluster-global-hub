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

package hubmanagement

import (
	"context"
	"fmt"
	"time"

	apierrors "k8s.io/apimachinery/pkg/api/errors"
	addonv1alpha1 "open-cluster-management.io/api/addon/v1alpha1"
	ctrl "sigs.k8s.io/controller-runtime"
	"sigs.k8s.io/controller-runtime/pkg/client"
	"sigs.k8s.io/controller-runtime/pkg/predicate"

	"github.com/stolostron/multicluster-global-hub/pkg/constants"
	"github.com/stolostron/multicluster-global-hub/pkg/database/models"
	"github.com/stolostron/multicluster-global-hub/pkg/logger"
)

var log = logger.DefaultZapLogger()

type managerClusterAddonController struct {
	client client.Client
}

// ManagedClusterAddonController is to mark the agent inactive once the addon is deleted
func AddManagedClusterAddonController(mgr ctrl.Manager) error {
	addonCtrl := &managerClusterAddonController{
		client: mgr.GetClient(),
	}
	configMapPredicate := predicate.NewPredicateFuncs(func(object client.Object) bool {
		return object.GetName() == constants.GHManagedClusterAddonName
	})
	return ctrl.NewControllerManagedBy(mgr).
		For(&addonv1alpha1.ManagedClusterAddOn{}).
		WithEventFilter(configMapPredicate).
		Complete(addonCtrl)
}

func (c *managerClusterAddonController) Reconcile(ctx context.Context, request ctrl.Request) (ctrl.Result, error) {
	if hubStatusManager == nil {
		log.Warn("The hub management process should be started")
		return ctrl.Result{Requeue: true, RequeueAfter: 5 * time.Second}, nil
	}

	addon := &addonv1alpha1.ManagedClusterAddOn{}
	if err := c.client.Get(ctx, request.NamespacedName, addon); apierrors.IsNotFound(err) {
		if request.Namespace == "" {
			return ctrl.Result{}, nil
		}

		log.Infof("inactive the agent when the global hub addon(%s) is deleted", request.Namespace)
		err := hubStatusManager.inactive(ctx, []models.LeafHubHeartbeat{{
			Name: request.Namespace,
		}})
		return ctrl.Result{}, err
	} else if err != nil {
		return ctrl.Result{Requeue: true, RequeueAfter: 5 * time.Second}, fmt.Errorf("failed to get addon: %w", err)
	}

	if !addon.DeletionTimestamp.IsZero() {
		log.Infof("inactive the agent when the global hub addon(%s) is deleting", addon.Namespace)
		err := hubStatusManager.inactive(ctx, []models.LeafHubHeartbeat{{
			Name: request.Namespace,
		}})
		if err != nil {
			return ctrl.Result{}, err
		}
	}

	return ctrl.Result{}, nil
}
