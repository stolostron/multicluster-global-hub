package grc

import (
	"fmt"
	"sync"

	policiesv1 "open-cluster-management.io/governance-policy-propagator/api/v1"

	"github.com/stolostron/multicluster-global-hub/pkg/bundle"
	"github.com/stolostron/multicluster-global-hub/pkg/bundle/base"
	"github.com/stolostron/multicluster-global-hub/pkg/bundle/metadata"
	"github.com/stolostron/multicluster-global-hub/pkg/constants"
)

var (
	_ bundle.AgentBundle   = (*MinimalComplianceBundle)(nil)
	_ bundle.ManagerBundle = (*MinimalComplianceBundle)(nil)
)

// MinimalComplianceBundle abstracts management of minimal compliance status bundle.
type MinimalComplianceBundle struct {
	base.BaseMinimalComplianceBundle
	lock sync.Mutex
}

// NewAgentMinimalComplianceBundle creates a new instance of MinimalComplianceStatusBundle.
func NewAgentMinimalComplianceBundle(leafHubName string) bundle.AgentBundle {
	return &MinimalComplianceBundle{
		BaseMinimalComplianceBundle: base.BaseMinimalComplianceBundle{
			Objects:       make([]*base.MinimalCompliance, 0),
			LeafHubName:   leafHubName,
			BundleVersion: metadata.NewBundleVersion(),
		},
	}
}

// NewAgentMinimalComplianceBundle creates a new instance of MinimalComplianceStatusBundle.
func NewManagerMinimalComplianceBundle() bundle.ManagerBundle {
	return &MinimalComplianceBundle{}
}

// Manger - GetLeafHubName returns the leaf hub name that sent the bundle.
func (bundle *MinimalComplianceBundle) GetLeafHubName() string {
	return bundle.LeafHubName
}

// Manger - GetObjects returns the objects in the bundle.
func (bundle *MinimalComplianceBundle) GetObjects() []interface{} {
	result := make([]interface{}, len(bundle.Objects))
	for i, obj := range bundle.Objects {
		result[i] = obj
	}

	return result
}

// Manger
func (bundle *MinimalComplianceBundle) SetVersion(version *metadata.BundleVersion) {
	bundle.lock.Lock()
	defer bundle.lock.Unlock()
	bundle.BundleVersion = version
}

// Agent - UpdateObject function to update a single object inside a bundle.
func (b *MinimalComplianceBundle) UpdateObject(object bundle.Object) {
	b.lock.Lock()
	defer b.lock.Unlock()

	policy, isPolicy := object.(*policiesv1.Policy)
	if !isPolicy {
		return // do not handle objects other than policy
	}

	originPolicyID, found := object.GetAnnotations()[constants.OriginOwnerReferenceAnnotation]
	if !found {
		return // origin owner reference annotation not found, not handling this policy (wasn't sent from hub of hubs)
	}

	index, err := b.getObjectIndexByUID(originPolicyID)
	if err != nil { // object not found, need to add it to the bundle
		b.Objects = append(b.Objects,
			b.getMinimalPolicyComplianceStatus(originPolicyID, policy))
		b.BundleVersion.Incr()

		return
	}

	// if we reached here, object already exists in the bundle, check if the object has changed.
	if !b.updateObjectIfChanged(index, policy) {
		return // returns true if changed, otherwise false. if cluster list didn't change, don't increment generation.
	}

	// if cluster list has changed - update resource version of the object and bundle generation
	b.BundleVersion.Incr()
}

// DeleteObject function to delete a single object inside a bundle.
func (b *MinimalComplianceBundle) DeleteObject(object bundle.Object) {
	b.lock.Lock()
	defer b.lock.Unlock()
	_, isPolicy := object.(*policiesv1.Policy)
	if !isPolicy {
		return // do not handle objects other than policy
	}

	index, err := b.getObjectIndexByObj(object)
	if err != nil { // trying to delete object which doesn't exist - return with no error
		return
	}

	b.Objects = append(b.Objects[:index], b.Objects[index+1:]...) // remove from objects
	b.BundleVersion.Incr()
}

// GetVersion function to get bundle version.
func (b *MinimalComplianceBundle) GetVersion() *metadata.BundleVersion {
	return b.BundleVersion
}

func (bundle *MinimalComplianceBundle) getObjectIndexByUID(uid string) (int, error) {
	for i, object := range bundle.Objects {
		if object.PolicyID == uid {
			return i, nil
		}
	}

	return -1, fmt.Errorf("not found obj by uid: %s", uid)
}

func (b *MinimalComplianceBundle) getMinimalPolicyComplianceStatus(originPolicyID string,
	policy *policiesv1.Policy,
) *base.MinimalCompliance {
	appliedClusters, nonCompliantClusters := b.getNumOfClusters(policy)

	return &base.MinimalCompliance{
		PolicyID:             originPolicyID,
		NamespacedName:       fmt.Sprintf("%s/%s", policy.Namespace, policy.Name),
		RemediationAction:    policy.Spec.RemediationAction,
		NonCompliantClusters: nonCompliantClusters,
		AppliedClusters:      appliedClusters,
	}
}

func (b *MinimalComplianceBundle) updateObjectIfChanged(index int, policy *policiesv1.Policy) bool {
	appliedClusters, nonCompliantClusters := b.getNumOfClusters(policy)

	if b.Objects[index].RemediationAction != policy.Spec.RemediationAction {
		b.Objects[index].RemediationAction = policy.Spec.RemediationAction
		b.Objects[index].AppliedClusters = appliedClusters
		b.Objects[index].NonCompliantClusters = nonCompliantClusters

		return true
	}

	if b.Objects[index].AppliedClusters != appliedClusters {
		b.Objects[index].AppliedClusters = appliedClusters
		b.Objects[index].NonCompliantClusters = nonCompliantClusters

		return true
	}

	if b.Objects[index].NonCompliantClusters != nonCompliantClusters {
		b.Objects[index].NonCompliantClusters = nonCompliantClusters
		return true
	}

	return false
}

func (b *MinimalComplianceBundle) getNumOfClusters(policy *policiesv1.Policy) (int, int) {
	appliedClusters := len(policy.Status.Status)
	nonCompliantClusters := 0

	for _, clusterCompliance := range policy.Status.Status {
		if clusterCompliance.ComplianceState != policiesv1.Compliant {
			nonCompliantClusters++
		}
	}

	return appliedClusters, nonCompliantClusters
}

func (b *MinimalComplianceBundle) getObjectIndexByObj(obj bundle.Object) (int, error) {
	originPolicyID, found := obj.GetAnnotations()[constants.OriginOwnerReferenceAnnotation]
	if found {
		for i, object := range b.Objects {
			if originPolicyID == string(object.PolicyID) {
				return i, nil
			}
		}
	} else {
		for i, object := range b.Objects {
			if string(object.NamespacedName) == fmt.Sprintf("%s/%s", obj.GetNamespace(), obj.GetName()) {
				return i, nil
			}
		}
	}
	return -1, fmt.Errorf("not found object by id or namespacedName")
}

func (bundle *MinimalComplianceBundle) IncrVersion() {
	bundle.lock.Lock()
	defer bundle.lock.Unlock()
	bundle.BundleVersion.Incr()
}
