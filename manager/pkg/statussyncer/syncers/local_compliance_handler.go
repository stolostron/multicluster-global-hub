package dbsyncer

import (
	"context"
	"fmt"

	set "github.com/deckarep/golang-set"
	"gorm.io/gorm"
	"gorm.io/gorm/clause"

	"github.com/stolostron/multicluster-global-hub/pkg/bundle/status"
	"github.com/stolostron/multicluster-global-hub/pkg/database"
	"github.com/stolostron/multicluster-global-hub/pkg/database/models"
)

func (syncer *PoliciesDBSyncer) handleLocalClustersPerPolicyBundle(ctx context.Context, bundle status.Bundle) error {
	logBundleHandlingMessage(syncer.log, bundle, startBundleHandlingMessage)
	leafHubName := bundle.GetLeafHubName()
	db := database.GetGorm()

	// policyID: { compliance: (cluster1, cluster2), nonCompliance: (cluster3, cluster4), unknowns: (cluster5) }
	allPolicyClusterSetsFromDB, err := getAllLocalPolicyClusterSets(db, "leaf_hub_name = ?", leafHubName)
	if err != nil {
		return err
	}

	for _, object := range bundle.GetObjects() { // every object is clusters list per policy with full state
		clustersPerPolicyFromBundle, ok := object.(*status.PolicyGenericComplianceStatus)
		if !ok {
			continue // do not handle objects other than PolicyGenericComplianceStatus
		}

		policyClusterSetFromDB, policyExistsInDB := allPolicyClusterSetsFromDB[clustersPerPolicyFromBundle.PolicyID]
		if !policyExistsInDB {
			policyClusterSetFromDB = NewPolicyClusterSets()
		}
		allClustersOnDB := policyClusterSetFromDB.GetAllClusters()
		batchUpsertLocalCompliances := []models.LocalStatusCompliance{}

		// handle compliant clusters of the policy
		allClustersOnDB, batchUpsertLocalCompliances = addClustersForLocalPolicies(leafHubName,
			clustersPerPolicyFromBundle.PolicyID, clustersPerPolicyFromBundle.CompliantClusters,
			allClustersOnDB, database.Compliant, policyClusterSetFromDB.GetClusters(database.Compliant),
			batchUpsertLocalCompliances)

		// handle non compliant clusters of the policy
		allClustersOnDB, batchUpsertLocalCompliances = addClustersForLocalPolicies(leafHubName,
			clustersPerPolicyFromBundle.PolicyID, clustersPerPolicyFromBundle.NonCompliantClusters,
			allClustersOnDB, database.NonCompliant,
			policyClusterSetFromDB.GetClusters(database.NonCompliant),
			batchUpsertLocalCompliances)

		// handle unknown compliance clusters of the policy
		allClustersOnDB, batchUpsertLocalCompliances = addClustersForLocalPolicies(leafHubName,
			clustersPerPolicyFromBundle.PolicyID, clustersPerPolicyFromBundle.UnknownComplianceClusters,
			allClustersOnDB, database.Unknown, policyClusterSetFromDB.GetClusters(database.Unknown),
			batchUpsertLocalCompliances)

		// batch upsert
		err = db.Clauses(clause.OnConflict{
			UpdateAll: true,
		}).CreateInBatches(batchUpsertLocalCompliances, 100).Error
		if err != nil {
			return err
		}

		// delete compliance status rows in the db that were not sent in the bundle (leaf hub sends only living resources)
		err = db.Transaction(func(tx *gorm.DB) error {
			for _, name := range allClustersOnDB.ToSlice() {
				clusterName, ok := name.(string)
				if !ok {
					continue
				}
				err := tx.Where(&models.LocalStatusCompliance{
					LeafHubName: leafHubName,
					PolicyID:    clustersPerPolicyFromBundle.PolicyID,
					ClusterName: clusterName,
				}).Delete(&models.LocalStatusCompliance{}).Error
				if err != nil {
					return fmt.Errorf(failedBatchFormat, err)
				}
			}
			return nil
		})
		if err != nil {
			return fmt.Errorf("failed to handle clusters per policy bundle - %w", err)
		}
		// keep this policy in db, should remove from db only policies that were not sent in the bundle
		delete(allPolicyClusterSetsFromDB, clustersPerPolicyFromBundle.PolicyID)
	}

	// delete the policy isn't contained on the bundle
	err = db.Transaction(func(tx *gorm.DB) error {
		for policyID := range allPolicyClusterSetsFromDB {
			err := tx.Where(&models.LocalStatusCompliance{
				PolicyID: policyID,
			}).Delete(&models.LocalStatusCompliance{}).Error
			if err != nil {
				return fmt.Errorf(failedBatchFormat, err)
			}
		}
		return nil
	})
	if err != nil {
		return fmt.Errorf("failed to handle clusters per policy bundle - %w", err)
	}
	logBundleHandlingMessage(syncer.log, bundle, finishBundleHandlingMessage)
	return nil
}

func addClustersForLocalPolicies(leafHub, policyID string, bundleClusters []string,
	allClusterFromDB set.Set, complianceStatus database.ComplianceStatus, typedClusters set.Set,
	allCompliances []models.LocalStatusCompliance,
) (set.Set, []models.LocalStatusCompliance) {
	for _, clusterName := range bundleClusters {
		if !allClusterFromDB.Contains(clusterName) {
			allCompliances = append(allCompliances, models.LocalStatusCompliance{
				LeafHubName: leafHub,
				PolicyID:    policyID,
				ClusterName: clusterName,
				Error:       database.ErrorNone,
				Compliance:  complianceStatus,
			})
			continue
		}
		if !typedClusters.Contains(clusterName) {
			allCompliances = append(allCompliances, models.LocalStatusCompliance{
				LeafHubName: leafHub,
				PolicyID:    policyID,
				ClusterName: clusterName,
				Error:       database.ErrorNone,
				Compliance:  complianceStatus,
			})
		}
		// either way if status was updated or not, remove from allClustersFromDB to mark this cluster as handled
		allClusterFromDB.Remove(clusterName)
	}
	return allClusterFromDB, allCompliances
}

func (syncer *PoliciesDBSyncer) handleCompleteLocalStatusComplianceBundle(ctx context.Context,
	bundle status.Bundle,
) error {
	logBundleHandlingMessage(syncer.log, bundle, startBundleHandlingMessage)
	leafHubName := bundle.GetLeafHubName()
	db := database.GetGorm()

	// policyID: { compliance: (cluster1, cluster2), nonCompliance: (cluster3, cluster4), unknowns: (cluster5) }
	allPolicyComplianceRowsFromDB, err := getAllLocalPolicyClusterSets(db,
		"leaf_hub_name = ? AND compliance <> ?",
		leafHubName, database.Compliant)
	if err != nil {
		return err
	}

	for _, object := range bundle.GetObjects() { // every object in bundle is policy compliance status
		policyComplianceStatus, ok := object.(*status.PolicyCompleteComplianceStatus)
		if !ok {
			continue // do not handle objects other than PolicyComplianceStatus
		}
		// nonCompliantClusters includes both non Compliant and Unknown clusters
		nonComplianceClusterSetsFromDB, policyExistsInDB := allPolicyComplianceRowsFromDB[policyComplianceStatus.PolicyID]
		if !policyExistsInDB {
			nonComplianceClusterSetsFromDB = NewPolicyClusterSets()
		}
		allNonComplianceClusters := nonComplianceClusterSetsFromDB.GetAllClusters()
		batchUpsertCompliances := []models.LocalStatusCompliance{}

		// update in db batch the non Compliant clusters as it was reported by leaf hub
		for _, clusterName := range policyComplianceStatus.NonCompliantClusters { // go over bundle non compliant clusters
			if !nonComplianceClusterSetsFromDB.GetClusters(database.NonCompliant).Contains(clusterName) {
				batchUpsertCompliances = append(batchUpsertCompliances, models.LocalStatusCompliance{
					PolicyID:    policyComplianceStatus.PolicyID,
					LeafHubName: leafHubName,
					ClusterName: clusterName,
					Compliance:  database.NonCompliant,
					Error:       database.ErrorNone,
				})
			} // if different need to update, otherwise no need to do anything.
			allNonComplianceClusters.Remove(clusterName) // mark cluster as handled
		}

		// update in db batch the unknown clusters as it was reported by leaf hub
		for _, clusterName := range policyComplianceStatus.UnknownComplianceClusters { // go over bundle unknown clusters
			if !nonComplianceClusterSetsFromDB.GetClusters(database.Unknown).Contains(clusterName) {
				batchUpsertCompliances = append(batchUpsertCompliances, models.LocalStatusCompliance{
					PolicyID:    policyComplianceStatus.PolicyID,
					LeafHubName: leafHubName,
					ClusterName: clusterName,
					Compliance:  database.Unknown,
					Error:       database.ErrorNone,
				})
			} // if different need to update, otherwise no need to do anything.
			allNonComplianceClusters.Remove(clusterName) // mark cluster as handled
		}

		for _, name := range allNonComplianceClusters.ToSlice() {
			clusterName, ok := name.(string)
			if !ok {
				continue
			}
			batchUpsertCompliances = append(batchUpsertCompliances, models.LocalStatusCompliance{
				PolicyID:    policyComplianceStatus.PolicyID,
				LeafHubName: leafHubName,
				ClusterName: clusterName,
				Compliance:  database.Compliant,
				Error:       database.ErrorNone,
			})
		}

		err = db.Clauses(clause.OnConflict{
			UpdateAll: true,
		}).CreateInBatches(batchUpsertCompliances, 100).Error
		if err != nil {
			return err
		}

		// for policies that are found in the db but not in the bundle - all clusters are Compliant (implicitly)
		delete(allPolicyComplianceRowsFromDB, policyComplianceStatus.PolicyID)
	}

	// update policies not in the bundle - all is Compliant
	err = db.Transaction(func(tx *gorm.DB) error {
		for policyID := range allPolicyComplianceRowsFromDB {
			err := db.Model(&models.LocalStatusCompliance{}).Where("policy_id = ? AND leaf_hub_name = ?",
				policyID, leafHubName).Updates(&models.LocalStatusCompliance{Compliance: database.Compliant}).Error
			if err != nil {
				return err
			}
		}
		return nil
	})
	if err != nil {
		return fmt.Errorf("failed deleting compliances from local complainces - %w", err)
	}

	logBundleHandlingMessage(syncer.log, bundle, finishBundleHandlingMessage)
	return nil
}

func getAllLocalPolicyClusterSets(db *gorm.DB, query interface{}, args ...interface{}) (
	map[string]*PolicyClustersSets, error,
) {
	var compliancesFromDB []models.LocalStatusCompliance
	err := db.Where(query, args...).
		Find(&compliancesFromDB).Error
	if err != nil {
		return nil, err
	}

	// policyID: { compliance: (cluster1, cluster2), nonCompliance: (cluster3, cluster4), unknowns: (cluster5) }
	allPolicyComplianceRowsFromDB := make(map[string]*PolicyClustersSets)
	for _, compliance := range compliancesFromDB {
		if _, ok := allPolicyComplianceRowsFromDB[compliance.PolicyID]; !ok {
			allPolicyComplianceRowsFromDB[compliance.PolicyID] = NewPolicyClusterSets()
		}
		allPolicyComplianceRowsFromDB[compliance.PolicyID].AddCluster(
			compliance.ClusterName, compliance.Compliance)
	}
	return allPolicyComplianceRowsFromDB, nil
}
