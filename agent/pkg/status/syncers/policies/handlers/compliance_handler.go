package handlers

import (
	"fmt"

	policiesv1 "open-cluster-management.io/governance-policy-propagator/api/v1"
	"sigs.k8s.io/controller-runtime/pkg/client"

	"github.com/stolostron/multicluster-global-hub/agent/pkg/status/interfaces"
	"github.com/stolostron/multicluster-global-hub/pkg/bundle/grc"
	"github.com/stolostron/multicluster-global-hub/pkg/constants"
	"github.com/stolostron/multicluster-global-hub/pkg/utils"
)

type complianceHandler struct {
	eventData    *grc.ComplianceBundle
	shouldUpdate func(client.Object) bool
}

func NewComplianceHandler(eventData *grc.ComplianceBundle, shouldUpdate func(client.Object) bool) interfaces.Handler {
	return &complianceHandler{
		eventData:    eventData,
		shouldUpdate: shouldUpdate,
	}
}

func (h *complianceHandler) Get() interface{} {
	return h.eventData
}

func (h *complianceHandler) Update(obj client.Object) bool {
	if !h.shouldUpdate(obj) {
		return false
	}

	policy, isPolicy := obj.(*policiesv1.Policy)
	if !isPolicy {
		return false // do not handle objects other than policy
	}

	policyID := extractPolicyIdentity(obj)
	index := getIndexByPolicyID(policyID, *h.eventData)
	if index == -1 { // object not found, need to add it to the bundle
		compliance := getNewCompliance(policyID, policy)
		if len(compliance.CompliantClusters) == 0 && len(compliance.NonCompliantClusters) == 0 &&
			len(compliance.UnknownComplianceClusters) == 0 && len(compliance.PendingComplianceClusters) == 0 {
			return false
		}
		*h.eventData = append(*h.eventData, *compliance)
		return true
	}

	// when we update a policy, we need to increase bundle generation only if cluster list of the policy has changed.
	// for the use case where no cluster was added/removed, we use the complete compliance bundle to update policy status
	// and not the compliance bundle which contains a lot more information (full state).
	//
	// that being said, we still want to update the internal data and keep it always up to date in case a policy will be
	// inserted/removed (or cluster added/removed) and full state bundle will be triggered.

	// returns true if cluster list has changed, otherwise false
	return h.updatePayloadIfChanged(index, policy)
}

// returns true if cluster list has changed(added/removed), otherwise returns false (even if cluster statuses changed).
func (h *complianceHandler) updatePayloadIfChanged(objectIndex int, policy *policiesv1.Policy) bool {
	newCompliantClusters, newNonCompliantClusters, newUnknownClusters, newPendingClusters := getClusterStatus(policy)
	allClusters := utils.Merge(newCompliantClusters, newNonCompliantClusters, newUnknownClusters, newPendingClusters)

	cachedCompliance := (*h.eventData)[objectIndex]
	clusterListChanged := len(cachedCompliance.CompliantClusters)+len(cachedCompliance.NonCompliantClusters)+
		len(cachedCompliance.UnknownComplianceClusters)+len(cachedCompliance.PendingComplianceClusters) != len(allClusters) ||
		!utils.ContainSubStrings(allClusters, cachedCompliance.CompliantClusters) ||
		!utils.ContainSubStrings(allClusters, cachedCompliance.NonCompliantClusters) ||
		!utils.ContainSubStrings(allClusters, cachedCompliance.UnknownComplianceClusters) ||
		!utils.ContainSubStrings(allClusters, cachedCompliance.PendingComplianceClusters)

	// in any case we want to update the internal bundle in case statuses changed
	cachedCompliance.CompliantClusters = newCompliantClusters
	cachedCompliance.NonCompliantClusters = newNonCompliantClusters
	cachedCompliance.UnknownComplianceClusters = newUnknownClusters
	cachedCompliance.PendingComplianceClusters = newPendingClusters

	(*h.eventData)[objectIndex] = cachedCompliance
	return clusterListChanged
}

func (h *complianceHandler) Delete(obj client.Object) bool {
	if !h.shouldUpdate(obj) {
		return false
	}

	index := getCompliancesIndexByObj(obj, *h.eventData)
	if index == -1 { // trying to delete object which doesn't exist
		return false
	}

	*h.eventData = append((*h.eventData)[:index], (*h.eventData)[index+1:]...) // remove from objects
	return true
}

func getCompliancesIndexByObj(obj client.Object, compliances []grc.Compliance) int {
	uid := extractPolicyIdentity(obj)
	if len(uid) > 0 {
		for i, compliance := range compliances {
			if uid == string(compliance.PolicyID) {
				return i
			}
		}
	} else {
		for i, compliance := range compliances {
			if string(compliance.NamespacedName) == fmt.Sprintf("%s/%s", obj.GetNamespace(), obj.GetName()) {
				return i
			}
		}
	}
	return -1
}

func extractPolicyIdentity(obj client.Object) string {
	// if global policy id exist, then return the global policyID, else return local policyID
	if utils.HasAnnotation(obj, constants.OriginOwnerReferenceAnnotation) {
		return obj.GetAnnotations()[constants.OriginOwnerReferenceAnnotation]
	} else {
		return string(obj.GetUID())
	}
}

func getIndexByPolicyID(uid string, compliances []grc.Compliance) int {
	for i, object := range compliances {
		if object.PolicyID == uid {
			return i
		}
	}
	return -1
}

func getNewCompliance(originPolicyID string, policy *policiesv1.Policy) *grc.Compliance {
	compClusters, nonCompClusters, unknownCompClusters, pendingCompClusters := getClusterStatus(policy)
	return &grc.Compliance{
		PolicyID:                  originPolicyID,
		NamespacedName:            fmt.Sprintf("%s/%s", policy.GetNamespace(), policy.GetName()),
		CompliantClusters:         compClusters,
		NonCompliantClusters:      nonCompClusters,
		UnknownComplianceClusters: unknownCompClusters,
		PendingComplianceClusters: pendingCompClusters,
	}
}

// getClusterStatus returns (list of compliant clusters, list of nonCompliant clusters, list of unknown clusters.
func getClusterStatus(policy *policiesv1.Policy) ([]string, []string, []string, []string) {
	compliantClusters := make([]string, 0)
	nonCompliantClusters := make([]string, 0)
	unknownComplianceClusters := make([]string, 0)
	pendingComplianceClusters := make([]string, 0)

	for _, clusterStatus := range policy.Status.Status {
		switch clusterStatus.ComplianceState {
		case policiesv1.Compliant:
			compliantClusters = append(compliantClusters, clusterStatus.ClusterName)
		case policiesv1.NonCompliant:
			nonCompliantClusters = append(nonCompliantClusters, clusterStatus.ClusterName)
		case policiesv1.Pending:
			pendingComplianceClusters = append(pendingComplianceClusters, clusterStatus.ClusterName)
		default:
			unknownComplianceClusters = append(unknownComplianceClusters, clusterStatus.ClusterName)
		}
	}
	return compliantClusters, nonCompliantClusters, unknownComplianceClusters, pendingComplianceClusters
}
