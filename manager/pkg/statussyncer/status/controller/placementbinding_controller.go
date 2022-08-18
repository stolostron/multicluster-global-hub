// Copyright (c) 2020 Red Hat, Inc.
// Copyright Contributors to the Open Cluster Management project

package controller

import (
	"context"
	"fmt"

	"github.com/go-logr/logr"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/types"
	policyv1 "open-cluster-management.io/governance-policy-propagator/api/v1"
	ctrl "sigs.k8s.io/controller-runtime"
	"sigs.k8s.io/controller-runtime/pkg/client"
	"sigs.k8s.io/controller-runtime/pkg/predicate"

	"github.com/stolostron/multicluster-global-hub/pkg/constants"
)

type placementBindingController struct {
	client client.Client
	log    logr.Logger
}

func (c *placementBindingController) Reconcile(ctx context.Context, request ctrl.Request) (ctrl.Result, error) {
	reqLogger := c.log.WithValues("Request.Namespace", request.Namespace, "Request.Name", request.Name)

	instance := &policyv1.PlacementBinding{}
	err := c.client.Get(ctx, request.NamespacedName, instance)
	if err != nil {
		reqLogger.Error(err, "Reconciliation failed")
		return ctrl.Result{}, err
	}

	for _, subject := range instance.Subjects {
		if subject.APIGroup == policyv1.SchemeGroupVersion.Group &&
			subject.Kind == policyv1.Kind {
			err := c.updatePolicyStatus(ctx, subject.Name, request.Namespace,
				instance.PlacementRef.Name, instance.Name)
			if err != nil {
				return ctrl.Result{}, err
			}
		}
	}

	reqLogger.Info("Reconciliation complete.")

	return ctrl.Result{}, nil
}

func (c *placementBindingController) updatePolicyStatus(ctx context.Context, name, namespace,
	placementRule, placementBinding string,
) error {
	policy := &policyv1.Policy{}
	err := c.client.Get(ctx, types.NamespacedName{
		Name:      name,
		Namespace: namespace,
	}, policy)
	if err != nil {
		return fmt.Errorf("failed to get policy: %w", err)
	}

	// TODO: need to handle placement in future
	for _, placement := range policy.Status.Placement {
		if placement.PlacementRule == placementRule {
			return nil
		}
	}

	originalPolicy := policy.DeepCopy()

	policy.Status.Placement = append(policy.Status.Placement, &policyv1.Placement{
		PlacementBinding: placementBinding,
		PlacementRule:    placementRule,
	})

	err = c.client.Status().Patch(ctx, policy, client.MergeFrom(originalPolicy))
	if err != nil {
		return fmt.Errorf("failed to update policy CR: %w", err)
	}
	return nil
}

func AddPlacementBindingController(mgr ctrl.Manager) error {
	placementBindingPredicate, _ := predicate.LabelSelectorPredicate(metav1.LabelSelector{
		MatchExpressions: []metav1.LabelSelectorRequirement{
			{
				Key:      constants.GlobalHubLocalResource,
				Operator: metav1.LabelSelectorOpDoesNotExist,
			},
		},
	})
	if err := ctrl.NewControllerManagedBy(mgr).
		For(&policyv1.PlacementBinding{}).
		WithEventFilter(placementBindingPredicate).
		Complete(&placementBindingController{
			client: mgr.GetClient(),
			log:    ctrl.Log.WithName("placementbindings-controller"),
		}); err != nil {
		return fmt.Errorf("failed to add placement binding controller to the manager: %w", err)
	}

	return nil
}
