package dbsyncer

import (
	"context"

	set "github.com/deckarep/golang-set"
	"github.com/go-logr/logr"

	statusbundle "github.com/stolostron/multicluster-global-hub/manager/pkg/statussyncer/bundle"
	"github.com/stolostron/multicluster-global-hub/pkg/bundle"
	"github.com/stolostron/multicluster-global-hub/pkg/bundle/helpers"
	"github.com/stolostron/multicluster-global-hub/pkg/bundle/registration"
	"github.com/stolostron/multicluster-global-hub/pkg/bundle/status"
	"github.com/stolostron/multicluster-global-hub/pkg/conflator"
	"github.com/stolostron/multicluster-global-hub/pkg/conflator/db/postgres"
	"github.com/stolostron/multicluster-global-hub/pkg/conflator/dependency"
	"github.com/stolostron/multicluster-global-hub/pkg/constants"
	"github.com/stolostron/multicluster-global-hub/pkg/database"
)

const failedBatchFormat = "failed to perform batch - %w"

// NewComplianceDBSyncer creates a new instance of PoliciesDBSyncer.
func NewComplianceDBSyncer(log logr.Logger) DBSyncer {
	dbSyncer := &PoliciesDBSyncer{
		log:                                           log,
		createClustersPerPolicyBundleFunc:             statusbundle.NewClustersPerPolicyBundle,
		createCompleteComplianceStatusBundleFunc:      statusbundle.NewCompleteComplianceStatusBundle,
		createDeltaComplianceStatusBundleFunc:         statusbundle.NewDeltaComplianceStatusBundle,
		createMinimalComplianceStatusBundleFunc:       statusbundle.NewMinimalComplianceStatusBundle,
		createLocalClustersPerPolicyBundleFunc:        statusbundle.NewLocalClustersPerPolicyBundle,
		createLocalCompleteComplianceStatusBundleFunc: statusbundle.NewLocalCompleteComplianceStatusBundle,
	}

	log.Info("initialized policies db syncer")

	return dbSyncer
}

// PoliciesDBSyncer implements policies db sync business logic.
type PoliciesDBSyncer struct {
	log                                           logr.Logger
	createClustersPerPolicyBundleFunc             status.CreateBundleFunction
	createCompleteComplianceStatusBundleFunc      status.CreateBundleFunction
	createDeltaComplianceStatusBundleFunc         status.CreateBundleFunction
	createMinimalComplianceStatusBundleFunc       status.CreateBundleFunction
	createLocalClustersPerPolicyBundleFunc        status.CreateBundleFunction
	createLocalCompleteComplianceStatusBundleFunc status.CreateBundleFunction
}

// RegisterCreateBundleFunctions registers create bundle functions within the transport instance.
func (syncer *PoliciesDBSyncer) RegisterCreateBundleFunctions(transportDispatcher BundleRegisterable) {
	fullStatusPredicate := func() bool { return true }
	minimalStatusPredicate := func() bool {
		return false
	}
	localPredicate := func() bool {
		return fullStatusPredicate()
	}

	transportDispatcher.BundleRegister(&registration.BundleRegistration{
		MsgID:            constants.ClustersPerPolicyMsgKey,
		CreateBundleFunc: syncer.createClustersPerPolicyBundleFunc,
		Predicate:        fullStatusPredicate,
	})

	transportDispatcher.BundleRegister(&registration.BundleRegistration{
		MsgID:            constants.PolicyCompleteComplianceMsgKey,
		CreateBundleFunc: syncer.createCompleteComplianceStatusBundleFunc,
		Predicate:        fullStatusPredicate,
	})

	transportDispatcher.BundleRegister(&registration.BundleRegistration{
		MsgID:            constants.PolicyDeltaComplianceMsgKey,
		CreateBundleFunc: syncer.createDeltaComplianceStatusBundleFunc,
		Predicate:        fullStatusPredicate,
	})

	transportDispatcher.BundleRegister(&registration.BundleRegistration{
		MsgID:            constants.MinimalPolicyComplianceMsgKey,
		CreateBundleFunc: syncer.createMinimalComplianceStatusBundleFunc,
		Predicate:        minimalStatusPredicate,
	})

	transportDispatcher.BundleRegister(&registration.BundleRegistration{
		MsgID:            constants.LocalClustersPerPolicyMsgKey,
		CreateBundleFunc: syncer.createLocalClustersPerPolicyBundleFunc,
		Predicate:        localPredicate,
	})

	transportDispatcher.BundleRegister(&registration.BundleRegistration{
		MsgID:            constants.LocalPolicyCompleteComplianceMsgKey,
		CreateBundleFunc: syncer.createLocalCompleteComplianceStatusBundleFunc,
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
func (syncer *PoliciesDBSyncer) RegisterBundleHandlerFunctions(conflationManager *conflator.ConflationManager) {
	clustersPerPolicyBundleType := helpers.GetBundleType(
		syncer.createClustersPerPolicyBundleFunc())
	completeComplianceStatusBundleType := helpers.GetBundleType(
		syncer.createCompleteComplianceStatusBundleFunc())
	localClustersPerPolicyBundleType := helpers.GetBundleType(
		syncer.createLocalClustersPerPolicyBundleFunc())

	conflationManager.Register(conflator.NewConflationRegistration(
		conflator.ClustersPerPolicyPriority, bundle.CompleteStateMode, clustersPerPolicyBundleType,
		func(ctx context.Context, bundle status.Bundle, dbClient postgres.StatusTransportBridgeDB) error {
			return syncer.handleClustersPerPolicyBundle(ctx, bundle)
		},
	))

	conflationManager.Register(conflator.NewConflationRegistration(
		conflator.CompleteComplianceStatusPriority, bundle.CompleteStateMode, completeComplianceStatusBundleType,
		func(ctx context.Context, bundle status.Bundle, dbClient postgres.StatusTransportBridgeDB) error {
			return syncer.handleCompleteStatusComplianceBundle(ctx, bundle)
		}).WithDependency(dependency.NewDependency(clustersPerPolicyBundleType, dependency.ExactMatch)))
	// compliance depends on clusters per policy. should be processed only when there is an exact match

	conflationManager.Register(conflator.NewConflationRegistration(
		conflator.DeltaComplianceStatusPriority, bundle.DeltaStateMode,
		helpers.GetBundleType(syncer.createDeltaComplianceStatusBundleFunc()),
		func(ctx context.Context, bundle status.Bundle, dbClient postgres.StatusTransportBridgeDB) error {
			return syncer.handleDeltaComplianceBundle(ctx, bundle)
		}).WithDependency(dependency.NewDependency(completeComplianceStatusBundleType, dependency.ExactMatch)))
	// delta compliance depends on complete compliance. should be processed only when there is an exact match

	conflationManager.Register(conflator.NewConflationRegistration(
		conflator.MinimalComplianceStatusPriority, bundle.CompleteStateMode,
		helpers.GetBundleType(syncer.createMinimalComplianceStatusBundleFunc()),
		func(ctx context.Context, bundle status.Bundle, dbClient postgres.StatusTransportBridgeDB) error {
			return syncer.handleMinimalComplianceBundle(ctx, bundle)
		},
	))

	conflationManager.Register(conflator.NewConflationRegistration(
		conflator.LocalClustersPerPolicyPriority, bundle.CompleteStateMode, localClustersPerPolicyBundleType,
		func(ctx context.Context, bundle status.Bundle, dbClient postgres.StatusTransportBridgeDB) error {
			return syncer.handleLocalClustersPerPolicyBundle(ctx, bundle)
		},
	))

	conflationManager.Register(conflator.NewConflationRegistration(
		conflator.LocalCompleteComplianceStatusPriority, bundle.CompleteStateMode,
		helpers.GetBundleType(syncer.createLocalCompleteComplianceStatusBundleFunc()),
		func(ctx context.Context, bundle status.Bundle, dbClient postgres.StatusTransportBridgeDB) error {
			return syncer.handleCompleteLocalStatusComplianceBundle(ctx, bundle)
		}).WithDependency(dependency.NewDependency(localClustersPerPolicyBundleType, dependency.ExactMatch)))
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
