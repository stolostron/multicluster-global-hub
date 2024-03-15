package policies

import (
	"fmt"

	policiesv1 "open-cluster-management.io/governance-policy-propagator/api/v1"
	"sigs.k8s.io/controller-runtime/pkg/client"

	"github.com/stolostron/multicluster-global-hub/agent/pkg/status/controller/generic"
	"github.com/stolostron/multicluster-global-hub/pkg/bundle/grc"
	"github.com/stolostron/multicluster-global-hub/pkg/bundle/version"
	"github.com/stolostron/multicluster-global-hub/pkg/enum"
	"github.com/stolostron/multicluster-global-hub/pkg/utils"
)

func CompleteComplianceEmitterWrapper(
	eventType enum.EventType,
	dependencyVersion *version.Version,
	predicate func(client.Object) bool,
) generic.ObjectEmitter {
	eventData := grc.CompleteComplianceBundle{}
	return generic.NewGenericObjectEmitter(
		eventType,
		&eventData,
		NewCompleteComplianceHandler(&eventData),
		generic.WithShouldUpdate(predicate),
		generic.WithDependencyVersion(dependencyVersion),
	)
}

type completeComplianceHandler struct {
	eventData *grc.CompleteComplianceBundle
}

func NewCompleteComplianceHandler(evtData *grc.CompleteComplianceBundle) generic.Handler {
	return &completeComplianceHandler{
		eventData: evtData,
	}
}

func (h *completeComplianceHandler) Update(obj client.Object) bool {
	policy, isPolicy := obj.(*policiesv1.Policy)
	if !isPolicy {
		return false // do not handle objects other than policy
	}
	if policy.Status.Status == nil {
		return false
	}

	originPolicyID := extractPolicyIdentity(obj)
	newComplete := newCompleteCompliance(originPolicyID, policy)

	index := getPayloadIndexByUID(originPolicyID, *(h.eventData))
	if index == -1 { // object not found, need to add it to the bundle (only in case it contains non-compliant/unknown)
		// don't send in the bundle a policy where all clusters are compliant
		if len(newComplete.UnknownComplianceClusters) == 0 && len(newComplete.NonCompliantClusters) == 0 {
			return false
		}

		*(h.eventData) = append(*(h.eventData), *newComplete)
		return true
	}

	// if we reached here, policy already exists in the bundle with at least one non compliant or unknown cluster.
	oldComplete := (*h.eventData)[index]
	if utils.Equal(oldComplete.NonCompliantClusters, newComplete.NonCompliantClusters) &&
		utils.Equal(oldComplete.UnknownComplianceClusters, newComplete.UnknownComplianceClusters) {
		return false
	}

	// the payload is updated
	(*h.eventData)[index].NonCompliantClusters = newComplete.NonCompliantClusters
	(*h.eventData)[index].UnknownComplianceClusters = newComplete.UnknownComplianceClusters

	// don't send in the bundle a policy where all clusters are compliant
	if len((*h.eventData)[index].NonCompliantClusters) == 0 && len((*h.eventData)[index].UnknownComplianceClusters) == 0 {
		(*h.eventData) = append((*h.eventData)[:index], (*h.eventData)[index+1:]...) // remove from objects
	}
	return true
}

func (h *completeComplianceHandler) Delete(obj client.Object) bool {
	_, isPolicy := obj.(*policiesv1.Policy)
	if !isPolicy {
		return false // don't handle objects other than policy
	}

	index := getPayloadIndexByObj(obj, *(h.eventData))
	if index == -1 { // trying to delete object which doesn't exist
		return false
	}

	// don't increase version, no need to send bundle when policy is removed (Compliance bundle is sent).
	*(h.eventData) = append((*h.eventData)[:index], (*h.eventData)[index+1:]...) // remove from objects
	return false
}

func newCompleteCompliance(originPolicyID string, policy *policiesv1.Policy) *grc.CompleteCompliance {
	nonCompliantClusters := make([]string, 0)
	unknownComplianceClusters := make([]string, 0)

	for _, clusterCompliance := range policy.Status.Status {
		if clusterCompliance.ComplianceState == policiesv1.Compliant {
			continue
		}
		if clusterCompliance.ComplianceState == policiesv1.NonCompliant {
			nonCompliantClusters = append(nonCompliantClusters, clusterCompliance.ClusterName)
		} else { // not compliant not non compliant -> means unknown
			unknownComplianceClusters = append(unknownComplianceClusters, clusterCompliance.ClusterName)
		}
	}

	return &grc.CompleteCompliance{
		PolicyID:                  originPolicyID,
		NamespacedName:            policy.Namespace + "/" + policy.Name,
		NonCompliantClusters:      nonCompliantClusters,
		UnknownComplianceClusters: unknownComplianceClusters,
	}
}

func getPayloadIndexByObj(obj client.Object, completes []grc.CompleteCompliance) int {
	uid := extractPolicyIdentity(obj)
	if len(uid) > 0 {
		for i, complete := range completes {
			if uid == string(complete.PolicyID) {
				return i
			}
		}
	} else {
		for i, complete := range completes {
			if string(complete.NamespacedName) == fmt.Sprintf("%s/%s", obj.GetNamespace(), obj.GetName()) {
				return i
			}
		}
	}
	return -1
}

func getPayloadIndexByUID(uid string, completeCompliances []grc.CompleteCompliance) int {
	for i, object := range completeCompliances {
		if object.PolicyID == uid {
			return i
		}
	}
	return -1
}
