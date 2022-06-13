package db

import set "github.com/deckarep/golang-set"

// NewPolicyClusterSets creates a new instance of PolicyClustersSets.
func NewPolicyClusterSets() *PolicyClustersSets {
	return &PolicyClustersSets{
		complianceToSetMap: map[ComplianceStatus]set.Set{
			Compliant:    set.NewSet(),
			NonCompliant: set.NewSet(),
			Unknown:      set.NewSet(),
		},
	}
}

// PolicyClustersSets is a data structure to hold both non compliant clusters set and unknown clusters set.
type PolicyClustersSets struct {
	complianceToSetMap map[ComplianceStatus]set.Set
}

// AddCluster adds the given cluster name to the given compliance status clusters set.
func (sets *PolicyClustersSets) AddCluster(clusterName string, complianceStatus ComplianceStatus) {
	sets.complianceToSetMap[complianceStatus].Add(clusterName)
}

// GetAllClusters returns the clusters set of a policy (union of compliant/nonCompliant/unknown clusters).
func (sets *PolicyClustersSets) GetAllClusters() set.Set {
	return sets.complianceToSetMap[Compliant].
		Union(sets.complianceToSetMap[NonCompliant].
			Union(sets.complianceToSetMap[Unknown]))
}

// GetClusters returns the clusters set by compliance status.
func (sets *PolicyClustersSets) GetClusters(complianceStatus ComplianceStatus) set.Set {
	return sets.complianceToSetMap[complianceStatus]
}
