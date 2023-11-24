package dbsyncer

import (
	"context"

	set "github.com/deckarep/golang-set"
	"github.com/go-logr/logr"

	"github.com/stolostron/multicluster-global-hub/pkg/bundle"
	"github.com/stolostron/multicluster-global-hub/pkg/bundle/grc"
	"github.com/stolostron/multicluster-global-hub/pkg/bundle/metadata"
	"github.com/stolostron/multicluster-global-hub/pkg/conflator"
	"github.com/stolostron/multicluster-global-hub/pkg/conflator/dependency"
	"github.com/stolostron/multicluster-global-hub/pkg/constants"
	"github.com/stolostron/multicluster-global-hub/pkg/database"
	"github.com/stolostron/multicluster-global-hub/pkg/transport/registration"
)

const failedBatchFormat = "failed to perform batch - %w"

// CreateBundleFunction function that specifies how to create a bundle.
type CreateBundleFunction func() bundle.ManagerBundle

// CompliancesDBSyncer implements policies db sync business logic.
type CompliancesDBSyncer struct {
	log                                     logr.Logger
	createComplianceBundleFunc              CreateBundleFunction
	createCompleteComplianceBundleFunc      CreateBundleFunction
	createDeltaComplianceFunc               CreateBundleFunction
	createMinimalComplianceBundleFunc       CreateBundleFunction
	createLocalComplianceBundleFunc         CreateBundleFunction
	createLocalCompleteComplianceBundleFunc CreateBundleFunction
}

// NewCompliancesDBSyncer creates a new instance of PoliciesDBSyncer.
func NewCompliancesDBSyncer(log logr.Logger) Syncer {
	dbSyncer := &CompliancesDBSyncer{
		log:                                     log,
		createComplianceBundleFunc:              grc.NewManagerComplianceBundle,
		createCompleteComplianceBundleFunc:      grc.NewManagerCompleteComplianceBundle,
		createDeltaComplianceFunc:               grc.NewManagerDeltaComplianceBundle,
		createMinimalComplianceBundleFunc:       grc.NewManagerMinimalComplianceBundle,
		createLocalComplianceBundleFunc:         grc.NewManagerLocalComplianceBundle,
		createLocalCompleteComplianceBundleFunc: grc.NewManagerLocalCompleteComplianceBundle,
	}

	log.Info("initialized policies db syncer")

	return dbSyncer
}

// RegisterCreateBundleFunctions registers create bundle functions within the transport instance.
func (syncer *CompliancesDBSyncer) RegisterCreateBundleFunctions(transportDispatcher BundleRegisterable) {
	fullStatusPredicate := func() bool { return true }
	minimalStatusPredicate := func() bool {
		return false
	}
	localPredicate := func() bool {
		return fullStatusPredicate()
	}

	transportDispatcher.BundleRegister(&registration.BundleRegistration{
		MsgID:            constants.ComplianceMsgKey,
		CreateBundleFunc: syncer.createComplianceBundleFunc,
		Predicate:        fullStatusPredicate,
	})

	transportDispatcher.BundleRegister(&registration.BundleRegistration{
		MsgID:            constants.CompleteComplianceMsgKey,
		CreateBundleFunc: syncer.createCompleteComplianceBundleFunc,
		Predicate:        fullStatusPredicate,
	})

	transportDispatcher.BundleRegister(&registration.BundleRegistration{
		MsgID:            constants.DeltaComplianceMsgKey,
		CreateBundleFunc: syncer.createDeltaComplianceFunc,
		Predicate:        fullStatusPredicate,
	})

	transportDispatcher.BundleRegister(&registration.BundleRegistration{
		MsgID:            constants.MinimalComplianceMsgKey,
		CreateBundleFunc: syncer.createMinimalComplianceBundleFunc,
		Predicate:        minimalStatusPredicate,
	})

	transportDispatcher.BundleRegister(&registration.BundleRegistration{
		MsgID:            constants.LocalComplianceMsgKey,
		CreateBundleFunc: syncer.createLocalComplianceBundleFunc,
		Predicate:        localPredicate,
	})

	transportDispatcher.BundleRegister(&registration.BundleRegistration{
		MsgID:            constants.LocalCompleteComplianceMsgKey,
		CreateBundleFunc: syncer.createLocalCompleteComplianceBundleFunc,
		Predicate:        localPredicate,
	})
}

// RegisterBundleHandlerFunctions registers bundle handler functions within the conflation manager.
// handler functions need to do "diff" between objects received in the bundle and the objects in database.
// leaf hub sends only the current existing objects, and status transport bridge should understand implicitly which
// objects were deleted.
// therefore, whatever is in the db and cannot be found in the bundle has to be deleted from the database.
// for the objects that appear in both, need to check if something has changed using resourceVersion field comparison
// and if the object was changed, update the db with the current object.
func (syncer *CompliancesDBSyncer) RegisterBundleHandlerFunctions(conflationManager *conflator.ConflationManager) {
	complianceBundleType := bundle.GetBundleType(syncer.createComplianceBundleFunc())
	completeComplianceBundleType := bundle.GetBundleType(syncer.createCompleteComplianceBundleFunc())
	localComplianceBundleType := bundle.GetBundleType(syncer.createLocalComplianceBundleFunc())

	// handle compliance bundle
	conflationManager.Register(conflator.NewConflationRegistration(
		conflator.CompliancePriority,
		metadata.CompleteStateMode,
		complianceBundleType,
		func(ctx context.Context, b bundle.ManagerBundle) error {
			return syncer.handleComplianceBundle(ctx, b)
		},
	))

	// handle complete compliance bundle
	conflationManager.Register(conflator.NewConflationRegistration(
		conflator.CompleteCompliancePriority,
		metadata.CompleteStateMode,
		completeComplianceBundleType,
		func(ctx context.Context, b bundle.ManagerBundle) error {
			return syncer.handleCompleteComplianceBundle(ctx, b)
		}).WithDependency(dependency.NewDependency(complianceBundleType, dependency.ExactMatch)))
	// compliance depends on clusters per policy. should be processed only when there is an exact match

	// handle delta compliance
	conflationManager.Register(conflator.NewConflationRegistration(
		conflator.DeltaCompliancePriority, metadata.DeltaStateMode,
		bundle.GetBundleType(syncer.createDeltaComplianceFunc()),
		func(ctx context.Context, b bundle.ManagerBundle) error {
			return syncer.handleDeltaComplianceBundle(ctx, b)
		}).WithDependency(dependency.NewDependency(completeComplianceBundleType, dependency.ExactMatch)))
	// delta compliance depends on complete compliance. should be processed only when there is an exact match

	// handle minimal compliance
	conflationManager.Register(conflator.NewConflationRegistration(
		conflator.MinimalCompliancePriority,
		metadata.CompleteStateMode,
		bundle.GetBundleType(syncer.createMinimalComplianceBundleFunc()),
		func(ctx context.Context, b bundle.ManagerBundle) error {
			return syncer.handleMinimalComplianceBundle(ctx, b)
		},
	))

	// handle local compliance
	conflationManager.Register(conflator.NewConflationRegistration(
		conflator.LocalCompliancePriority,
		metadata.CompleteStateMode,
		localComplianceBundleType,
		func(ctx context.Context, b bundle.ManagerBundle) error {
			return syncer.handleLocalComplianceBundle(ctx, b)
		},
	))

	conflationManager.Register(conflator.NewConflationRegistration(
		conflator.LocalCompleteCompliancePriority,
		metadata.CompleteStateMode,
		bundle.GetBundleType(syncer.createLocalCompleteComplianceBundleFunc()),
		func(ctx context.Context, b bundle.ManagerBundle) error {
			return syncer.handleLocalCompleteComplianceBundle(ctx, b)
		}).WithDependency(dependency.NewDependency(localComplianceBundleType, dependency.ExactMatch)))
}

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
