package dbsyncer

import (
	set "github.com/deckarep/golang-set"

	"github.com/stolostron/multicluster-global-hub/pkg/database"
)

// NewPolicyClusterSets creates a new instance of PolicyClustersSets.
func NewPolicyClusterSets() *PolicyClustersSets {
	return &PolicyClustersSets{
		complianceToSetMap: map[database.ComplianceStatus]set.Set{
			database.Compliant:    set.NewSet(),
			database.NonCompliant: set.NewSet(),
			database.Unknown:      set.NewSet(),
		},
	}
}

// PolicyClustersSets is a data structure to hold both non compliant clusters set and unknown clusters set.
type PolicyClustersSets struct {
	complianceToSetMap map[database.ComplianceStatus]set.Set
}

// AddCluster adds the given cluster name to the given compliance status clusters set.
func (sets *PolicyClustersSets) AddCluster(clusterName string, complianceStatus database.ComplianceStatus) {
	sets.complianceToSetMap[complianceStatus].Add(clusterName)
}

// GetAllClusters returns the clusters set of a policy (union of compliant/nonCompliant/unknown clusters).
func (sets *PolicyClustersSets) GetAllClusters() set.Set {
	return sets.complianceToSetMap[database.Compliant].
		Union(sets.complianceToSetMap[database.NonCompliant].
			Union(sets.complianceToSetMap[database.Unknown]))
}

// GetClusters returns the clusters set by compliance status.
func (sets *PolicyClustersSets) GetClusters(complianceStatus database.ComplianceStatus) set.Set {
	return sets.complianceToSetMap[complianceStatus]
}
