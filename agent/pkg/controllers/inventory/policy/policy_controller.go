// Copyright (c) 2024 Red Hat, Inc.
// Copyright Contributors to the Open Cluster Management project
// Policy Controller is used to send the policy create/update/delete to restful API

package policies

import (
	"context"
	"fmt"

	kessel "github.com/project-kessel/inventory-api/api/kessel/inventory/v1beta1/resources"
	"k8s.io/apimachinery/pkg/api/errors"
	"k8s.io/apimachinery/pkg/types"
	policiesv1 "open-cluster-management.io/governance-policy-propagator/api/v1"
	ctrl "sigs.k8s.io/controller-runtime"
	"sigs.k8s.io/controller-runtime/pkg/client"
	"sigs.k8s.io/controller-runtime/pkg/event"
	"sigs.k8s.io/controller-runtime/pkg/predicate"

	"github.com/stolostron/multicluster-global-hub/agent/pkg/configs"
	"github.com/stolostron/multicluster-global-hub/pkg/transport"
	"github.com/stolostron/multicluster-global-hub/pkg/transport/requester"
)

type PolicyInventorySyncer struct {
	runtimeClient      client.Client
	reporterInstanceId string
	requester          transport.Requester
}

func AddPolicyInventorySyncer(mgr ctrl.Manager, inventoryRequester transport.Requester) error {
	policyPredicate := predicate.Funcs{
		UpdateFunc: func(e event.UpdateEvent) bool {
			return e.ObjectOld.GetResourceVersion() < e.ObjectNew.GetResourceVersion()
		},
		CreateFunc: func(e event.CreateEvent) bool {
			return true
		},
		DeleteFunc: func(e event.DeleteEvent) bool {
			return !e.DeleteStateUnknown
		},
	}

	return ctrl.NewControllerManagedBy(mgr).Named("inventory-policy-controller").
		For(&policiesv1.Policy{}).
		WithEventFilter(policyPredicate).
		Complete(&PolicyInventorySyncer{
			runtimeClient:      mgr.GetClient(),
			reporterInstanceId: requester.GetInventoryClientName(configs.GetLeafHubName()),
			requester:          inventoryRequester,
		})
}

func (p *PolicyInventorySyncer) Reconcile(ctx context.Context, req ctrl.Request) (ctrl.Result, error) {
	policy := &policiesv1.Policy{}
	err := p.runtimeClient.Get(ctx, types.NamespacedName{Namespace: req.Namespace, Name: req.Name}, policy)
	if err != nil {
		if errors.IsNotFound(err) {
			return ctrl.Result{}, nil
		}
		return ctrl.Result{}, err
	}

	if resp, err := p.requester.GetHttpClient().PolicyServiceClient.CreateK8SPolicy(
		ctx, createK8SClusterPolicy(*policy, p.reporterInstanceId)); err != nil {
		return ctrl.Result{}, fmt.Errorf("failed to create k8s-policy %v: %w", resp, err)
	}
	// may check the response to decide whether need to update or not
	// if resp, err := p.requester.GetHttpClient().PolicyServiceClient.UpdateK8SPolicy(
	// 	ctx, updateK8SClusterPolicy(*policy, p.reporterInstanceId)); err != nil {
	// 	return ctrl.Result{}, fmt.Errorf("failed to update k8s-policy %v: %w", resp, err)
	// }

	if !policy.DeletionTimestamp.IsZero() {
		if resp, err := p.requester.GetHttpClient().PolicyServiceClient.DeleteK8SPolicy(
			ctx, deleteK8SClusterPolicy(*policy, p.reporterInstanceId)); err != nil {
			return ctrl.Result{}, fmt.Errorf("failed to delete k8s-policy %v: %w", resp, err)
		}
		return ctrl.Result{}, nil
	}
	return ctrl.Result{}, nil
}

func createK8SClusterPolicy(policy policiesv1.Policy, reporterInstanceId string) *kessel.CreateK8SPolicyRequest {
	return &kessel.CreateK8SPolicyRequest{
		K8SPolicy: &kessel.K8SPolicy{
			Metadata: &kessel.Metadata{
				ResourceType: "k8s-policy",
			},
			ReporterData: &kessel.ReporterData{
				ReporterType:       kessel.ReporterData_ACM,
				ReporterInstanceId: reporterInstanceId,
				ReporterVersion:    configs.GetMCHVersion(),
				LocalResourceId:    policy.Namespace + "/" + policy.Name,
			},
			ResourceData: &kessel.K8SPolicyDetail{
				Disabled: policy.Spec.Disabled,
				Severity: kessel.K8SPolicyDetail_MEDIUM, // need to update
			},
		},
	}
}

// func updateK8SClusterPolicy(policy policiesv1.Policy, reporterInstanceId string) *kessel.UpdateK8SPolicyRequest {
// 	return &kessel.UpdateK8SPolicyRequest{
// 		K8SPolicy: &kessel.K8SPolicy{
// 			Metadata: &kessel.Metadata{
// 				ResourceType: "k8s-policy",
// 			},
// 			ReporterData: &kessel.ReporterData{
// 				ReporterType:       kessel.ReporterData_ACM,
// 				ReporterInstanceId: reporterInstanceId,
// 				ReporterVersion:    config.GetMCHVersion(),
// 				LocalResourceId:    policy.Namespace + "/" + policy.Name,
// 			},
// 			ResourceData: &kessel.K8SPolicyDetail{
// 				Disabled: policy.Spec.Disabled,
// 				Severity: kessel.K8SPolicyDetail_MEDIUM, //need to update
// 			},
// 		},
// 	}
// }

func deleteK8SClusterPolicy(policy policiesv1.Policy, reporterInstanceId string) *kessel.DeleteK8SPolicyRequest {
	return &kessel.DeleteK8SPolicyRequest{
		ReporterData: &kessel.ReporterData{
			ReporterType:       kessel.ReporterData_ACM,
			ReporterInstanceId: reporterInstanceId,
			ReporterVersion:    configs.GetMCHVersion(),
			LocalResourceId:    policy.Namespace + "/" + policy.Name,
		},
	}
}
