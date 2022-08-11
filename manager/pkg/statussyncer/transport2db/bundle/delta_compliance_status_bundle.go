package bundle

import (
	"errors"
	"fmt"

	set "github.com/deckarep/golang-set"

	"github.com/stolostron/multicluster-globalhub/pkg/bundle/status"
)

var errWrongType = errors.New("received invalid type")

// NewDeltaComplianceStatusBundle creates a new instance of DeltaComplianceStatusBundle.
func NewDeltaComplianceStatusBundle() Bundle {
	return &DeltaComplianceStatusBundle{}
}

// DeltaComplianceStatusBundle abstracts management of delta compliance status bundle.
type DeltaComplianceStatusBundle struct {
	status.BaseDeltaComplianceStatusBundle
}

// GetLeafHubName returns the leaf hub name that sent the bundle.
func (bundle *DeltaComplianceStatusBundle) GetLeafHubName() string {
	return bundle.LeafHubName
}

// GetObjects returns the objects in the bundle.
func (bundle *DeltaComplianceStatusBundle) GetObjects() []interface{} {
	result := make([]interface{}, len(bundle.Objects))

	for i, obj := range bundle.Objects {
		result[i] = obj
	}

	return result
}

// GetVersion returns the bundle version.
func (bundle *DeltaComplianceStatusBundle) GetVersion() *status.BundleVersion {
	return bundle.BundleVersion
}

// GetDependencyVersion returns the bundle dependency required version.
func (bundle *DeltaComplianceStatusBundle) GetDependencyVersion() *status.BundleVersion {
	return bundle.BaseBundleVersion
}

// InheritEvents updates the content of this bundle with that of another older one (this bundle is the source of truth).
func (bundle *DeltaComplianceStatusBundle) InheritEvents(olderBundle Bundle) error {
	if olderBundle == nil {
		return nil
	}

	oldDeltaComplianceBundle, ok := olderBundle.(*DeltaComplianceStatusBundle)
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

func (bundle *DeltaComplianceStatusBundle) inheritObjects(oldObjects []*status.PolicyGenericComplianceStatus) {
	policiesMap := make(map[string]*policyStatus, len(bundle.Objects))
	survivingOldPolicies := make([]*status.PolicyGenericComplianceStatus, 0, len(oldObjects))

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

func newPolicyStatus(policyGenericStatus *status.PolicyGenericComplianceStatus) *policyStatus {
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
func (ps *policyStatus) appendNewClusters(policyGenericStatus *status.PolicyGenericComplianceStatus) {
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

func updatePolicyStatusInBundle(policyGenericStatus *status.PolicyGenericComplianceStatus,
	policyStatus *policyStatus,
) {
	policyGenericStatus.CompliantClusters = createSliceFromSet(policyStatus.compliantClusters)
	policyGenericStatus.NonCompliantClusters =
		createSliceFromSet(policyStatus.nonCompliantClusters)
	policyGenericStatus.UnknownComplianceClusters =
		createSliceFromSet(policyStatus.unknownClusters)
}
