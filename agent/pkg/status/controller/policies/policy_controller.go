// Copyright (c) 2024 Red Hat, Inc.
// Copyright Contributors to the Open Cluster Management project
// Policy Controller is used to send the policy create/update/delete to restful API

package policies

import (
	"context"
	"fmt"
	"reflect"

	kesselv1betarelations "github.com/project-kessel/inventory-api/api/kessel/inventory/v1beta1/relationships"
	kessel "github.com/project-kessel/inventory-api/api/kessel/inventory/v1beta1/resources"
	"k8s.io/apimachinery/pkg/api/errors"
	"k8s.io/apimachinery/pkg/types"
	policiesv1 "open-cluster-management.io/governance-policy-propagator/api/v1"
	ctrl "sigs.k8s.io/controller-runtime"
	"sigs.k8s.io/controller-runtime/pkg/client"
	"sigs.k8s.io/controller-runtime/pkg/controller/controllerutil"
	"sigs.k8s.io/controller-runtime/pkg/event"
	"sigs.k8s.io/controller-runtime/pkg/predicate"

	"github.com/stolostron/multicluster-global-hub/agent/pkg/config"
	statusconfig "github.com/stolostron/multicluster-global-hub/agent/pkg/status/controller/config"
	"github.com/stolostron/multicluster-global-hub/pkg/constants"
	"github.com/stolostron/multicluster-global-hub/pkg/transport"
	"github.com/stolostron/multicluster-global-hub/pkg/transport/requester"
)

type PolicyController struct {
	runtimeClient      client.Client
	reporterInstanceId string
	requester          transport.Requester
}

func AddPolicyController(mgr ctrl.Manager, inventoryRequester transport.Requester) error {
	policyPredicate := predicate.Funcs{
		UpdateFunc: func(e event.UpdateEvent) bool {
			// do not trigger the delete event for the replicated policies
			if _, exist := e.ObjectOld.GetLabels()[constants.PolicyEventRootPolicyNameLabelKey]; exist {
				return false
			}
			return !reflect.DeepEqual(e.ObjectNew.(*policiesv1.Policy).Status,
				e.ObjectOld.(*policiesv1.Policy).Status) ||
				!reflect.DeepEqual(e.ObjectNew.GetLabels(), e.ObjectOld.GetLabels()) ||
				e.ObjectNew.GetGeneration() != e.ObjectOld.GetGeneration()
		},
		CreateFunc: func(e event.CreateEvent) bool {
			// do not trigger the create event for the replicated policies
			if _, exist := e.Object.GetLabels()[constants.PolicyEventRootPolicyNameLabelKey]; exist {
				return false
			}
			// add the annotation to indicate the request is a create request
			// the annotation won't propagate to the etcd
			annotations := e.Object.GetAnnotations()
			if annotations == nil {
				annotations = map[string]string{}
			}
			annotations[constants.InventoryResourceCreatingAnnotationlKey] = ""
			e.Object.SetAnnotations(annotations)
			return true
		},
		DeleteFunc: func(e event.DeleteEvent) bool {
			// do not trigger the delete event for the replicated policies
			if _, exist := e.Object.GetLabels()[constants.PolicyEventRootPolicyNameLabelKey]; exist {
				return false
			}
			return !e.DeleteStateUnknown
		},
	}

	return ctrl.NewControllerManagedBy(mgr).Named("inventory-policy-controller").
		For(&policiesv1.Policy{}).
		WithEventFilter(policyPredicate).
		Complete(&PolicyController{
			runtimeClient:      mgr.GetClient(),
			reporterInstanceId: requester.GetInventoryClientName(statusconfig.GetLeafHubName()),
			requester:          inventoryRequester,
		})
}

func (p *PolicyController) Reconcile(ctx context.Context, req ctrl.Request) (ctrl.Result, error) {
	policy := &policiesv1.Policy{}
	err := p.runtimeClient.Get(ctx, types.NamespacedName{Namespace: req.Namespace, Name: req.Name}, policy)
	if err != nil {
		if errors.IsNotFound(err) {
			return ctrl.Result{}, nil
		}
		return ctrl.Result{}, err
	}

	k8sPolicy := generateK8SPolicy(policy, p.reporterInstanceId)

	annotations := policy.GetAnnotations()
	if annotations != nil {
		if _, ok := annotations[constants.InventoryResourceCreatingAnnotationlKey]; ok {
			if resp, err := p.requester.GetHttpClient().PolicyServiceClient.CreateK8SPolicy(
				ctx, &kessel.CreateK8SPolicyRequest{K8SPolicy: k8sPolicy}); err != nil {
				return ctrl.Result{}, fmt.Errorf("failed to create k8s-policy %v: %w", resp, err)
			}
		}
	}

	if policy.DeletionTimestamp.IsZero() {
		// add a finalizer to the policy object
		if !controllerutil.ContainsFinalizer(policy, constants.InventoryResourceFinalizer) {
			controllerutil.AddFinalizer(policy, constants.InventoryResourceFinalizer)
			if err := p.runtimeClient.Update(ctx, policy); err != nil {
				return ctrl.Result{}, err
			}
		}
	} else {
		// The policy object is being deleted
		if controllerutil.ContainsFinalizer(policy, constants.InventoryResourceFinalizer) {
			if resp, err := p.requester.GetHttpClient().PolicyServiceClient.DeleteK8SPolicy(
				ctx, deleteK8SClusterPolicy(*policy, p.reporterInstanceId)); err != nil {
				return ctrl.Result{}, fmt.Errorf("failed to delete k8s-policy %v: %w", resp, err)
			}
			for _, compliancePerClusterStatus := range policy.Status.Status {
				if resp, err := p.requester.GetHttpClient().K8SPolicyIsPropagatedToK8SClusterServiceHTTPClient.
					DeleteK8SPolicyIsPropagatedToK8SCluster(
						ctx, deleteK8SPolicyIsPropagatedToK8SCluster(policy.Namespace+"/"+policy.Name,
							compliancePerClusterStatus.ClusterName, p.reporterInstanceId)); err != nil {
					return ctrl.Result{}, fmt.Errorf("failed to delete k8s-policy_is-propagated-to_k8s-cluster %v: %w", resp, err)
				}
			}
			// remove finalizer
			controllerutil.RemoveFinalizer(policy, constants.InventoryResourceFinalizer)
			if err := p.runtimeClient.Update(ctx, policy); err != nil {
				return ctrl.Result{}, err
			}
		}
		return ctrl.Result{}, nil
	}

	if resp, err := p.requester.GetHttpClient().PolicyServiceClient.UpdateK8SPolicy(
		ctx, &kessel.UpdateK8SPolicyRequest{K8SPolicy: k8sPolicy}); err != nil {
		return ctrl.Result{}, fmt.Errorf("failed to update k8s-policy %v: %w", resp, err)
	}
	for _, compliancePerClusterStatus := range policy.Status.Status {
		if resp, err := p.requester.GetHttpClient().K8SPolicyIsPropagatedToK8SClusterServiceHTTPClient.
			UpdateK8SPolicyIsPropagatedToK8SCluster(
				ctx, updateK8SPolicyIsPropagatedToK8SCluster(policy.Namespace+"/"+policy.Name,
					compliancePerClusterStatus.ClusterName, string(compliancePerClusterStatus.ComplianceState),
					p.reporterInstanceId)); err != nil {
			return ctrl.Result{}, fmt.Errorf("failed to update k8s-policy_is-propagated-to_k8s-cluster %v: %w", resp, err)
		}
	}

	return ctrl.Result{}, nil
}

func updateK8SPolicyIsPropagatedToK8SCluster(subjectId, objectId, status, reporterInstanceId string) *kesselv1betarelations.
	UpdateK8SPolicyIsPropagatedToK8SClusterRequest {
	var relationStatus kesselv1betarelations.K8SPolicyIsPropagatedToK8SClusterDetail_Status
	switch status {
	case "NonCompliant":
		relationStatus = kesselv1betarelations.K8SPolicyIsPropagatedToK8SClusterDetail_VIOLATIONS
	case "Compliant":
		relationStatus = kesselv1betarelations.K8SPolicyIsPropagatedToK8SClusterDetail_NO_VIOLATIONS
	default:
		relationStatus = kesselv1betarelations.K8SPolicyIsPropagatedToK8SClusterDetail_STATUS_OTHER
	}
	return &kesselv1betarelations.UpdateK8SPolicyIsPropagatedToK8SClusterRequest{
		K8SpolicyIspropagatedtoK8Scluster: &kesselv1betarelations.K8SPolicyIsPropagatedToK8SCluster{
			Metadata: &kesselv1betarelations.Metadata{
				RelationshipType: "k8s-policy_ispropagatedto_k8s-cluster",
			},
			ReporterData: &kesselv1betarelations.ReporterData{
				ReporterType:           kesselv1betarelations.ReporterData_ACM,
				ReporterInstanceId:     reporterInstanceId,
				ReporterVersion:        config.GetMCHVersion(),
				SubjectLocalResourceId: subjectId,
				ObjectLocalResourceId:  objectId,
			},
			RelationshipData: &kesselv1betarelations.K8SPolicyIsPropagatedToK8SClusterDetail{
				Status: relationStatus,
			},
		},
	}
}

func deleteK8SPolicyIsPropagatedToK8SCluster(subjectId, objectId, reporterInstanceId string) *kesselv1betarelations.
	DeleteK8SPolicyIsPropagatedToK8SClusterRequest {
	return &kesselv1betarelations.DeleteK8SPolicyIsPropagatedToK8SClusterRequest{
		ReporterData: &kesselv1betarelations.ReporterData{
			ReporterType:           kesselv1betarelations.ReporterData_ACM,
			ReporterInstanceId:     reporterInstanceId,
			ReporterVersion:        config.GetMCHVersion(),
			SubjectLocalResourceId: subjectId,
			ObjectLocalResourceId:  objectId,
		},
	}
}

func generateK8SPolicy(policy *policiesv1.Policy, reporterInstanceId string) *kessel.K8SPolicy {
	kesselLabels := []*kessel.ResourceLabel{}
	for key, value := range policy.Labels {
		kesselLabels = append(kesselLabels, &kessel.ResourceLabel{
			Key:   key,
			Value: value,
		})
	}
	return &kessel.K8SPolicy{
		Metadata: &kessel.Metadata{
			ResourceType: "k8s-policy",
			Labels:       kesselLabels,
		},
		ReporterData: &kessel.ReporterData{
			ReporterType:       kessel.ReporterData_ACM,
			ReporterInstanceId: reporterInstanceId,
			ReporterVersion:    config.GetMCHVersion(),
			LocalResourceId:    policy.Namespace + "/" + policy.Name,
		},
		ResourceData: &kessel.K8SPolicyDetail{
			Disabled: policy.Spec.Disabled,
			Severity: kessel.K8SPolicyDetail_MEDIUM, // need to update
		},
	}
}

func deleteK8SClusterPolicy(policy policiesv1.Policy, reporterInstanceId string) *kessel.DeleteK8SPolicyRequest {
	return &kessel.DeleteK8SPolicyRequest{
		ReporterData: &kessel.ReporterData{
			ReporterType:       kessel.ReporterData_ACM,
			ReporterInstanceId: reporterInstanceId,
			ReporterVersion:    config.GetMCHVersion(),
			LocalResourceId:    policy.Namespace + "/" + policy.Name,
		},
	}
}
