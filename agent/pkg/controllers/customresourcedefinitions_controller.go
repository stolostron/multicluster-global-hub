// Copyright (c) 2022 Red Hat, Inc.
// Copyright Contributors to the Open Cluster Management project

package controllers

import (
	"context"
	"fmt"

	apiextensionsv1 "k8s.io/apiextensions-apiserver/pkg/apis/apiextensions/v1"
	ctrl "sigs.k8s.io/controller-runtime"
	"sigs.k8s.io/controller-runtime/pkg/event"
	"sigs.k8s.io/controller-runtime/pkg/predicate"
)

const (
	MCHCRDName = "multiclusterhubs.operator.open-cluster-management.io"
)

type customResourceDefinitionsController struct{}

func (c *customResourceDefinitionsController) Reconcile(ctx context.Context, request ctrl.Request) (
	ctrl.Result, error,
) {
	// do nothing
	return ctrl.Result{}, nil
}

func AddCustomResourceDefinitionsController(mgr ctrl.Manager) error {
	crdPredicate := predicate.Funcs{
		CreateFunc: func(e event.CreateEvent) bool {
			if e.Object.GetName() == MCHCRDName {
				if err := startClusterClaimController(mgr); err != nil {
					fmt.Printf("failed to start the clusterCalimController: %v", err)
				}
				return true
			}
			return false
		},
	}
	if err := ctrl.NewControllerManagedBy(mgr).
		For(&apiextensionsv1.CustomResourceDefinition{}).
		WithEventFilter(crdPredicate).
		Complete(&customResourceDefinitionsController{}); err != nil {
		return fmt.Errorf("failed to add customresourcedefinitions controller to the manager: %w", err)
	}
	return nil
}
