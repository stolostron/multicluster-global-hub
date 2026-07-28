/*
Copyright (c) 2026 Red Hat, Inc.
Copyright Contributors to the Open Cluster Management project

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

package hubha

import (
	"context"
	"fmt"
	"strings"

	apierrors "k8s.io/apimachinery/pkg/api/errors"
	"k8s.io/apimachinery/pkg/apis/meta/v1/unstructured"
	"k8s.io/apimachinery/pkg/runtime/schema"
	"k8s.io/client-go/util/retry"
	clusterv1 "open-cluster-management.io/api/cluster/v1"
	ctrl "sigs.k8s.io/controller-runtime"
	"sigs.k8s.io/controller-runtime/pkg/client"
	"sigs.k8s.io/controller-runtime/pkg/event"
	"sigs.k8s.io/controller-runtime/pkg/predicate"

	"github.com/stolostron/multicluster-global-hub/agent/pkg/configs"
	"github.com/stolostron/multicluster-global-hub/pkg/constants"
)

var klusterletConfigGVK = schema.GroupVersionKind{
	Group:   "config.open-cluster-management.io",
	Version: "v1alpha1",
	Kind:    "KlusterletConfig",
}

// haConfigAnnotator annotates ManagedClusters created after the initial HAConfig sync.
type haConfigAnnotator struct {
	client client.Client
}

// AddHAConfigAnnotator registers a controller that applies (or clears) the HA
// klusterlet-config annotation on ManagedCluster create/update.
func AddHAConfigAnnotator(mgr ctrl.Manager) error {
	r := &haConfigAnnotator{client: mgr.GetClient()}
	return ctrl.NewControllerManagedBy(mgr).
		Named("ha-config-annotator").
		For(&clusterv1.ManagedCluster{}).
		WithEventFilter(predicate.Funcs{
			CreateFunc: func(e event.CreateEvent) bool {
				if shouldSkipHAAnnotation(e.Object) {
					return hasHAKlusterletConfigAnnotation(e.Object)
				}
				return true
			},
			UpdateFunc: func(e event.UpdateEvent) bool {
				if shouldSkipHAAnnotation(e.ObjectNew) {
					return hasHAKlusterletConfigAnnotation(e.ObjectNew)
				}
				oldAnn := e.ObjectOld.GetAnnotations()[klusterletConfigAnnotation]
				newAnn := e.ObjectNew.GetAnnotations()[klusterletConfigAnnotation]
				return oldAnn != newAnn || newAnn == ""
			},
			DeleteFunc: func(e event.DeleteEvent) bool {
				return false
			},
		}).
		Complete(r)
}

func (r *haConfigAnnotator) Reconcile(ctx context.Context, req ctrl.Request) (ctrl.Result, error) {
	mc := &clusterv1.ManagedCluster{}
	if err := r.client.Get(ctx, req.NamespacedName, mc); err != nil {
		if apierrors.IsNotFound(err) {
			return ctrl.Result{}, nil
		}
		return ctrl.Result{}, fmt.Errorf("failed to get ManagedCluster %s: %w", req.Name, err)
	}

	if shouldSkipHAAnnotation(mc) {
		if err := clearHAKlusterletConfigAnnotation(ctx, r.client, mc.Name); err != nil {
			return ctrl.Result{}, err
		}
		return ctrl.Result{}, nil
	}

	// Annotate spokes only on the active hub.
	if !isActiveHub() {
		return ctrl.Result{}, nil
	}

	klusterletConfigName, err := findHAKlusterletConfigName(ctx, r.client)
	if err != nil {
		return ctrl.Result{}, fmt.Errorf("failed to find HA KlusterletConfig: %w", err)
	}
	if klusterletConfigName == "" {
		return ctrl.Result{}, nil
	}

	if err := annotateManagedCluster(ctx, r.client, mc, klusterletConfigName); err != nil {
		return ctrl.Result{}, err
	}
	return ctrl.Result{}, nil
}

func isActiveHub() bool {
	cfg := configs.GetAgentConfig()
	return cfg != nil && cfg.GetHubRole() == constants.GHHubRoleActive
}

// shouldSkipHAAnnotation is true for local-cluster and hubs imported into Global Hub.
func shouldSkipHAAnnotation(obj client.Object) bool {
	if isLocalManagedCluster(obj) {
		return true
	}
	return isGlobalHubManagedHub(obj)
}

func hasHAKlusterletConfigAnnotation(obj client.Object) bool {
	return obj.GetAnnotations()[klusterletConfigAnnotation] != ""
}

func isLocalManagedCluster(obj client.Object) bool {
	if obj.GetLabels()[constants.LocalClusterName] == "true" {
		return true
	}
	return obj.GetName() == constants.LocalClusterName
}

func isGlobalHubManagedHub(obj client.Object) bool {
	labels := obj.GetLabels()
	if labels == nil {
		return false
	}
	if _, ok := labels[constants.GHDeployModeLabelKey]; ok {
		return true
	}
	if _, ok := labels[constants.GHHubRoleLabelKey]; ok {
		return true
	}
	return false
}

func findHAKlusterletConfigName(ctx context.Context, c client.Client) (string, error) {
	list := &unstructured.UnstructuredList{}
	list.SetGroupVersionKind(schema.GroupVersionKind{
		Group:   klusterletConfigGVK.Group,
		Version: klusterletConfigGVK.Version,
		Kind:    klusterletConfigGVK.Kind + "List",
	})
	if err := c.List(ctx, list); err != nil {
		return "", err
	}
	for i := range list.Items {
		name := list.Items[i].GetName()
		if strings.HasPrefix(name, klusterletConfigPrefix) {
			return name, nil
		}
	}
	return "", nil
}

func annotateManagedCluster(ctx context.Context, c client.Client,
	mc *clusterv1.ManagedCluster, klusterletConfigName string,
) error {
	return retry.RetryOnConflict(retry.DefaultRetry, func() error {
		current := &clusterv1.ManagedCluster{}
		if err := c.Get(ctx, client.ObjectKeyFromObject(mc), current); err != nil {
			return fmt.Errorf("failed to get ManagedCluster %s: %w", mc.Name, err)
		}
		if shouldSkipHAAnnotation(current) {
			return clearHAAnnotationLocked(ctx, c, current)
		}
		annotations := current.GetAnnotations()
		if annotations == nil {
			annotations = make(map[string]string)
		}
		if annotations[klusterletConfigAnnotation] == klusterletConfigName {
			return nil
		}
		annotations[klusterletConfigAnnotation] = klusterletConfigName
		current.SetAnnotations(annotations)
		if err := c.Update(ctx, current); err != nil {
			return fmt.Errorf("failed to update ManagedCluster %s with klusterlet-config annotation: %w",
				current.Name, err)
		}
		log.Infow("annotated managed cluster",
			"cluster", current.Name, "klusterlet-config", klusterletConfigName)
		return nil
	})
}

func clearHAKlusterletConfigAnnotation(ctx context.Context, c client.Client, name string) error {
	return retry.RetryOnConflict(retry.DefaultRetry, func() error {
		current := &clusterv1.ManagedCluster{}
		if err := c.Get(ctx, client.ObjectKey{Name: name}, current); err != nil {
			return fmt.Errorf("failed to get ManagedCluster %s: %w", name, err)
		}
		return clearHAAnnotationLocked(ctx, c, current)
	})
}

func clearHAAnnotationLocked(ctx context.Context, c client.Client, current *clusterv1.ManagedCluster) error {
	annotations := current.GetAnnotations()
	if annotations == nil || annotations[klusterletConfigAnnotation] == "" {
		return nil
	}
	delete(annotations, klusterletConfigAnnotation)
	current.SetAnnotations(annotations)
	if err := c.Update(ctx, current); err != nil {
		return fmt.Errorf("failed to clear klusterlet-config annotation on ManagedCluster %s: %w",
			current.Name, err)
	}
	log.Infow("removed klusterlet-config annotation", "cluster", current.Name)
	return nil
}
