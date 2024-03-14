package policies

import (
	"fmt"

	policiesv1 "open-cluster-management.io/governance-policy-propagator/api/v1"
	"sigs.k8s.io/controller-runtime/pkg/client"

	"github.com/stolostron/multicluster-global-hub/agent/pkg/status/controller/generic"
	"github.com/stolostron/multicluster-global-hub/pkg/bundle/grc"
	eventversion "github.com/stolostron/multicluster-global-hub/pkg/bundle/version"
	"github.com/stolostron/multicluster-global-hub/pkg/constants"
	"github.com/stolostron/multicluster-global-hub/pkg/enum"
	"github.com/stolostron/multicluster-global-hub/pkg/utils"
)

func ComplianceEmitterWrapper(
	eventType enum.EventType,
	version *eventversion.Version,
	predicate func(client.Object) bool,
) generic.ObjectEmitter {
	eventData := grc.ComplianceBundle{}
	return generic.NewGenericObjectEmitter(
		eventType,
		&eventData,
		NewComplianceHandler(&eventData),
		generic.WithShouldUpdate(predicate),
		generic.WithVersion(version),
	)
}

type complianceHandler struct {
	eventData *grc.ComplianceBundle
}

func NewComplianceHandler(eventData *grc.ComplianceBundle) generic.Handler {
	return &complianceHandler{
		eventData: eventData,
	}
}

func (h *complianceHandler) Update(obj client.Object) bool {
	policy, isPolicy := obj.(*policiesv1.Policy)
	if !isPolicy {
		return false // do not handle objects other than policy
	}

	policyID := extractPolicyIdentity(obj)
	index := getIndexByPolicyID(policyID, *h.eventData)
	if index == -1 { // object not found, need to add it to the bundle
		compliance := getNewCompliance(policyID, policy)
		if len(compliance.CompliantClusters) == 0 && len(compliance.NonCompliantClusters) == 0 &&
			len(compliance.UnknownComplianceClusters) == 0 {
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
	newCompliantClusters, newNonCompliantClusters, newUnknownClusters := getClusterStatus(policy)
	allClusters := utils.Merge(newCompliantClusters, newNonCompliantClusters, newUnknownClusters)

	cachedCompliance := (*h.eventData)[objectIndex]
	clusterListChanged := false

	// check if any cluster was added or removed
	if len(cachedCompliance.CompliantClusters)+len(cachedCompliance.NonCompliantClusters)+
		len(cachedCompliance.UnknownComplianceClusters) != len(allClusters) ||
		!utils.ContainSubStrings(allClusters, cachedCompliance.CompliantClusters) ||
		!utils.ContainSubStrings(allClusters, cachedCompliance.NonCompliantClusters) ||
		!utils.ContainSubStrings(allClusters, cachedCompliance.UnknownComplianceClusters) {
		clusterListChanged = true // at least one cluster was added/removed
	}

	// in any case we want to update the internal bundle in case statuses changed
	cachedCompliance.CompliantClusters = newCompliantClusters
	cachedCompliance.NonCompliantClusters = newNonCompliantClusters
	cachedCompliance.UnknownComplianceClusters = newUnknownClusters
	(*h.eventData)[objectIndex] = cachedCompliance
	return clusterListChanged
}

func (h *complianceHandler) Delete(obj client.Object) bool {
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
	compliantClusters, nonCompliantClusters, unknownComplianceClusters := getClusterStatus(policy)
	return &grc.Compliance{
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
