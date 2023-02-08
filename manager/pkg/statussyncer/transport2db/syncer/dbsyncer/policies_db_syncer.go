package dbsyncer

import (
	"context"
	"fmt"

	set "github.com/deckarep/golang-set"
	"github.com/go-logr/logr"
	corev1 "k8s.io/api/core/v1"

	statusbundle "github.com/stolostron/multicluster-global-hub/manager/pkg/statussyncer/transport2db/bundle"
	"github.com/stolostron/multicluster-global-hub/manager/pkg/statussyncer/transport2db/syncer/dispatcher"
	"github.com/stolostron/multicluster-global-hub/pkg/bundle"
	"github.com/stolostron/multicluster-global-hub/pkg/bundle/helpers"
	"github.com/stolostron/multicluster-global-hub/pkg/bundle/registration"
	"github.com/stolostron/multicluster-global-hub/pkg/bundle/status"
	"github.com/stolostron/multicluster-global-hub/pkg/conflator"
	"github.com/stolostron/multicluster-global-hub/pkg/conflator/dependency"
	"github.com/stolostron/multicluster-global-hub/pkg/constants"
	"github.com/stolostron/multicluster-global-hub/pkg/database"
)

const failedBatchFormat = "failed to perform batch - %w"

// NewPoliciesDBSyncer creates a new instance of PoliciesDBSyncer.
func NewPoliciesDBSyncer(log logr.Logger, config *corev1.ConfigMap) DBSyncer {
	dbSyncer := &PoliciesDBSyncer{
		log:                                           log,
		config:                                        config,
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
	config                                        *corev1.ConfigMap
	createClustersPerPolicyBundleFunc             status.CreateBundleFunction
	createCompleteComplianceStatusBundleFunc      status.CreateBundleFunction
	createDeltaComplianceStatusBundleFunc         status.CreateBundleFunction
	createMinimalComplianceStatusBundleFunc       status.CreateBundleFunction
	createLocalClustersPerPolicyBundleFunc        status.CreateBundleFunction
	createLocalCompleteComplianceStatusBundleFunc status.CreateBundleFunction
}

// RegisterCreateBundleFunctions registers create bundle functions within the transport instance.
func (syncer *PoliciesDBSyncer) RegisterCreateBundleFunctions(transportDispatcher *dispatcher.TransportDispatcher) {
	fullStatusPredicate := func() bool { return syncer.config.Data["aggregationLevel"] == "full" }
	minimalStatusPredicate := func() bool {
		return syncer.config.Data["aggregationLevel"] == "minimal"
	}
	localPredicate := func() bool {
		return fullStatusPredicate() &&
			syncer.config.Data["enableLocalPolicies"] == "true"
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
		func(ctx context.Context, bundle status.Bundle, dbClient database.StatusTransportBridgeDB) error {
			return syncer.handleClustersPerPolicyBundle(ctx, bundle, dbClient,
				database.StatusSchema, database.ComplianceTableName)
		},
	))

	conflationManager.Register(conflator.NewConflationRegistration(
		conflator.CompleteComplianceStatusPriority, bundle.CompleteStateMode, completeComplianceStatusBundleType,
		func(ctx context.Context, bundle status.Bundle, dbClient database.StatusTransportBridgeDB) error {
			return syncer.handleCompleteComplianceBundle(ctx, bundle, dbClient,
				database.StatusSchema, database.ComplianceTableName)
		}).WithDependency(dependency.NewDependency(clustersPerPolicyBundleType, dependency.ExactMatch)))
	// compliance depends on clusters per policy. should be processed only when there is an exact match

	conflationManager.Register(conflator.NewConflationRegistration(
		conflator.DeltaComplianceStatusPriority, bundle.DeltaStateMode,
		helpers.GetBundleType(syncer.createDeltaComplianceStatusBundleFunc()),
		func(ctx context.Context, bundle status.Bundle, dbClient database.StatusTransportBridgeDB) error {
			return syncer.handleDeltaComplianceBundle(ctx, bundle, dbClient,
				database.StatusSchema, database.ComplianceTableName)
		}).WithDependency(dependency.NewDependency(completeComplianceStatusBundleType, dependency.ExactMatch)))
	// delta compliance depends on complete compliance. should be processed only when there is an exact match

	conflationManager.Register(conflator.NewConflationRegistration(
		conflator.MinimalComplianceStatusPriority, bundle.CompleteStateMode,
		helpers.GetBundleType(syncer.createMinimalComplianceStatusBundleFunc()),
		func(ctx context.Context, bundle status.Bundle, dbClient database.StatusTransportBridgeDB) error {
			return syncer.handleMinimalComplianceBundle(ctx, bundle, dbClient)
		},
	))

	conflationManager.Register(conflator.NewConflationRegistration(
		conflator.LocalClustersPerPolicyPriority, bundle.CompleteStateMode, localClustersPerPolicyBundleType,
		func(ctx context.Context, bundle status.Bundle, dbClient database.StatusTransportBridgeDB) error {
			return syncer.handleClustersPerPolicyBundle(ctx, bundle, dbClient, database.LocalStatusSchema,
				database.ComplianceTableName)
		},
	))

	conflationManager.Register(conflator.NewConflationRegistration(
		conflator.LocalCompleteComplianceStatusPriority, bundle.CompleteStateMode,
		helpers.GetBundleType(syncer.createLocalCompleteComplianceStatusBundleFunc()),
		func(ctx context.Context, bundle status.Bundle, dbClient database.StatusTransportBridgeDB) error {
			return syncer.handleCompleteComplianceBundle(ctx, bundle, dbClient, database.LocalStatusSchema,
				database.ComplianceTableName)
		}).WithDependency(dependency.NewDependency(localClustersPerPolicyBundleType, dependency.ExactMatch)))
}

// if we got inside the handler function, then the bundle version is newer than what was already handled.
// handling clusters per policy bundle inserts or deletes rows from/to the compliance table.
// in case the row already exists (leafHubName, policyId, clusterName) it updates the compliance status accordingly.
// this bundle is triggered only when policy was added/removed or when placement rule has changed which caused list of
// clusters (of at least one policy) to change.
// in other cases where only compliance status change, only compliance bundle is received.
func (syncer *PoliciesDBSyncer) handleClustersPerPolicyBundle(ctx context.Context, bundle status.Bundle,
	dbClient database.PoliciesStatusDB, dbSchema string, dbTableName string,
) error {
	logBundleHandlingMessage(syncer.log, bundle, startBundleHandlingMessage)
	leafHubName := bundle.GetLeafHubName()

	complianceRowsFromDB, err := dbClient.GetComplianceStatusByLeafHub(ctx, dbSchema, dbTableName, leafHubName)
	if err != nil {
		return fmt.Errorf("failed fetching leaf hub '%s' compliance status rows from db - %w", leafHubName, err)
	}

	batchBuilder := dbClient.NewPoliciesBatchBuilder(dbSchema, dbTableName, leafHubName)

	for _, object := range bundle.GetObjects() { // every object is clusters list per policy with full state
		clustersPerPolicy, ok := object.(*status.PolicyGenericComplianceStatus)
		if !ok {
			continue // do not handle objects other than PolicyGenericComplianceStatus
		}

		clustersFromDB, policyExistsInDB := complianceRowsFromDB[clustersPerPolicy.PolicyID]
		if !policyExistsInDB {
			clustersFromDB = database.NewPolicyClusterSets()
		}

		syncer.handleClusterPerPolicy(batchBuilder, clustersPerPolicy, clustersFromDB)

		// keep this policy in db, should remove from db only policies that were not sent in the bundle
		delete(complianceRowsFromDB, clustersPerPolicy.PolicyID)
	}
	// remove policies that were not sent in the bundle
	for policyID := range complianceRowsFromDB {
		batchBuilder.DeletePolicy(policyID)
	}
	// batch may contain up to the number of compliance status rows per leaf hub, that is (num_of_policies * num_of_MCs)
	if err := dbClient.SendBatch(ctx, batchBuilder.Build()); err != nil {
		return fmt.Errorf(failedBatchFormat, err)
	}

	logBundleHandlingMessage(syncer.log, bundle, finishBundleHandlingMessage)

	return nil
}

func (syncer *PoliciesDBSyncer) handleClusterPerPolicy(batchBuilder database.PoliciesBatchBuilder,
	clustersFromBundle *status.PolicyGenericComplianceStatus, clustersFromDB *database.PolicyClustersSets,
) {
	allClustersFromDB := clustersFromDB.GetAllClusters()

	// handle compliant clusters of the policy
	allClustersFromDB = syncer.handleClustersPerPolicyWithSpecificCompliance(batchBuilder, clustersFromBundle.PolicyID,
		clustersFromBundle.CompliantClusters, allClustersFromDB,
		clustersFromDB.GetClusters(database.Compliant), database.Compliant)
	// handle non compliant clusters of the policy
	allClustersFromDB = syncer.handleClustersPerPolicyWithSpecificCompliance(batchBuilder, clustersFromBundle.PolicyID,
		clustersFromBundle.NonCompliantClusters, allClustersFromDB,
		clustersFromDB.GetClusters(database.NonCompliant),
		database.NonCompliant)
	// handle unknown clusters of the policy
	allClustersFromDB = syncer.handleClustersPerPolicyWithSpecificCompliance(batchBuilder, clustersFromBundle.PolicyID,
		clustersFromBundle.UnknownComplianceClusters, allClustersFromDB,
		clustersFromDB.GetClusters(database.Unknown),
		database.Unknown)

	// delete compliance status rows in the db that were not sent in the bundle (leaf hub sends only living resources)
	allClustersFromDB.Each(func(object interface{}) bool {
		clusterName, ok := object.(string)
		if !ok {
			return false // if object is not a cluster name string ,do nothing.
		}

		batchBuilder.DeleteClusterStatus(clustersFromBundle.PolicyID, clusterName)

		return false // return true with this set implementation will stop the iteration, therefore need to return false
	})
}

// typedClustersFromDB is a set that contains the clusters from db with specific compliance status - that is
// all compliant/nonCompliant/unknown clusters and only them.
func (syncer *PoliciesDBSyncer) handleClustersPerPolicyWithSpecificCompliance(
	batchBuilder database.PoliciesBatchBuilder, policyID string, typedClustersFromBundle []string,
	allClustersFromDB set.Set, typedClustersFromDB set.Set, complianceStatus database.ComplianceStatus,
) set.Set {
	for _, clusterName := range typedClustersFromBundle { // go over the clusters from bundle
		if !allClustersFromDB.Contains(clusterName) { // check if cluster not found in the db compliance table
			batchBuilder.Insert(policyID, clusterName, database.ErrorNone, complianceStatus)
			continue
		}
		// compliance row exists both in db and in the bundle, check if we need to update status
		if !typedClustersFromDB.Contains(clusterName) { // if cluster is in db but not with the right compliance status
			batchBuilder.UpdateClusterCompliance(policyID, clusterName, complianceStatus)
		}
		// either way if status was updated or not, remove from allClustersFromDB to mark this cluster as handled
		allClustersFromDB.Remove(clusterName)
	}

	return allClustersFromDB
}

// if we got to the handler function, then the bundle pre-conditions were satisfied (the version is newer than what
// was already handled and base bundle was already handled successfully)
// we assume that 'ClustersPerPolicy' handler function handles the addition or removal of clusters rows.
// in this handler function, we handle only the existing clusters rows.
func (syncer *PoliciesDBSyncer) handleCompleteComplianceBundle(ctx context.Context, bundle status.Bundle,
	dbClient database.PoliciesStatusDB, dbSchema string, dbTableName string,
) error {
	logBundleHandlingMessage(syncer.log, bundle, startBundleHandlingMessage)
	leafHubName := bundle.GetLeafHubName()

	nonCompliantRowsFromDB, err := dbClient.GetNonCompliantClustersByLeafHub(ctx, dbSchema, dbTableName, leafHubName)
	if err != nil {
		return fmt.Errorf("failed fetching leaf hub '%s' compliance status rows from db - %w", leafHubName, err)
	}

	batchBuilder := dbClient.NewPoliciesBatchBuilder(dbSchema, dbTableName, leafHubName)

	for _, object := range bundle.GetObjects() { // every object in bundle is policy compliance status
		policyComplianceStatus, ok := object.(*status.PolicyCompleteComplianceStatus)
		if !ok {
			continue // do not handle objects other than PolicyComplianceStatus
		}
		// nonCompliantClusters includes both non Compliant and Unknown clusters
		nonCompliantClustersFromDB, policyExistsInDB := nonCompliantRowsFromDB[policyComplianceStatus.PolicyID]
		if !policyExistsInDB {
			nonCompliantClustersFromDB = database.NewPolicyClusterSets()
		}

		syncer.handlePolicyCompleteComplianceStatus(batchBuilder, policyComplianceStatus,
			nonCompliantClustersFromDB.GetClusters(database.NonCompliant),
			nonCompliantClustersFromDB.GetClusters(database.Unknown))

		// for policies that are found in the db but not in the bundle - all clusters are Compliant (implicitly)
		delete(nonCompliantRowsFromDB, policyComplianceStatus.PolicyID)
	}
	// update policies not in the bundle - all is Compliant
	for policyID := range nonCompliantRowsFromDB {
		batchBuilder.UpdatePolicyCompliance(policyID, database.Compliant)
	}
	// batch may contain up to the number of compliance status rows per leaf hub, that is (num_of_policies * num_of_MCs)
	if err := dbClient.SendBatch(ctx, batchBuilder.Build()); err != nil {
		return fmt.Errorf(failedBatchFormat, err)
	}

	logBundleHandlingMessage(syncer.log, bundle, finishBundleHandlingMessage)

	return nil
}

func (syncer *PoliciesDBSyncer) handlePolicyCompleteComplianceStatus(batchBuilder database.PoliciesBatchBuilder,
	policyComplianceStatus *status.PolicyCompleteComplianceStatus, nonCompliantClustersFromDB set.Set,
	unknownClustersFromDB set.Set,
) {
	// put non compliant and unknown clusters in a single set.
	// the clusters that will be left in this set, will be considered implicitly as compliant
	clustersFromDB := nonCompliantClustersFromDB.Union(unknownClustersFromDB)

	// update in db batch the non Compliant clusters as it was reported by leaf hub
	for _, clusterName := range policyComplianceStatus.NonCompliantClusters { // go over bundle non compliant clusters
		if !nonCompliantClustersFromDB.Contains(clusterName) { // check if row is different than non compliant in db
			batchBuilder.UpdateClusterCompliance(policyComplianceStatus.PolicyID, clusterName, database.NonCompliant)
		} // if different need to update, otherwise no need to do anything.

		clustersFromDB.Remove(clusterName) // mark cluster as handled
	}
	// update in db batch the unknown clusters as it was reported by leaf hub
	for _, clusterName := range policyComplianceStatus.UnknownComplianceClusters { // go over bundle unknown clusters
		if !unknownClustersFromDB.Contains(clusterName) { // check if row is different than unknown in db
			batchBuilder.UpdateClusterCompliance(policyComplianceStatus.PolicyID, clusterName, database.Unknown)
		} // if different need to update, otherwise no need to do anything.

		clustersFromDB.Remove(clusterName) // mark cluster as handled
	}
	// clusters left in the union of non compliant + unknown clusters from db are implicitly considered as Compliant
	clustersFromDB.Each(func(object interface{}) bool {
		clusterName, ok := object.(string)
		if !ok {
			return false // if object is not a cluster name string ,do nothing.
		}
		// change to Compliant
		batchBuilder.UpdateClusterCompliance(policyComplianceStatus.PolicyID, clusterName, database.Compliant)

		return false // return true with this set implementation will stop the iteration, therefore need to return false
	})
}

// if we got to the handler function, then the bundle pre-conditions were satisfied.
func (syncer *PoliciesDBSyncer) handleDeltaComplianceBundle(ctx context.Context, bundle status.Bundle,
	dbClient database.PoliciesStatusDB, dbSchema string, dbTableName string,
) error {
	logBundleHandlingMessage(syncer.log, bundle, startBundleHandlingMessage)
	leafHubName := bundle.GetLeafHubName()

	batchBuilder := dbClient.NewPoliciesBatchBuilder(dbSchema, dbTableName, leafHubName)

	for _, object := range bundle.GetObjects() { // every object in bundle is policy generic compliance status
		policyGenericComplianceStatus, ok := object.(*status.PolicyGenericComplianceStatus)
		if !ok {
			continue // do not handle objects other than PolicyComplianceStatus
		}

		syncer.handleDeltaPolicyComplianceStatus(batchBuilder, policyGenericComplianceStatus)
	}
	// batch may contain up to the number of compliance status rows per leaf hub, that is (num_of_policies * num_of_MCs)
	if err := dbClient.SendBatch(ctx, batchBuilder.Build()); err != nil {
		return fmt.Errorf(failedBatchFormat, err)
	}

	logBundleHandlingMessage(syncer.log, bundle, finishBundleHandlingMessage)

	return nil
}

// handleDeltaPolicyComplianceStatus updates db with leaf hub's given clusters with the given status as-is.
func (syncer *PoliciesDBSyncer) handleDeltaPolicyComplianceStatus(batchBuilder database.PoliciesBatchBuilder,
	policyComplianceStatus *status.PolicyGenericComplianceStatus,
) {
	for _, cluster := range policyComplianceStatus.CompliantClusters {
		batchBuilder.UpdateClusterCompliance(policyComplianceStatus.PolicyID, cluster, database.Compliant)
	}

	for _, cluster := range policyComplianceStatus.NonCompliantClusters {
		batchBuilder.UpdateClusterCompliance(policyComplianceStatus.PolicyID, cluster, database.NonCompliant)
	}

	for _, cluster := range policyComplianceStatus.UnknownComplianceClusters {
		batchBuilder.UpdateClusterCompliance(policyComplianceStatus.PolicyID, cluster, database.Unknown)
	}
}

// if we got to the handler function, then the bundle pre-conditions are satisfied.
func (syncer *PoliciesDBSyncer) handleMinimalComplianceBundle(ctx context.Context, bundle status.Bundle,
	dbClient database.AggregatedPoliciesStatusDB,
) error {
	logBundleHandlingMessage(syncer.log, bundle, startBundleHandlingMessage)
	leafHubName := bundle.GetLeafHubName()

	policyIDsFromDB, err := dbClient.GetPolicyIDsByLeafHub(ctx, database.StatusSchema,
		database.MinimalComplianceTable, leafHubName)
	if err != nil {
		return fmt.Errorf("failed fetching leaf hub '%s' policies from db - %w", leafHubName, err)
	}

	for _, object := range bundle.GetObjects() { // every object in bundle is minimal policy compliance status.
		minPolicyCompliance, ok := object.(*status.MinimalPolicyComplianceStatus)
		if !ok {
			continue // do not handle objects other than MinimalPolicyComplianceStatus.
		}

		if err := dbClient.InsertOrUpdateAggregatedPolicyCompliance(ctx,
			database.StatusSchema, database.MinimalComplianceTable,
			leafHubName, minPolicyCompliance.PolicyID, minPolicyCompliance.AppliedClusters,
			minPolicyCompliance.NonCompliantClusters); err != nil {
			return fmt.Errorf("failed to update minimal compliance of policy '%s', leaf hub '%s' in db - %w",
				minPolicyCompliance.PolicyID, leafHubName, err)
		}
		// policy that is found both in db and bundle, need to remove from policiesFromDB
		// eventually we will be left with policies not in the bundle inside policyIDsFromDB and will use it to remove
		// policies that has to be deleted from the table.
		policyIDsFromDB.Remove(minPolicyCompliance.PolicyID)
	}

	// remove policies that in the db but were not sent in the bundle (leaf hub sends only living resources).
	for _, object := range policyIDsFromDB.ToSlice() {
		policyID, ok := object.(string)
		if !ok {
			continue
		}

		if err := dbClient.DeleteAllComplianceRows(ctx, database.StatusSchema,
			database.MinimalComplianceTable, leafHubName,
			policyID); err != nil {
			return fmt.Errorf("failed deleted compliance rows of policy '%s', leaf hub '%s' from db - %w",
				policyID, leafHubName, err)
		}
	}

	logBundleHandlingMessage(syncer.log, bundle, finishBundleHandlingMessage)

	return nil
}
