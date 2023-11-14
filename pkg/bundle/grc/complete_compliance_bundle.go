package grc

import (
	"fmt"
	"sync"

	policiesv1 "open-cluster-management.io/governance-policy-propagator/api/v1"

	"github.com/stolostron/multicluster-global-hub/pkg/bundle"
	"github.com/stolostron/multicluster-global-hub/pkg/bundle/base"
	"github.com/stolostron/multicluster-global-hub/pkg/bundle/metadata"
	utils "github.com/stolostron/multicluster-global-hub/pkg/utils"
)

var (
	_ bundle.AgentBundle            = (*CompleteComplianceBundle)(nil)
	_ bundle.ManagerDependantBundle = (*CompleteComplianceBundle)(nil)
)

// CompleteComplianceBundle abstracts management of compliance status bundle.
type CompleteComplianceBundle struct {
	base.BaseCompleteComplianceBundle
	baseBundle       bundle.AgentBundle
	extractObjIDFunc ExtractObjIDFunc
	lock             sync.Mutex
}

// NewAgentCompleteComplianceBundle creates a new instance of ComplianceStatusBundle.
func NewAgentCompleteComplianceBundle(leafHubName string, baseBundle bundle.AgentBundle,
	extractObjIDFunc ExtractObjIDFunc,
) bundle.AgentBundle {
	return &CompleteComplianceBundle{
		BaseCompleteComplianceBundle: base.BaseCompleteComplianceBundle{
			Objects:           make([]*base.GenericCompleteCompliance, 0),
			LeafHubName:       leafHubName,
			BaseBundleVersion: baseBundle.GetVersion(), // ALWAYS SYNCED SINCE POINTER
			BundleVersion:     metadata.NewBundleVersion(),
		},
		baseBundle:       baseBundle,
		extractObjIDFunc: extractObjIDFunc,
		lock:             sync.Mutex{},
	}
}

func NewManagerCompleteComplianceBundle() bundle.ManagerBundle {
	return &CompleteComplianceBundle{}
}

// GetLeafHubName returns the leaf hub name that sent the bundle.
func (bundle *CompleteComplianceBundle) GetLeafHubName() string {
	return bundle.LeafHubName
}

// GetObjects returns the objects in the bundle.
func (bundle *CompleteComplianceBundle) GetObjects() []interface{} {
	result := make([]interface{}, len(bundle.Objects))
	for i, obj := range bundle.Objects {
		result[i] = obj
	}

	return result
}

// GetDependencyVersion returns the bundle dependency required version.
func (bundle *CompleteComplianceBundle) GetDependencyVersion() *metadata.BundleVersion {
	return bundle.BaseBundleVersion
}

func (bundle *CompleteComplianceBundle) SetVersion(version *metadata.BundleVersion) {
	bundle.lock.Lock()
	defer bundle.lock.Unlock()
	bundle.BundleVersion = version
}

// Agent - UpdateObject function to update a single object inside a bundle.
func (b *CompleteComplianceBundle) UpdateObject(object bundle.Object) {
	b.lock.Lock()
	defer b.lock.Unlock()

	policy, isPolicy := object.(*policiesv1.Policy)
	if !isPolicy {
		return // do not handle objects other than policy
	}

	originPolicyID, ok := b.extractObjIDFunc(object)
	if !ok {
		return // cant update the object without finding its id.
	}

	index, err := b.getObjectIndexByUID(originPolicyID)
	if err != nil { // object not found, need to add it to the bundle (only in case it contains non-compliant/unknown)
		policyComplianceObject := b.getPolicyComplianceStatus(originPolicyID, policy)
		// don't send in the bundle a policy where all clusters are compliant
		if b.containsNonCompliantOrUnknownClusters(policyComplianceObject) {
			b.Objects = append(b.Objects, policyComplianceObject)
			b.BundleVersion.Incr() // increase bundle version if objects array was changed
		}

		return
	}
	// if we reached here, policy already exists in the bundle with at least one non compliant or unknown cluster.
	if !b.updateBundleIfObjectChanged(index, policy) {
		return // true if changed, otherwise false. if policy compliance didn't change don't increment version value.
	}

	// don't send in the bundle a policy where all clusters are compliant
	if !b.containsNonCompliantOrUnknownClusters(b.Objects[index]) {
		b.Objects = append(b.Objects[:index], b.Objects[index+1:]...) // remove from objects
	}

	// increase bundle version value in the case where cluster lists were changed
	b.BundleVersion.Incr()
}

// Agent - DeleteObject function to delete a single object inside a bundle.
func (b *CompleteComplianceBundle) DeleteObject(object bundle.Object) {
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

	// do not increase generation, no need to send bundle when policy is removed (clusters per policy bundle is sent).
	b.Objects = append(b.Objects[:index], b.Objects[index+1:]...) // remove from objects
	b.BundleVersion.Incr()
}

// Agent GetBundleVersion function to get bundle version
func (bundle *CompleteComplianceBundle) GetVersion() *metadata.BundleVersion {
	return bundle.BundleVersion
}

func (bundle *CompleteComplianceBundle) getObjectIndexByUID(uid string) (int, error) {
	for i, object := range bundle.Objects {
		if object.PolicyID == uid {
			return i, nil
		}
	}

	return -1, fmt.Errorf("non find object: %s", uid)
}

func (bundle *CompleteComplianceBundle) getPolicyComplianceStatus(originPolicyID string,
	policy *policiesv1.Policy,
) *base.GenericCompleteCompliance {
	nonCompliantClusters, unknownComplianceClusters := bundle.getNonCompliantAndUnknownClusters(policy)

	return &base.GenericCompleteCompliance{
		PolicyID:                  originPolicyID,
		NamespacedName:            policy.Namespace + "/" + policy.Name,
		NonCompliantClusters:      nonCompliantClusters,
		UnknownComplianceClusters: unknownComplianceClusters,
	}
}

// returns a list of non compliant clusters and a list of unknown compliance clusters.
func (bundle *CompleteComplianceBundle) getNonCompliantAndUnknownClusters(policy *policiesv1.Policy) ([]string,
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
func (bundle *CompleteComplianceBundle) updateBundleIfObjectChanged(objectIndex int, policy *policiesv1.Policy) bool {
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

func (bundle *CompleteComplianceBundle) clusterListsEqual(oldClusters []string, newClusters []string) bool {
	if len(oldClusters) != len(newClusters) {
		return false
	}

	for _, newClusterName := range newClusters {
		if !utils.ContainsString(oldClusters, newClusterName) {
			return false
		}
	}

	return true
}

func (bundle *CompleteComplianceBundle) containsNonCompliantOrUnknownClusters(
	policyComplianceStatus *base.GenericCompleteCompliance,
) bool {
	if len(policyComplianceStatus.UnknownComplianceClusters) == 0 &&
		len(policyComplianceStatus.NonCompliantClusters) == 0 {
		return false
	}

	return true
}

func (bundle *CompleteComplianceBundle) getObjectIndexByObj(obj bundle.Object) (int, error) {
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
	return -1, fmt.Errorf("not found object by uid or namespacedName")
}
