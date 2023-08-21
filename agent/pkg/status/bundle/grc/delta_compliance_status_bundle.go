package grc

import (
	"errors"
	"fmt"
	"sync"

	set "github.com/deckarep/golang-set"
	policyv1 "open-cluster-management.io/governance-policy-propagator/api/v1"

	agentbundle "github.com/stolostron/multicluster-global-hub/agent/pkg/status/bundle"
	statusbundle "github.com/stolostron/multicluster-global-hub/pkg/bundle/status"
)

const unknownComplianceStatus = "unknown"

var errPolicyStatusUnchanged = errors.New("policy status did not changed")

// NewDeltaComplianceStatusBundle creates a new instance of DeltaComplianceStatusBundle.
func NewDeltaComplianceStatusBundle(leafHubName string, baseBundle agentbundle.Bundle,
	clustersPerPolicyBundle *ClustersPerPolicyBundle, extractObjIDFunc agentbundle.ExtractObjIDFunc,
) agentbundle.DeltaStateBundle {
	return &DeltaComplianceStatusBundle{
		BaseDeltaComplianceStatusBundle: statusbundle.BaseDeltaComplianceStatusBundle{
			Objects:           make([]*statusbundle.PolicyGenericComplianceStatus, 0),
			LeafHubName:       leafHubName,
			BaseBundleVersion: statusbundle.NewBundleVersion(),
			BundleVersion:     statusbundle.NewBundleVersion(),
		},
		cyclicTransportationBundleID: 0,
		baseBundle:                   baseBundle,
		clustersPerPolicyBaseBundle:  clustersPerPolicyBundle,
		complianceRecordsCache:       make(map[string]*policyComplianceStatus),
		extractObjIDFunc:             extractObjIDFunc,
		lock:                         sync.Mutex{},
	}
}

// DeltaComplianceStatusBundle abstracts management of compliance status bundle.
type DeltaComplianceStatusBundle struct {
	statusbundle.BaseDeltaComplianceStatusBundle
	cyclicTransportationBundleID int
	baseBundle                   agentbundle.Bundle
	clustersPerPolicyBaseBundle  *ClustersPerPolicyBundle
	complianceRecordsCache       map[string]*policyComplianceStatus
	extractObjIDFunc             agentbundle.ExtractObjIDFunc
	lock                         sync.Mutex
}

// policyComplianceStatus holds each policy's full compliance status at all times. It is needed since the
// clustersPerPolicy bundle is updated first (sending order dependency), therefore we cannot use its generic statuses.
type policyComplianceStatus struct {
	compliantClustersSet    set.Set
	nonCompliantClustersSet set.Set
	unknownClustersSet      set.Set
}

// GetTransportationID function to get bundle transportation ID to be attached to message-key during transportation.
func (bundle *DeltaComplianceStatusBundle) GetTransportationID() int {
	bundle.lock.Lock()
	defer bundle.lock.Unlock()

	return bundle.cyclicTransportationBundleID
}

// UpdateObject function to update a single object inside a bundle.
func (bundle *DeltaComplianceStatusBundle) UpdateObject(object agentbundle.Object) {
	bundle.lock.Lock()
	defer bundle.lock.Unlock()

	policy, isPolicy := object.(*policyv1.Policy)
	if !isPolicy {
		return // do not handle objects other than policy
	}

	originPolicyID, ok := bundle.extractObjIDFunc(object)
	if !ok {
		return // can't update the object without finding its id
	}

	// if policy is new then sync what's in the clustersPerPolicy base
	if _, policyHasRecords := bundle.complianceRecordsCache[originPolicyID]; !policyHasRecords {
		bundle.updateSpecificPolicyRecordsFromBase(originPolicyID)
		return
	}

	// get policy compliance status, this will also update info in records
	policyComplianceObject, err := bundle.getPolicyComplianceStatus(originPolicyID, policy)
	if err != nil {
		return // error found means object should be skipped
	}

	index, err := bundle.getObjectIndexByUID(originPolicyID)
	if err != nil { // object not found, need to add it to the bundle
		bundle.Objects = append(bundle.Objects, policyComplianceObject)
		bundle.BundleVersion.Incr()

		return
	}

	// object found, update content
	bundle.updatePolicyComplianceStatus(index, policyComplianceObject)
	bundle.BundleVersion.Incr()
}

// DeleteObject function to delete all instances of a single object inside a bundle.
func (bundle *DeltaComplianceStatusBundle) DeleteObject(object agentbundle.Object) {
	bundle.lock.Lock()
	defer bundle.lock.Unlock()

	_, isPolicy := object.(*policyv1.Policy)
	if !isPolicy {
		return // do not handle objects other than policy
	}

	index, originPolicyID, err := bundle.getObjectIndexByObj(object)
	if err != nil { // trying to delete object which doesn't exist - return with no error
		return
	}

	// do not increase generation, no need to send bundle when policy is removed (clusters per policy bundle is sent)
	bundle.Objects = append(bundle.Objects[:index], bundle.Objects[index+1:]...) // remove from objects

	// delete from policies records and the policies' existence map
	delete(bundle.complianceRecordsCache, originPolicyID)
	bundle.BundleVersion.Incr()
}

// GetBundleVersion function to get bundle version.
func (bundle *DeltaComplianceStatusBundle) GetBundleVersion() *statusbundle.BundleVersion {
	bundle.lock.Lock()
	defer bundle.lock.Unlock()

	return bundle.BundleVersion
}

// SyncState syncs the state of the delta-bundle with the full-state.
func (bundle *DeltaComplianceStatusBundle) SyncState() {
	bundle.lock.Lock()
	defer bundle.lock.Unlock()

	// update version
	bundle.BaseBundleVersion = bundle.baseBundle.GetBundleVersion()

	// update policy records from the ClustersPerPolicy bundle's (full-state) status
	bundle.complianceRecordsCache = make(map[string]*policyComplianceStatus)
	bundle.updatePolicyRecordsFromBase()

	// reset ID since state-sync means base has changed and a new line is starting
	bundle.cyclicTransportationBundleID = 0
}

// Reset flushes the objects in the bundle (after delivery).
func (bundle *DeltaComplianceStatusBundle) Reset() {
	bundle.lock.Lock()
	defer bundle.lock.Unlock()

	bundle.Objects = nil                  // safe since go1.0
	bundle.cyclicTransportationBundleID++ // increment ID since a reset means a new bundle is starting
}

func (bundle *DeltaComplianceStatusBundle) updateSpecificPolicyRecordsFromBase(policyID string) {
	policiesGenericComplianceStatuses := bundle.clustersPerPolicyBaseBundle.BaseClustersPerPolicyBundle.Objects

	for _, genericComplianceStatus := range policiesGenericComplianceStatuses {
		if genericComplianceStatus.PolicyID != policyID {
			continue
		}

		// create new records for policy and fill it with status
		bundle.syncGenericStatus(genericComplianceStatus)

		break // found the policy, no need to continue
	}
}

// updateObjectRecordsFromBase ensures that the delta state objects records are up-to-date when bundle gets enabled.
func (bundle *DeltaComplianceStatusBundle) updatePolicyRecordsFromBase() {
	policiesGenericComplianceStatuses := bundle.clustersPerPolicyBaseBundle.BaseClustersPerPolicyBundle.Objects

	for _, genericComplianceStatus := range policiesGenericComplianceStatuses {
		// create new records for policy and fill it with status
		bundle.syncGenericStatus(genericComplianceStatus)
	}
}

func (bundle *DeltaComplianceStatusBundle) getObjectIndexByUID(uid string) (int, error) {
	for i, object := range bundle.Objects {
		if object.PolicyID == uid {
			return i, nil
		}
	}

	return -1, agentbundle.ErrObjectNotFound
}

// getPolicyComplianceStatus gets compliance statuses of a new policy object (relative to this bundle).
func (bundle *DeltaComplianceStatusBundle) getPolicyComplianceStatus(originPolicyID string,
	policy *policyv1.Policy,
) (*statusbundle.PolicyGenericComplianceStatus, error) {
	compliantClusters, nonCompliantClusters,
		unknownComplianceClusters := bundle.getChangedClusters(policy, originPolicyID)

	if len(compliantClusters)+len(nonCompliantClusters)+len(unknownComplianceClusters) == 0 {
		return nil, errPolicyStatusUnchanged // status flicker / CpP already covered it
	}

	return &statusbundle.PolicyGenericComplianceStatus{
		PolicyID:                  originPolicyID,
		NamespacedName:            fmt.Sprintf("%s/%s", policy.Namespace, policy.Name),
		CompliantClusters:         compliantClusters,
		NonCompliantClusters:      nonCompliantClusters,
		UnknownComplianceClusters: unknownComplianceClusters,
	}, nil
}

// getChangedClusters returns arrays of changed compliance (cluster names).
func (bundle *DeltaComplianceStatusBundle) getChangedClusters(policy *policyv1.Policy,
	policyID string,
) ([]string, []string, []string) {
	compliantClusters := make([]string, 0)
	nonCompliantClusters := make([]string, 0)
	unknownComplianceClusters := make([]string, 0)

	for _, clusterCompliance := range policy.Status.Status {
		switch clusterCompliance.ComplianceState {
		case policyv1.Compliant:
			if bundle.complianceRecordsCache[policyID].compliantClustersSet.Contains(clusterCompliance.ClusterName) {
				continue
			}

			bundle.complianceRecordsCache[policyID].compliantClustersSet.Add(clusterCompliance.ClusterName)
			bundle.complianceRecordsCache[policyID].nonCompliantClustersSet.Remove(clusterCompliance.ClusterName)
			bundle.complianceRecordsCache[policyID].unknownClustersSet.Remove(clusterCompliance.ClusterName)

			compliantClusters = append(compliantClusters, clusterCompliance.ClusterName)
		case policyv1.NonCompliant:
			if bundle.complianceRecordsCache[policyID].nonCompliantClustersSet.Contains(clusterCompliance.ClusterName) {
				continue
			}

			bundle.complianceRecordsCache[policyID].compliantClustersSet.Remove(clusterCompliance.ClusterName)
			bundle.complianceRecordsCache[policyID].nonCompliantClustersSet.Add(clusterCompliance.ClusterName)
			bundle.complianceRecordsCache[policyID].unknownClustersSet.Remove(clusterCompliance.ClusterName)

			nonCompliantClusters = append(nonCompliantClusters, clusterCompliance.ClusterName)
		default:
			if bundle.complianceRecordsCache[policyID].unknownClustersSet.Contains(clusterCompliance.ClusterName) {
				continue
			}

			bundle.complianceRecordsCache[policyID].compliantClustersSet.Remove(clusterCompliance.ClusterName)
			bundle.complianceRecordsCache[policyID].nonCompliantClustersSet.Remove(clusterCompliance.ClusterName)
			bundle.complianceRecordsCache[policyID].unknownClustersSet.Add(clusterCompliance.ClusterName)

			unknownComplianceClusters = append(unknownComplianceClusters, clusterCompliance.ClusterName)
		}
	}

	return compliantClusters, nonCompliantClusters, unknownComplianceClusters
}

// updatePolicyComplianceStatus updates compliance statuses of an already listed policy object.
func (bundle *DeltaComplianceStatusBundle) updatePolicyComplianceStatus(policyIndex int,
	newPolicyStatus *statusbundle.PolicyGenericComplianceStatus,
) {
	// get existing policy state
	existingPolicyState := bundle.getExistingPolicyState(policyIndex)

	// update the policy state above
	for _, cluster := range newPolicyStatus.CompliantClusters {
		existingPolicyState[cluster] = policyv1.Compliant
	}

	for _, cluster := range newPolicyStatus.NonCompliantClusters {
		existingPolicyState[cluster] = policyv1.NonCompliant
	}

	for _, cluster := range newPolicyStatus.UnknownComplianceClusters {
		existingPolicyState[cluster] = unknownComplianceStatus
	}

	// generate new compliance lists from the updated policy state map
	compliantClusters := make([]string, 0)
	nonCompliantClusters := make([]string, 0)
	unknownComplianceClusters := make([]string, 0)

	for cluster, compliance := range existingPolicyState {
		switch compliance {
		case policyv1.Compliant:
			compliantClusters = append(compliantClusters, cluster)
		case policyv1.NonCompliant:
			nonCompliantClusters = append(nonCompliantClusters, cluster)
		default:
			unknownComplianceClusters = append(unknownComplianceClusters, cluster)
		}
	}

	// update policy
	bundle.Objects[policyIndex].CompliantClusters = compliantClusters
	bundle.Objects[policyIndex].NonCompliantClusters = nonCompliantClusters
	bundle.Objects[policyIndex].UnknownComplianceClusters = unknownComplianceClusters
}

// getExistingPolicyState returns a map of [cluster name -> compliance state] for the policy indexed by policyIndex.
func (bundle *DeltaComplianceStatusBundle) getExistingPolicyState(policyIndex int) map[string]policyv1.ComplianceState {
	existingPolicyState := make(map[string]policyv1.ComplianceState)

	for _, cluster := range bundle.Objects[policyIndex].CompliantClusters {
		existingPolicyState[cluster] = policyv1.Compliant
	}

	for _, cluster := range bundle.Objects[policyIndex].NonCompliantClusters {
		existingPolicyState[cluster] = policyv1.NonCompliant
	}

	for _, cluster := range bundle.Objects[policyIndex].UnknownComplianceClusters {
		existingPolicyState[cluster] = unknownComplianceStatus
	}

	return existingPolicyState
}

func (bundle *DeltaComplianceStatusBundle) syncGenericStatus(status *statusbundle.PolicyGenericComplianceStatus) {
	bundle.complianceRecordsCache[status.PolicyID] = &policyComplianceStatus{
		compliantClustersSet:    agentbundle.CreateSetFromSlice(status.CompliantClusters),
		nonCompliantClustersSet: agentbundle.CreateSetFromSlice(status.NonCompliantClusters),
		unknownClustersSet:      agentbundle.CreateSetFromSlice(status.UnknownComplianceClusters),
	}
}

func (bundle *DeltaComplianceStatusBundle) getObjectIndexByObj(obj agentbundle.Object) (int, string, error) {
	uid, _ := bundle.extractObjIDFunc(obj)
	if len(uid) > 0 {
		for i, object := range bundle.Objects {
			if uid == string(object.PolicyID) {
				return i, uid, nil
			}
		}
	} else {
		for i, object := range bundle.Objects {
			if string(object.NamespacedName) == fmt.Sprintf("%s/%s", obj.GetNamespace(), obj.GetName()) {
				return i, object.PolicyID, nil
			}
		}
	}
	return -1, "", agentbundle.ErrObjectNotFound
}
