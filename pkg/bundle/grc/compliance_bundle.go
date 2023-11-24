package grc

import (
	"errors"
	"fmt"
	"sync"

	policiesv1 "open-cluster-management.io/governance-policy-propagator/api/v1"

	"github.com/stolostron/multicluster-global-hub/pkg/bundle"
	"github.com/stolostron/multicluster-global-hub/pkg/bundle/base"
	"github.com/stolostron/multicluster-global-hub/pkg/bundle/metadata"
	utils "github.com/stolostron/multicluster-global-hub/pkg/utils"
)

var (
	_ bundle.AgentBundle   = (*ComplianceBundle)(nil)
	_ bundle.ManagerBundle = (*ComplianceBundle)(nil)
)

type ExtractObjIDFunc func(obj bundle.Object) (string, bool)

// ComplianceBundle abstracts management of clusters per policy bundle.
type ComplianceBundle struct {
	base.BaseComplianceBundle

	// agent bundle properties
	extractObjIDFunc ExtractObjIDFunc
	lock             sync.Mutex
}

// NewAgentComplianceBundle creates a new instance of ComplianceBundle.
func NewAgentComplianceBundle(leafHubName string, extractObjIDFunc ExtractObjIDFunc) bundle.AgentBundle {
	return &ComplianceBundle{
		BaseComplianceBundle: base.BaseComplianceBundle{
			Objects:       make([]*base.GenericCompliance, 0),
			LeafHubName:   leafHubName,
			BundleVersion: metadata.NewBundleVersion(),
		},
		extractObjIDFunc: extractObjIDFunc,
	}
}

// NewClustersPerPolicyBundle creates a new instance of ClustersPerPolicyBundle.
func NewManagerComplianceBundle() bundle.ManagerBundle {
	return &ComplianceBundle{}
}

// Manager - GetLeafHubName returns the leaf hub name that sent the bundle.
func (b *ComplianceBundle) GetLeafHubName() string {
	return b.LeafHubName
}

// GetObjects returns the objects in the bundle.
func (b *ComplianceBundle) GetObjects() []interface{} {
	result := make([]interface{}, len(b.Objects))
	for i, obj := range b.Objects {
		result[i] = obj
	}

	return result
}

// Manager
func (b *ComplianceBundle) SetVersion(version *metadata.BundleVersion) {
	b.lock.Lock()
	defer b.lock.Unlock()
	b.BundleVersion = version
}

// Both Manager and Agent - GetVersion returns the bundle version.
func (b *ComplianceBundle) GetVersion() *metadata.BundleVersion {
	return b.BundleVersion
}

// Agent - UpdateObject function to update a single object inside a bundle.
func (b *ComplianceBundle) UpdateObject(object bundle.Object) {
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
	if err != nil { // object not found, need to add it to the bundle
		b.Objects = append(b.Objects, b.getNewCompliance(originPolicyID, policy))
		b.BundleVersion.Incr()
		return
	}
	// when we update a policy, we need to increase bundle generation only if cluster list of the policy has changed.
	// for the use case where no cluster was added/removed, we use the status compliance bundle to update hub of hubs
	// and not the clusters per policy bundle which contains a lot more information (full state).
	//
	// that being said, we still want to update the internal data and keep it always up to date in case a policy will be
	// inserted/removed (or cluster added/removed) and full state bundle will be triggered.
	if b.updateObjectIfChanged(index, policy) { // returns true if cluster list has changed, otherwise false
		b.BundleVersion.Incr()
	}
}

// Agent - DeleteObject function to delete a single object inside a bundle.
func (b *ComplianceBundle) DeleteObject(object bundle.Object) {
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

func (b *ComplianceBundle) getObjectIndexByUID(uid string) (int, error) {
	for i, object := range b.Objects {
		if object.PolicyID == uid {
			return i, nil
		}
	}
	return -1, errors.New("object not found")
}

// getClusterStatuses returns (list of compliant clusters, list of nonCompliant clusters, list of unknown clusters,
// list of all clusters).
func (b *ComplianceBundle) getClusterStatuses(policy *policiesv1.Policy) ([]string, []string, []string,
	[]string,
) {
	compliantClusters := make([]string, 0)
	nonCompliantClusters := make([]string, 0)
	unknownComplianceClusters := make([]string, 0)
	allClusters := make([]string, 0)

	for _, clusterStatus := range policy.Status.Status {
		if clusterStatus.ComplianceState == policiesv1.Compliant {
			compliantClusters = append(compliantClusters, clusterStatus.ClusterName)
			allClusters = append(allClusters, clusterStatus.ClusterName)

			continue
		}
		// else
		if clusterStatus.ComplianceState == policiesv1.NonCompliant {
			nonCompliantClusters = append(nonCompliantClusters, clusterStatus.ClusterName)
			allClusters = append(allClusters, clusterStatus.ClusterName)

			continue
		}
		// else
		unknownComplianceClusters = append(unknownComplianceClusters, clusterStatus.ClusterName)
		allClusters = append(allClusters, clusterStatus.ClusterName)
	}

	return compliantClusters, nonCompliantClusters, unknownComplianceClusters, allClusters
}

func (b *ComplianceBundle) getNewCompliance(originPolicyID string, policy *policiesv1.Policy,
) *base.GenericCompliance {
	compliantClusters, nonCompliantClusters, unknownComplianceClusters, _ := b.getClusterStatuses(policy)
	return &base.GenericCompliance{
		PolicyID:                  originPolicyID,
		NamespacedName:            fmt.Sprintf("%s/%s", policy.GetNamespace(), policy.GetName()),
		CompliantClusters:         compliantClusters,
		NonCompliantClusters:      nonCompliantClusters,
		UnknownComplianceClusters: unknownComplianceClusters,
	}
}

// returns true if cluster list has changed, otherwise returns false (even if cluster statuses changed).
func (b *ComplianceBundle) updateObjectIfChanged(objectIndex int, policy *policiesv1.Policy) bool {
	newCompliantClusters, newNonCompliantClusters, newUnknownClusters, allClusters := b.getClusterStatuses(policy)
	oldPolicyStatus := b.Objects[objectIndex]
	clusterListChanged := false

	// check if any cluster was added or removed
	if len(oldPolicyStatus.CompliantClusters)+len(oldPolicyStatus.NonCompliantClusters)+
		len(oldPolicyStatus.UnknownComplianceClusters) != len(allClusters) ||
		!b.clusterListContains(oldPolicyStatus.CompliantClusters, allClusters) ||
		!b.clusterListContains(oldPolicyStatus.NonCompliantClusters, allClusters) ||
		!b.clusterListContains(oldPolicyStatus.UnknownComplianceClusters, allClusters) {
		clusterListChanged = true // at least one cluster was added/removed
	}

	// in any case we want to update the internal bundle in case statuses changed
	oldPolicyStatus.CompliantClusters = newCompliantClusters
	oldPolicyStatus.NonCompliantClusters = newNonCompliantClusters
	oldPolicyStatus.UnknownComplianceClusters = newUnknownClusters

	return clusterListChanged
}

func (b *ComplianceBundle) clusterListContains(subsetClusters []string, allClusters []string) bool {
	for _, clusterName := range subsetClusters {
		if !utils.ContainsString(allClusters, clusterName) {
			return false
		}
	}

	return true
}

func (b *ComplianceBundle) getObjectIndexByObj(obj bundle.Object) (int, error) {
	uid, _ := b.extractObjIDFunc(obj)
	if len(uid) > 0 {
		for i, object := range b.Objects {
			if uid == string(object.PolicyID) {
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

	return -1, errors.New("object not found by id or namespacedName")
}
