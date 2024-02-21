package grc

import (
	"fmt"

	"github.com/stolostron/multicluster-global-hub/pkg/bundle"
	"github.com/stolostron/multicluster-global-hub/pkg/constants"
	"github.com/stolostron/multicluster-global-hub/pkg/utils"
	policiesv1 "open-cluster-management.io/governance-policy-propagator/api/v1"
	"sigs.k8s.io/controller-runtime/pkg/client"
)

var _ bundle.Payload = (*CompliancePayload)(nil)

type Compliance struct {
	PolicyID                  string   `json:"policyId"`
	NamespacedName            string   `json:"-"` // need it to delete obj from bundle for these without finalizer.
	CompliantClusters         []string `json:"compliantClusters"`
	NonCompliantClusters      []string `json:"nonCompliantClusters"`
	UnknownComplianceClusters []string `json:"unknownComplianceClusters"`
}

type CompliancePayload []Compliance

func (p *CompliancePayload) Update(obj client.Object) bool {

	policy, isPolicy := obj.(*policiesv1.Policy)
	if !isPolicy {
		return false // do not handle objects other than policy
	}

	policyID := extractPolicyID(obj)
	index := getIndexByPolicyID(policyID, *p)
	if index == -1 { // object not found, need to add it to the bundle
		*p = append(*p, *getNewCompliance(policyID, policy))
		return true
	}

	// when we update a policy, we need to increase bundle generation only if cluster list of the policy has changed.
	// for the use case where no cluster was added/removed, we use the complete compliance bundle to update policy status
	// and not the compliance bundle which contains a lot more information (full state).
	//
	// that being said, we still want to update the internal data and keep it always up to date in case a policy will be
	// inserted/removed (or cluster added/removed) and full state bundle will be triggered.

	// returns true if cluster list has changed, otherwise false
	return p.updatePayloadIfChanged(index, policy)
}

// returns true if cluster list has changed(added/removed), otherwise returns false (even if cluster statuses changed).
func (p *CompliancePayload) updatePayloadIfChanged(objectIndex int, policy *policiesv1.Policy) bool {
	newCompliantClusters, newNonCompliantClusters, newUnknownClusters := getClusterStatus(policy)
	allClusters := utils.Merge(newCompliantClusters, newNonCompliantClusters, newUnknownClusters)

	payloadCompliance := (*p)[objectIndex]
	clusterListChanged := false

	// check if any cluster was added or removed
	if len(payloadCompliance.CompliantClusters)+len(payloadCompliance.NonCompliantClusters)+
		len(payloadCompliance.UnknownComplianceClusters) != len(allClusters) ||
		!utils.ContainSubStrings(allClusters, payloadCompliance.CompliantClusters) ||
		!utils.ContainSubStrings(allClusters, payloadCompliance.NonCompliantClusters) ||
		!utils.ContainSubStrings(allClusters, payloadCompliance.UnknownComplianceClusters) {
		clusterListChanged = true // at least one cluster was added/removed
	}

	// in any case we want to update the internal bundle in case statuses changed
	payloadCompliance.CompliantClusters = newCompliantClusters
	payloadCompliance.NonCompliantClusters = newNonCompliantClusters
	payloadCompliance.UnknownComplianceClusters = newUnknownClusters

	return clusterListChanged
}

func (p *CompliancePayload) Delete(obj client.Object) bool {

	index := getObjectIndexByObj(obj, *p)
	if index == -1 { // trying to delete object which doesn't exist
		return false
	}

	*p = append((*p)[:index], (*p)[index+1:]...) // remove from objects
	return true
}

func getObjectIndexByObj(obj client.Object, compliances []Compliance) int {
	uid := extractPolicyID(obj)
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

func extractPolicyID(obj client.Object) string {
	// if global policy id exist, then return the global policyID, else return local policyID
	if utils.HasAnnotation(obj, constants.OriginOwnerReferenceAnnotation) {
		return obj.GetAnnotations()[constants.OriginOwnerReferenceAnnotation]
	} else {
		return string(obj.GetUID())
	}
}

func getIndexByPolicyID(uid string, compliances []Compliance) int {
	for i, object := range compliances {
		if object.PolicyID == uid {
			return i
		}
	}
	return -1
}

func getNewCompliance(originPolicyID string, policy *policiesv1.Policy) *Compliance {
	compliantClusters, nonCompliantClusters, unknownComplianceClusters := getClusterStatus(policy)
	return &Compliance{
		PolicyID:                  originPolicyID,
		NamespacedName:            fmt.Sprintf("%s/%s", policy.GetNamespace(), policy.GetName()),
		CompliantClusters:         compliantClusters,
		NonCompliantClusters:      nonCompliantClusters,
		UnknownComplianceClusters: unknownComplianceClusters,
	}
}

// getClusterStatus returns (list of compliant clusters, list of nonCompliant clusters, list of unknown clusters.
func getClusterStatus(policy *policiesv1.Policy) ([]string, []string, []string) {
	compliantClusters := make([]string, 0)
	nonCompliantClusters := make([]string, 0)
	unknownComplianceClusters := make([]string, 0)
	for _, clusterStatus := range policy.Status.Status {
		if clusterStatus.ComplianceState == policiesv1.Compliant {
			compliantClusters = append(compliantClusters, clusterStatus.ClusterName)
		} else if clusterStatus.ComplianceState == policiesv1.NonCompliant {
			nonCompliantClusters = append(nonCompliantClusters, clusterStatus.ClusterName)
		} else {
			unknownComplianceClusters = append(unknownComplianceClusters, clusterStatus.ClusterName)
		}
	}
	return compliantClusters, nonCompliantClusters, unknownComplianceClusters
}
