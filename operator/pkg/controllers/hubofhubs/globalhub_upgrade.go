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

package hubofhubs

import (
	"context"

	clusterv1 "open-cluster-management.io/api/cluster/v1"
	"sigs.k8s.io/controller-runtime/pkg/client"
	"sigs.k8s.io/controller-runtime/pkg/controller/controllerutil"

	"github.com/stolostron/multicluster-global-hub/operator/pkg/constants"
	commonconstants "github.com/stolostron/multicluster-global-hub/pkg/constants"
)

func (r *MulticlusterGlobalHubReconciler) Upgrade(ctx context.Context) error {
	clusters := &clusterv1.ManagedClusterList{}
	if err := r.List(ctx, clusters, &client.ListOptions{}); err != nil {
		return err
	}

	for idx := range clusters.Items {
		managedHub := &clusters.Items[idx]
		if managedHub.Name == constants.LocalClusterName {
			continue
		}

		if ok := controllerutil.RemoveFinalizer(managedHub, commonconstants.GlobalHubCleanupFinalizer); ok {
			r.Log.Info("remove finalizer from cluster", "cluster", managedHub.GetName(), "finalizers", managedHub.Finalizers)
			if err := r.Update(ctx, managedHub, &client.UpdateOptions{}); err != nil {
				return err
			}
		}
	}

	return nil
}
