package grc

import (
	"errors"
	"fmt"
	"sync"

	set "github.com/deckarep/golang-set"
	policyv1 "open-cluster-management.io/governance-policy-propagator/api/v1"

	"github.com/stolostron/multicluster-global-hub/pkg/bundle"
	"github.com/stolostron/multicluster-global-hub/pkg/bundle/base"
	"github.com/stolostron/multicluster-global-hub/pkg/bundle/metadata"
	utils "github.com/stolostron/multicluster-global-hub/pkg/utils"
)

var (
	_ bundle.AgentDeltaBundle   = (*DeltaComplianceBundle)(nil)
	_ bundle.ManagerDeltaBundle = (*DeltaComplianceBundle)(nil)
)

var (
	errPolicyStatusUnchanged = errors.New("policy status did not changed")
	errWrongType             = errors.New("received invalid type")
)

const unknownComplianceStatus = "unknown"

// DeltaComplianceBundle abstracts management of compliance status bundle.
type DeltaComplianceBundle struct {
	base.BaseDeltaComplianceBundle
	cyclicTransportationBundleID int
	baseBundle                   bundle.AgentBundle
	complianceBundle             *ComplianceBundle
	complianceRecordsCache       map[string]*policyComplianceStatus
	extractObjIDFunc             ExtractObjIDFunc
	lock                         sync.Mutex
}

// NewAgentDeltaComplianceBundle creates a new instance of DeltaCompliancestatus.
func NewAgentDeltaComplianceBundle(leafHubName string, baseBundle bundle.AgentBundle,
	complianceBundle *ComplianceBundle, extractObjIDFunc ExtractObjIDFunc,
) bundle.AgentDeltaBundle {
	return &DeltaComplianceBundle{
		BaseDeltaComplianceBundle: base.BaseDeltaComplianceBundle{
			Objects:           make([]*base.GenericCompliance, 0),
			LeafHubName:       leafHubName,
			BaseBundleVersion: baseBundle.GetVersion(),
			BundleVersion:     metadata.NewBundleVersion(),
		},
		cyclicTransportationBundleID: 0,
		baseBundle:                   baseBundle,
		complianceBundle:             complianceBundle,
		complianceRecordsCache:       make(map[string]*policyComplianceStatus),
		extractObjIDFunc:             extractObjIDFunc,
	}
}

func NewManagerDeltaComplianceBundle() bundle.ManagerBundle {
	return &DeltaComplianceBundle{}
}

// policyComplianceStatus holds each policy's full compliance status at all times. It is needed since the
// clustersPerPolicy bundle is updated first (sending order dependency), therefore we cannot use its generic statuses.
type policyComplianceStatus struct {
	compliantClustersSet    set.Set
	nonCompliantClustersSet set.Set
	unknownClustersSet      set.Set
}

// Manager - GetLeafHubName returns the leaf hub name that sent the bundle.
func (bundle *DeltaComplianceBundle) GetLeafHubName() string {
	return bundle.LeafHubName
}

// Manager GetObjects returns the objects in the bundle.
func (bundle *DeltaComplianceBundle) GetObjects() []interface{} {
	result := make([]interface{}, len(bundle.Objects))

	for i, obj := range bundle.Objects {
		result[i] = obj
	}

	return result
}

// Manager - GetDependencyVersion returns the bundle dependency required version.
func (bundle *DeltaComplianceBundle) GetDependencyVersion() *metadata.BundleVersion {
	return bundle.BaseBundleVersion
}

// Manager InheritEvents updates the content of this bundle with that of another older one
// (this bundle is the source of truth).
func (bundle *DeltaComplianceBundle) InheritEvents(olderBundle bundle.ManagerBundle) error {
	if olderBundle == nil {
		return nil
	}

	oldDeltaComplianceBundle, ok := olderBundle.(*DeltaComplianceBundle)
	if !ok {
		return fmt.Errorf("%w - expecting %s", errWrongType, "DeltaComplianceStatusBundle")
	}

	if !oldDeltaComplianceBundle.GetDependencyVersion().Equals(bundle.GetDependencyVersion()) {
		// if old bundle's dependency version is not equal then its content is covered by a complete-state baseline.
		return nil
	}

	bundle.inheritObjects(oldDeltaComplianceBundle.Objects)

	return nil
}

func (bundle *DeltaComplianceBundle) inheritObjects(oldObjects []*base.GenericCompliance) {
	policiesMap := make(map[string]*policyStatus, len(bundle.Objects))
	survivingOldPolicies := make([]*base.GenericCompliance, 0, len(oldObjects))

	// create policy-info for my policies
	for _, policyGenericStatus := range bundle.Objects {
		policiesMap[policyGenericStatus.PolicyID] = newPolicyStatus(policyGenericStatus)
	}

	// go over old bundle policies (inherited) and add those missing / clusters whose statuses are not mapped
	for _, policyGenericStatus := range oldObjects {
		policyInfo, found := policiesMap[policyGenericStatus.PolicyID]
		if !found {
			// policy was not mapped, add it whole
			survivingOldPolicies = append(survivingOldPolicies, policyGenericStatus)

			continue
		}

		// policy exists in map, add clusters that do not exist currently
		policyInfo.appendNewClusters(policyGenericStatus)
	}

	// turn updated policy-info collections back into policy generic-statuses
	for _, policyGenericStatus := range bundle.Objects {
		updatePolicyStatusInBundle(policyGenericStatus,
			policiesMap[policyGenericStatus.PolicyID])
	}

	// update bundle's objects with the surviving policies as-is
	bundle.Objects = append(survivingOldPolicies, bundle.Objects...)
}

func newPolicyStatus(policyGenericStatus *base.GenericCompliance) *policyStatus {
	return &policyStatus{
		compliantClusters:    createSetFromSlice(policyGenericStatus.CompliantClusters),
		nonCompliantClusters: createSetFromSlice(policyGenericStatus.NonCompliantClusters),
		unknownClusters:      createSetFromSlice(policyGenericStatus.UnknownComplianceClusters),
	}
}

type policyStatus struct {
	compliantClusters    set.Set
	nonCompliantClusters set.Set
	unknownClusters      set.Set
}

// appendNewClusters gets a policy-status of an old bundle (inherited) and updates self's content with
// clusters that are not mapped currently. (e.g. if a cluster is now compliant and in the old policy
// received it is non-compliant, nothing happens).
func (ps *policyStatus) appendNewClusters(policyGenericStatus *base.GenericCompliance) {
	for _, cluster := range policyGenericStatus.CompliantClusters {
		if !ps.contains(cluster) {
			ps.compliantClusters.Add(cluster)
		}
	}

	for _, cluster := range policyGenericStatus.NonCompliantClusters {
		if !ps.contains(cluster) {
			ps.nonCompliantClusters.Add(cluster)
		}
	}

	for _, cluster := range policyGenericStatus.UnknownComplianceClusters {
		if !ps.contains(cluster) {
			ps.unknownClusters.Add(cluster)
		}
	}
}

func (ps *policyStatus) contains(cluster string) bool {
	if ps.unknownClusters.Contains(cluster) || ps.nonCompliantClusters.Contains(cluster) ||
		ps.compliantClusters.Contains(cluster) {
		return true
	}

	return false
}

func updatePolicyStatusInBundle(policyGenericStatus *base.GenericCompliance,
	policyStatus *policyStatus,
) {
	policyGenericStatus.CompliantClusters = createSliceFromSet(policyStatus.compliantClusters)
	policyGenericStatus.NonCompliantClusters = createSliceFromSet(policyStatus.nonCompliantClusters)
	policyGenericStatus.UnknownComplianceClusters = createSliceFromSet(policyStatus.unknownClusters)
}

func (bundle *DeltaComplianceBundle) SetVersion(version *metadata.BundleVersion) {
	bundle.BundleVersion = version
}

// Agent - GetTransportationID function to get bundle transportation ID to be attached to message-key
// during transportation.
func (b *DeltaComplianceBundle) GetTransportationID() int {
	b.lock.Lock()
	defer b.lock.Unlock()

	return b.cyclicTransportationBundleID
}

// Agent - UpdateObject function to update a single object inside a bundle.
func (b *DeltaComplianceBundle) UpdateObject(object bundle.Object) {
	b.lock.Lock()
	defer b.lock.Unlock()

	policy, isPolicy := object.(*policyv1.Policy)
	if !isPolicy {
		return // do not handle objects other than policy
	}

	originPolicyID, ok := b.extractObjIDFunc(object)
	if !ok {
		return // can't update the object without finding its id
	}

	// if policy is new then sync what's in the clustersPerPolicy base
	if _, policyHasRecords := b.complianceRecordsCache[originPolicyID]; !policyHasRecords {
		b.updateSpecificPolicyRecordsFromBase(originPolicyID)
		return
	}

	// get policy compliance status, this will also update info in records
	policyComplianceObject, err := b.getPolicyComplianceStatus(originPolicyID, policy)
	if err != nil {
		return // error found means object should be skipped
	}

	index, err := b.getObjectIndexByUID(originPolicyID)
	if err != nil { // object not found, need to add it to the bundle
		b.Objects = append(b.Objects, policyComplianceObject)
		b.BundleVersion.Incr()

		return
	}

	// object found, update content
	b.updatePolicyComplianceStatus(index, policyComplianceObject)
	b.BundleVersion.Incr()
}

// Agent - DeleteObject function to delete all instances of a single object inside a bundle.
func (bundle *DeltaComplianceBundle) DeleteObject(object bundle.Object) {
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

// Agent - GetBundleVersion function to get bundle version.
func (bundle *DeltaComplianceBundle) GetVersion() *metadata.BundleVersion {
	return bundle.BundleVersion
}

// Agent - SyncState syncs the state of the delta-bundle with the full-state.
func (bundle *DeltaComplianceBundle) SyncState() {
	bundle.lock.Lock()
	defer bundle.lock.Unlock()

	// update version
	bundle.BaseBundleVersion = bundle.baseBundle.GetVersion()

	// update policy records from the ClustersPerPolicy bundle's (full-state) status
	bundle.complianceRecordsCache = make(map[string]*policyComplianceStatus)
	bundle.updatePolicyRecordsFromBase()

	// reset ID since state-sync means base has changed and a new line is starting
	bundle.cyclicTransportationBundleID = 0
}

// Agent - Reset flushes the objects in the bundle (after delivery).
func (bundle *DeltaComplianceBundle) Reset() {
	bundle.lock.Lock()
	defer bundle.lock.Unlock()

	bundle.Objects = nil                  // safe since go1.0
	bundle.cyclicTransportationBundleID++ // increment ID since a reset means a new bundle is starting
}

func (bundle *DeltaComplianceBundle) updateSpecificPolicyRecordsFromBase(policyID string) {
	policiesGenericComplianceStatuses := bundle.complianceBundle.BaseComplianceBundle.Objects

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
func (bundle *DeltaComplianceBundle) updatePolicyRecordsFromBase() {
	policiesGenericComplianceStatuses := bundle.complianceBundle.BaseComplianceBundle.Objects

	for _, genericComplianceStatus := range policiesGenericComplianceStatuses {
		// create new records for policy and fill it with status
		bundle.syncGenericStatus(genericComplianceStatus)
	}
}

func (bundle *DeltaComplianceBundle) getObjectIndexByUID(uid string) (int, error) {
	for i, object := range bundle.Objects {
		if object.PolicyID == uid {
			return i, nil
		}
	}

	return -1, fmt.Errorf("not found obj: %s", uid)
}

// getPolicyComplianceStatus gets compliance statuses of a new policy object (relative to this bundle).
func (bundle *DeltaComplianceBundle) getPolicyComplianceStatus(originPolicyID string,
	policy *policyv1.Policy,
) (*base.GenericCompliance, error) {
	compliantClusters, nonCompliantClusters,
		unknownComplianceClusters := bundle.getChangedClusters(policy, originPolicyID)

	if len(compliantClusters)+len(nonCompliantClusters)+len(unknownComplianceClusters) == 0 {
		return nil, errPolicyStatusUnchanged // status flicker / CpP already covered it
	}

	return &base.GenericCompliance{
		PolicyID:                  originPolicyID,
		NamespacedName:            fmt.Sprintf("%s/%s", policy.Namespace, policy.Name),
		CompliantClusters:         compliantClusters,
		NonCompliantClusters:      nonCompliantClusters,
		UnknownComplianceClusters: unknownComplianceClusters,
	}, nil
}

// getChangedClusters returns arrays of changed compliance (cluster names).
func (bundle *DeltaComplianceBundle) getChangedClusters(policy *policyv1.Policy,
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
func (bundle *DeltaComplianceBundle) updatePolicyComplianceStatus(policyIndex int,
	newPolicyStatus *base.GenericCompliance,
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
func (bundle *DeltaComplianceBundle) getExistingPolicyState(policyIndex int) map[string]policyv1.ComplianceState {
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

func (bundle *DeltaComplianceBundle) syncGenericStatus(status *base.GenericCompliance) {
	bundle.complianceRecordsCache[status.PolicyID] = &policyComplianceStatus{
		compliantClustersSet:    utils.CreateSetFromSlice(status.CompliantClusters),
		nonCompliantClustersSet: utils.CreateSetFromSlice(status.NonCompliantClusters),
		unknownClustersSet:      utils.CreateSetFromSlice(status.UnknownComplianceClusters),
	}
}

func (bundle *DeltaComplianceBundle) getObjectIndexByObj(obj bundle.Object) (int, string, error) {
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
	return -1, "", fmt.Errorf("not found obj by uid or namespacedName")
}

// createSetFromSlice returns a set contains all items in the given slice. if slice is nil, returns empty set.
func createSetFromSlice(slice []string) set.Set {
	if slice == nil {
		return set.NewSet()
	}

	result := set.NewSet()
	for _, item := range slice {
		result.Add(item)
	}

	return result
}

// createSliceFromSet returns a slice of strings contains all items in the given set of strings. If set is nil, returns
// empty slice. If the set contains a non-string element, returns empty slice.
func createSliceFromSet(set set.Set) []string {
	if set == nil {
		return []string{}
	}

	result := make([]string, 0, set.Cardinality())

	for elem := range set.Iter() {
		elemString, ok := elem.(string)
		if !ok {
			return []string{}
		}

		result = append(result, elemString)
	}

	return result
}

func (bundle *DeltaComplianceBundle) IncrVersion() {
	bundle.lock.Lock()
	defer bundle.lock.Unlock()
	bundle.BundleVersion.Incr()
}
