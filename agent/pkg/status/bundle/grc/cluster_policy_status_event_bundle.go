package grc

import (
	"fmt"
	"sync"

	policiesv1 "open-cluster-management.io/governance-policy-propagator/api/v1"
	"sigs.k8s.io/controller-runtime/pkg/client"

	bundlepkg "github.com/stolostron/multicluster-global-hub/agent/pkg/status/bundle"
	statusbundle "github.com/stolostron/multicluster-global-hub/pkg/bundle/status"
)

// NewClustersPerPolicyBundle creates a new instance of ClustersPerPolicyBundle.
func NewClusterPolicyHistoryEventBundle(leafHubName string, incarnation uint64,
	runtimeClient client.Client,
) bundlepkg.Bundle {
	return &ClusterPolicyHistoryEventBundle{
		BaseClusterPolicyHistoryEventBundle: statusbundle.BaseClusterPolicyHistoryEventBundle{
			PolicyStatusEvents: make(map[string]*statusbundle.BaseClusterPolicyHistoryEventBundle),
			LeafHubName:        leafHubName,
			BundleVersion:      statusbundle.NewBundleVersion(incarnation, 0),
		},
		lock:          sync.Mutex{},
		runtimeClient: runtimeClient,
	}
}

type ClusterPolicyHistoryEventBundle struct {
	statusbundle.BaseClusterPolicyHistoryEventBundle
	lock          sync.Mutex
	runtimeClient client.Client
}

// UpdateObject function to update a single object inside a bundle.
func (bundle *ClusterPolicyHistoryEventBundle) UpdateObject(object bundlepkg.Object) {
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
	if err != nil { // object not found, need to add it to the bundle
		bundle.Objects = append(bundle.Objects,
			bundle.getClustersPerPolicy(originPolicyID, policy))
		bundle.BundleVersion.Generation++

		return
	}
	// when we update a policy, we need to increase bundle generation only if cluster list of the policy has changed.
	// for the use case where no cluster was added/removed, we use the status compliance bundle to update hub of hubs
	// and not the clusters per policy bundle which contains a lot more information (full state).
	//
	// that being said, we still want to update the internal data and keep it always up to date in case a policy will be
	// inserted/removed (or cluster added/removed) and full state bundle will be triggered.
	if bundle.updateObjectIfChanged(index, policy) { // returns true if cluster list has changed, otherwise false
		bundle.BundleVersion.Generation++
	}
}

// DeleteObject function to delete a single object inside a bundle.
func (bundle *ClusterPolicyHistoryEventBundle) DeleteObject(object bundlepkg.Object) {
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

	bundle.Objects = append(bundle.Objects[:index], bundle.Objects[index+1:]...) // remove from objects
	bundle.BundleVersion.Generation++
}

// GetBundleVersion function to get bundle version.
func (bundle *ClusterPolicyHistoryEventBundle) GetBundleVersion() *statusbundle.BundleVersion {
	bundle.lock.Lock()
	defer bundle.lock.Unlock()

	return bundle.BundleVersion
}

func (bundle *ClusterPolicyHistoryEventBundle) getObjectIndexByUID(uid string) (int, error) {
	for i, object := range bundle.Objects {
		if object.PolicyID == uid {
			return i, nil
		}
	}

	return -1, bundlepkg.ErrObjectNotFound
}

// getClusterStatuses returns (list of compliant clusters, list of nonCompliant clusters, list of unknown clusters,
// list of all clusters).
func (bundle *ClusterPolicyHistoryEventBundle) getClusterStatuses(policy *policiesv1.Policy) ([]string, []string, []string,
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

func (bundle *ClusterPolicyHistoryEventBundle) getClustersPerPolicy(originPolicyID string,
	policy *policiesv1.Policy,
) *statusbundle.PolicyGenericComplianceStatus {
	compliantClusters, nonCompliantClusters, unknownComplianceClusters, _ := bundle.getClusterStatuses(policy)

	return &statusbundle.PolicyGenericComplianceStatus{
		PolicyID:                  originPolicyID,
		NamespacedName:            fmt.Sprintf("%s/%s", policy.GetNamespace(), policy.GetName()),
		CompliantClusters:         compliantClusters,
		NonCompliantClusters:      nonCompliantClusters,
		UnknownComplianceClusters: unknownComplianceClusters,
	}
}

// returns true if cluster list has changed, otherwise returns false (even if cluster statuses changed).
func (bundle *ClusterPolicyHistoryEventBundle) updateObjectIfChanged(objectIndex int, policy *policiesv1.Policy) bool {
	newCompliantClusters, newNonCompliantClusters, newUnknownClusters, newClusters := bundle.getClusterStatuses(policy)
	oldPolicyStatus := bundle.Objects[objectIndex]
	clusterListChanged := false

	// check if any cluster was added or removed
	if len(oldPolicyStatus.CompliantClusters)+len(oldPolicyStatus.NonCompliantClusters)+
		len(oldPolicyStatus.UnknownComplianceClusters) != len(newClusters) ||
		!bundle.clusterListContains(oldPolicyStatus.CompliantClusters, newClusters) ||
		!bundle.clusterListContains(oldPolicyStatus.NonCompliantClusters, newClusters) ||
		!bundle.clusterListContains(oldPolicyStatus.UnknownComplianceClusters, newClusters) {
		clusterListChanged = true // at least one cluster was added/removed
	}

	// in any case we want to update the internal bundle in case statuses changed
	oldPolicyStatus.CompliantClusters = newCompliantClusters
	oldPolicyStatus.NonCompliantClusters = newNonCompliantClusters
	oldPolicyStatus.UnknownComplianceClusters = newUnknownClusters

	return clusterListChanged
}

func (bundle *ClusterPolicyHistoryEventBundle) clusterListContains(subsetClusters []string, allClusters []string) bool {
	for _, clusterName := range subsetClusters {
		if !bundlepkg.ContainsString(allClusters, clusterName) {
			return false
		}
	}

	return true
}

func (bundle *ClusterPolicyHistoryEventBundle) getObjectIndexByObj(obj bundlepkg.Object) (int, error) {
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
