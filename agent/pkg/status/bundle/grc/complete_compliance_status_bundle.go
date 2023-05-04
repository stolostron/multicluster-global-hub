package grc

import (
	"fmt"
	"sync"

	policiesv1 "open-cluster-management.io/governance-policy-propagator/api/v1"

	bundlepkg "github.com/stolostron/multicluster-global-hub/agent/pkg/status/bundle"
	statusbundle "github.com/stolostron/multicluster-global-hub/pkg/bundle/status"
)

// NewCompleteComplianceStatusBundle creates a new instance of ComplianceStatusBundle.
func NewCompleteComplianceStatusBundle(leafHubName string, baseBundle bundlepkg.Bundle, incarnation uint64,
	extractObjIDFunc bundlepkg.ExtractObjIDFunc,
) bundlepkg.Bundle {
	return &ComplianceStatusBundle{
		BaseCompleteComplianceStatusBundle: statusbundle.BaseCompleteComplianceStatusBundle{
			Objects:           make([]*statusbundle.PolicyCompleteComplianceStatus, 0),
			LeafHubName:       leafHubName,
			BaseBundleVersion: baseBundle.GetBundleVersion(), // ALWAYS SYNCED SINCE POINTER
			BundleVersion:     statusbundle.NewBundleVersion(incarnation, 0),
		},
		baseBundle:       baseBundle,
		extractObjIDFunc: extractObjIDFunc,
		lock:             sync.Mutex{},
	}
}

// ComplianceStatusBundle abstracts management of compliance status bundle.
type ComplianceStatusBundle struct {
	statusbundle.BaseCompleteComplianceStatusBundle
	baseBundle       bundlepkg.Bundle
	extractObjIDFunc bundlepkg.ExtractObjIDFunc
	lock             sync.Mutex
}

// UpdateObject function to update a single object inside a bundle.
func (bundle *ComplianceStatusBundle) UpdateObject(object bundlepkg.Object) {
	bundle.lock.Lock()
	defer bundle.lock.Unlock()

	policy, isPolicy := object.(*policiesv1.Policy)
	if !isPolicy {
		return // do not handle objects other than policy
	}

	originPolicyID, ok := bundle.extractObjIDFunc(object)
	if !ok {
		return // cant update the object without finding its id.
	}

	index, err := bundle.getObjectIndexByUID(originPolicyID)
	if err != nil { // object not found, need to add it to the bundle (only in case it contains non-compliant/unknown)
		policyComplianceObject := bundle.getPolicyComplianceStatus(originPolicyID, policy)
		// don't send in the bundle a policy where all clusters are compliant
		if bundle.containsNonCompliantOrUnknownClusters(policyComplianceObject) {
			bundle.Objects = append(bundle.Objects, policyComplianceObject)
			bundle.BundleVersion.Generation++ // increase generation if objects array was changed
		}

		return
	}
	// if we reached here, policy already exists in the bundle with at least one non compliant or unknown cluster.
	if !bundle.updateBundleIfObjectChanged(index, policy) {
		return // true if changed, otherwise false. if policy compliance didn't change don't increment generation.
	}

	// don't send in the bundle a policy where all clusters are compliant
	if !bundle.containsNonCompliantOrUnknownClusters(bundle.Objects[index]) {
		bundle.Objects = append(bundle.Objects[:index], bundle.Objects[index+1:]...) // remove from objects
	}

	// increase bundle generation in the case where cluster lists were changed
	bundle.BundleVersion.Generation++
}

// DeleteObject function to delete a single object inside a bundle.
func (bundle *ComplianceStatusBundle) DeleteObject(object bundlepkg.Object) {
	bundle.lock.Lock()
	defer bundle.lock.Unlock()

	_, isPolicy := object.(*policiesv1.Policy)
	if !isPolicy {
		return // do not handle objects other than policy
	}

	index, err := bundle.getObjectIndexByObj(object)
	if err != nil { // trying to delete object which doesn't exist - return with no error
		return
	}

	// do not increase generation, no need to send bundle when policy is removed (clusters per policy bundle is sent).
	bundle.Objects = append(bundle.Objects[:index], bundle.Objects[index+1:]...) // remove from objects
}

// GetBundleVersion function to get bundle version
func (bundle *ComplianceStatusBundle) GetBundleVersion() *statusbundle.BundleVersion {
	bundle.lock.Lock()
	defer bundle.lock.Unlock()

	return bundle.BundleVersion
}

func (bundle *ComplianceStatusBundle) getObjectIndexByUID(uid string) (int, error) {
	for i, object := range bundle.Objects {
		if object.PolicyID == uid {
			return i, nil
		}
	}

	return -1, bundlepkg.ErrObjectNotFound
}

func (bundle *ComplianceStatusBundle) getPolicyComplianceStatus(originPolicyID string,
	policy *policiesv1.Policy,
) *statusbundle.PolicyCompleteComplianceStatus {
	nonCompliantClusters, unknownComplianceClusters := bundle.getNonCompliantAndUnknownClusters(policy)

	return &statusbundle.PolicyCompleteComplianceStatus{
		PolicyID:                  originPolicyID,
		NamespacedName:            policy.Namespace + "/" + policy.Name,
		NonCompliantClusters:      nonCompliantClusters,
		UnknownComplianceClusters: unknownComplianceClusters,
	}
}

// returns a list of non compliant clusters and a list of unknown compliance clusters.
func (bundle *ComplianceStatusBundle) getNonCompliantAndUnknownClusters(policy *policiesv1.Policy) ([]string,
	[]string,
) {
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

	return nonCompliantClusters, unknownComplianceClusters
}

// if a cluster was removed, object is not considered as changed.
func (bundle *ComplianceStatusBundle) updateBundleIfObjectChanged(objectIndex int, policy *policiesv1.Policy) bool {
	oldPolicyComplianceStatus := bundle.Objects[objectIndex]
	newNonCompliantClusters, newUnknownComplianceClusters := bundle.getNonCompliantAndUnknownClusters(policy)

	if !bundle.clusterListsEqual(oldPolicyComplianceStatus.NonCompliantClusters, newNonCompliantClusters) ||
		!bundle.clusterListsEqual(oldPolicyComplianceStatus.UnknownComplianceClusters, newUnknownComplianceClusters) {
		oldPolicyComplianceStatus.NonCompliantClusters = newNonCompliantClusters
		oldPolicyComplianceStatus.UnknownComplianceClusters = newUnknownComplianceClusters

		return true
	}

	return false
}

func (bundle *ComplianceStatusBundle) clusterListsEqual(oldClusters []string, newClusters []string) bool {
	if len(oldClusters) != len(newClusters) {
		return false
	}

	for _, newClusterName := range newClusters {
		if !bundlepkg.ContainsString(oldClusters, newClusterName) {
			return false
		}
	}

	return true
}

func (bundle *ComplianceStatusBundle) containsNonCompliantOrUnknownClusters(
	policyComplianceStatus *statusbundle.PolicyCompleteComplianceStatus,
) bool {
	if len(policyComplianceStatus.UnknownComplianceClusters) == 0 &&
		len(policyComplianceStatus.NonCompliantClusters) == 0 {
		return false
	}

	return true
}

func (bundle *ComplianceStatusBundle) getObjectIndexByObj(obj bundlepkg.Object) (int, error) {
	uid, _ := bundle.extractObjIDFunc(obj)
	if len(uid) > 0 {
		for i, object := range bundle.Objects {
			if uid == string(object.PolicyID) {
				return i, nil
			}
		}
	} else {
		for i, object := range bundle.Objects {
			if string(object.NamespacedName) == fmt.Sprintf("%s/%s", obj.GetNamespace(), obj.GetName()) {
				return i, nil
			}
		}
	}
	return -1, bundlepkg.ErrObjectNotFound
}
