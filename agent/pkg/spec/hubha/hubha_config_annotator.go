// Copyright (c) 2025 Red Hat, Inc.
// Copyright Contributors to the Open Cluster Management project

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

	"github.com/stolostron/multicluster-global-hub/pkg/constants"
)

var klusterletConfigGVK = schema.GroupVersionKind{
	Group:   "config.open-cluster-management.io",
	Version: "v1alpha1",
	Kind:    "KlusterletConfig",
}

// haConfigAnnotator ensures ManagedClusters created after the initial HAConfig
// sync still receive agent.open-cluster-management.io/klusterlet-config.
// HAConfigSyncer only annotates clusters present when the CloudEvent arrives.
type haConfigAnnotator struct {
	client client.Client
}

// AddHAConfigAnnotator watches ManagedCluster creates/updates and applies the
// HA KlusterletConfig annotation when an ha-standby-* KlusterletConfig exists.
func AddHAConfigAnnotator(mgr ctrl.Manager) error {
	r := &haConfigAnnotator{client: mgr.GetClient()}
	return ctrl.NewControllerManagedBy(mgr).
		Named("ha-config-annotator").
		For(&clusterv1.ManagedCluster{}).
		WithEventFilter(predicate.Funcs{
			CreateFunc: func(e event.CreateEvent) bool {
				return !isLocalManagedCluster(e.Object)
			},
			UpdateFunc: func(e event.UpdateEvent) bool {
				if isLocalManagedCluster(e.ObjectNew) {
					return false
				}
				// Reconcile when the annotation is missing or changed.
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

// Reconcile annotates a ManagedCluster with the HA KlusterletConfig name when
// Hub HA has already been configured on this regional hub.
func (r *haConfigAnnotator) Reconcile(ctx context.Context, req ctrl.Request) (ctrl.Result, error) {
	mc := &clusterv1.ManagedCluster{}
	if err := r.client.Get(ctx, req.NamespacedName, mc); err != nil {
		if apierrors.IsNotFound(err) {
			return ctrl.Result{}, nil
		}
		return ctrl.Result{}, fmt.Errorf("failed to get ManagedCluster %s: %w", req.Name, err)
	}

	if isLocalManagedCluster(mc) {
		return ctrl.Result{}, nil
	}

	klusterletConfigName, err := findHAKlusterletConfigName(ctx, r.client)
	if err != nil {
		return ctrl.Result{}, fmt.Errorf("failed to find HA KlusterletConfig: %w", err)
	}
	if klusterletConfigName == "" {
		// HA config not applied yet; nothing to do until Sync creates it.
		return ctrl.Result{}, nil
	}

	if err := annotateManagedCluster(ctx, r.client, mc, klusterletConfigName); err != nil {
		return ctrl.Result{}, err
	}
	return ctrl.Result{}, nil
}

// isLocalManagedCluster returns true for the hub's self-managed local cluster,
// identified by the local-cluster=true label or the conventional name.
func isLocalManagedCluster(obj client.Object) bool {
	if obj.GetLabels()[constants.LocalClusterName] == "true" {
		return true
	}
	return obj.GetName() == constants.LocalClusterName
}

// findHAKlusterletConfigName returns the name of the first KlusterletConfig with
// the ha-standby- prefix, or "" if none exists.
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

// annotateManagedCluster sets the klusterlet-config annotation on mc when it
// is missing or differs from klusterletConfigName.
func annotateManagedCluster(ctx context.Context, c client.Client,
	mc *clusterv1.ManagedCluster, klusterletConfigName string,
) error {
	return retry.RetryOnConflict(retry.DefaultRetry, func() error {
		current := &clusterv1.ManagedCluster{}
		if err := c.Get(ctx, client.ObjectKeyFromObject(mc), current); err != nil {
			return err
		}
		// Re-check after reload: another controller may have marked this as local-cluster.
		if isLocalManagedCluster(current) {
			return nil
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
			return err
		}
		log.Infof("annotated managed cluster %s with klusterlet-config=%s",
			current.Name, klusterletConfigName)
		return nil
	})
}
