package grc

import (
	"sync"

	policiesv1 "open-cluster-management.io/governance-policy-propagator/api/v1"

	bundlepkg "github.com/stolostron/hub-of-hubs/agent/pkg/status/bundle"
	statusbundle "github.com/stolostron/hub-of-hubs/pkg/bundle/status"
	"github.com/stolostron/hub-of-hubs/pkg/constants"
)

// NewMinimalComplianceStatusBundle creates a new instance of MinimalComplianceStatusBundle.
func NewMinimalComplianceStatusBundle(leafHubName string, incarnation uint64) bundlepkg.Bundle {
	return &MinimalComplianceStatusBundle{
		BaseMinimalComplianceStatusBundle: statusbundle.BaseMinimalComplianceStatusBundle{
			Objects:       make([]*statusbundle.MinimalPolicyComplianceStatus, 0),
			LeafHubName:   leafHubName,
			BundleVersion: statusbundle.NewBundleVersion(incarnation, 0),
		},
		lock: sync.Mutex{},
	}
}

// MinimalComplianceStatusBundle abstracts management of minimal compliance status bundle.
type MinimalComplianceStatusBundle struct {
	statusbundle.BaseMinimalComplianceStatusBundle
	lock sync.Mutex
}

// UpdateObject function to update a single object inside a bundle.
func (bundle *MinimalComplianceStatusBundle) UpdateObject(object bundlepkg.Object) {
	bundle.lock.Lock()
	defer bundle.lock.Unlock()

	policy, isPolicy := object.(*policiesv1.Policy)
	if !isPolicy {
		return // do not handle objects other than policy
	}

	originPolicyID, found := object.GetAnnotations()[constants.OriginOwnerReferenceAnnotation]
	if !found {
		return // origin owner reference annotation not found, not handling this policy (wasn't sent from hub of hubs)
	}

	index, err := bundle.getObjectIndexByUID(originPolicyID)
	if err != nil { // object not found, need to add it to the bundle
		bundle.Objects = append(bundle.Objects,
			bundle.getMinimalPolicyComplianceStatus(originPolicyID, policy))
		bundle.BundleVersion.Generation++

		return
	}

	// if we reached here, object already exists in the bundle, check if the object has changed.
	if !bundle.updateObjectIfChanged(index, policy) {
		return // returns true if changed, otherwise false. if cluster list didn't change, don't increment generation.
	}

	// if cluster list has changed - update resource version of the object and bundle generation
	bundle.BundleVersion.Generation++
}

// DeleteObject function to delete a single object inside a bundle.
func (bundle *MinimalComplianceStatusBundle) DeleteObject(object bundlepkg.Object) {
	bundle.lock.Lock()
	defer bundle.lock.Unlock()

	_, isPolicy := object.(*policiesv1.Policy)
	if !isPolicy {
		return // do not handle objects other than policy
	}

	originPolicyID, found := object.GetAnnotations()[constants.OriginOwnerReferenceAnnotation]
	if !found {
		return // origin owner reference annotation not found, don't handle this policy
	}

	index, err := bundle.getObjectIndexByUID(originPolicyID)
	if err != nil { // trying to delete object which doesn't exist - return with no error
		return
	}

	bundle.Objects = append(bundle.Objects[:index], bundle.Objects[index+1:]...) // remove from objects
	bundle.BundleVersion.Generation++
}

// GetBundleVersion function to get bundle version.
func (bundle *MinimalComplianceStatusBundle) GetBundleVersion() *statusbundle.BundleVersion {
	bundle.lock.Lock()
	defer bundle.lock.Unlock()

	return bundle.BundleVersion
}

func (bundle *MinimalComplianceStatusBundle) getObjectIndexByUID(uid string) (int, error) {
	for i, object := range bundle.Objects {
		if object.PolicyID == uid {
			return i, nil
		}
	}

	return -1, bundlepkg.ErrObjectNotFound
}

func (bundle *MinimalComplianceStatusBundle) getMinimalPolicyComplianceStatus(originPolicyID string,
	policy *policiesv1.Policy,
) *statusbundle.MinimalPolicyComplianceStatus {
	appliedClusters, nonCompliantClusters := bundle.getNumOfClusters(policy)

	return &statusbundle.MinimalPolicyComplianceStatus{
		PolicyID:             originPolicyID,
		RemediationAction:    policy.Spec.RemediationAction,
		NonCompliantClusters: nonCompliantClusters,
		AppliedClusters:      appliedClusters,
	}
}

func (bundle *MinimalComplianceStatusBundle) updateObjectIfChanged(index int, policy *policiesv1.Policy) bool {
	appliedClusters, nonCompliantClusters := bundle.getNumOfClusters(policy)

	if bundle.Objects[index].RemediationAction != policy.Spec.RemediationAction {
		bundle.Objects[index].RemediationAction = policy.Spec.RemediationAction
		bundle.Objects[index].AppliedClusters = appliedClusters
		bundle.Objects[index].NonCompliantClusters = nonCompliantClusters

		return true
	}

	if bundle.Objects[index].AppliedClusters != appliedClusters {
		bundle.Objects[index].AppliedClusters = appliedClusters
		bundle.Objects[index].NonCompliantClusters = nonCompliantClusters

		return true
	}

	if bundle.Objects[index].NonCompliantClusters != nonCompliantClusters {
		bundle.Objects[index].NonCompliantClusters = nonCompliantClusters
		return true
	}

	return false
}

func (bundle *MinimalComplianceStatusBundle) getNumOfClusters(policy *policiesv1.Policy) (int, int) {
	appliedClusters := len(policy.Status.Status)
	nonCompliantClusters := 0

	for _, clusterCompliance := range policy.Status.Status {
		if clusterCompliance.ComplianceState != policiesv1.Compliant {
			nonCompliantClusters++
		}
	}

	return appliedClusters, nonCompliantClusters
}
