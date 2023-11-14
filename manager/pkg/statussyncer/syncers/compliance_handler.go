package dbsyncer

import (
	"context"
	"fmt"

	set "github.com/deckarep/golang-set"
	"gorm.io/gorm"
	"gorm.io/gorm/clause"

	"github.com/stolostron/multicluster-global-hub/pkg/bundle"
	"github.com/stolostron/multicluster-global-hub/pkg/bundle/base"
	"github.com/stolostron/multicluster-global-hub/pkg/database"
	"github.com/stolostron/multicluster-global-hub/pkg/database/models"
)

// if we got inside the handler function, then the bundle version is newer than what was already handled.
// handling clusters per policy bundle inserts or deletes rows from/to the compliance table.
// in case the row already exists (leafHubName, policyId, clusterName) it updates the compliance status accordingly.
// this bundle is triggered only when policy was added/removed or when placement rule has changed which caused list of
// clusters (of at least one policy) to change.
// in other cases where only compliance status change, only compliance bundle is received.
func (syncer *CompliancesDBSyncer) handleComplianceBundle(ctx context.Context,
	bundle bundle.ManagerBundle,
) error {
	logBundleHandlingMessage(syncer.log, bundle, startBundleHandlingMessage)
	leafHubName := bundle.GetLeafHubName()
	db := database.GetGorm()

	var compliancesFromDB []models.StatusCompliance
	err := db.Where(&models.StatusCompliance{
		LeafHubName: leafHubName,
	}).Find(&compliancesFromDB).Error
	if err != nil {
		return err
	}
	// policyID: { compliance: (cluster1, cluster2), nonCompliance: (cluster3, cluster4), unknowns: (cluster5) }
	allPolicyClusterSetsFromDB := convertStatusComplianceToClusterSets(compliancesFromDB)

	err = db.Transaction(func(tx *gorm.DB) error {
		for _, object := range bundle.GetObjects() { // every object is clusters list per policy with full state
			clustersPerPolicyFromBundle, ok := object.(*base.GenericCompliance)
			if !ok {
				continue // do not handle objects other than PolicyGenericComplianceStatus
			}

			policyClusterSetFromDB, policyExistsInDB := allPolicyClusterSetsFromDB[clustersPerPolicyFromBundle.PolicyID]
			if !policyExistsInDB {
				policyClusterSetFromDB = NewPolicyClusterSets()
			}
			allClustersOnDB := policyClusterSetFromDB.GetAllClusters()

			var err error
			// handle compliant clusters of the policy
			allClustersOnDB, err = handleClustersPerPolicyWithTx(tx, leafHubName, clustersPerPolicyFromBundle.PolicyID,
				clustersPerPolicyFromBundle.CompliantClusters, allClustersOnDB, database.Compliant,
				policyClusterSetFromDB.GetClusters(database.Compliant))
			if err != nil {
				return fmt.Errorf(failedBatchFormat, err)
			}

			// handle non compliant clusters of the policy
			allClustersOnDB, err = handleClustersPerPolicyWithTx(tx, leafHubName, clustersPerPolicyFromBundle.PolicyID,
				clustersPerPolicyFromBundle.NonCompliantClusters, allClustersOnDB, database.NonCompliant,
				policyClusterSetFromDB.GetClusters(database.NonCompliant))
			if err != nil {
				return fmt.Errorf(failedBatchFormat, err)
			}

			// handle unknown compliance clusters of the policy
			allClustersOnDB, err = handleClustersPerPolicyWithTx(tx, leafHubName, clustersPerPolicyFromBundle.PolicyID,
				clustersPerPolicyFromBundle.UnknownComplianceClusters, allClustersOnDB, database.Unknown,
				policyClusterSetFromDB.GetClusters(database.Unknown))
			if err != nil {
				return fmt.Errorf(failedBatchFormat, err)
			}

			// delete compliance status rows in the db that were not sent in the bundle (leaf hub sends only living resources)
			for _, name := range allClustersOnDB.ToSlice() {
				clusterName, ok := name.(string)
				if !ok {
					continue
				}
				err := tx.Where(&models.StatusCompliance{
					LeafHubName: leafHubName,
					PolicyID:    clustersPerPolicyFromBundle.PolicyID,
					ClusterName: clusterName,
				}).Delete(&models.StatusCompliance{}).Error
				if err != nil {
					return fmt.Errorf(failedBatchFormat, err)
				}
			}
			// keep this policy in db, should remove from db only policies that were not sent in the bundle
			delete(allPolicyClusterSetsFromDB, clustersPerPolicyFromBundle.PolicyID)
		}

		// remove policies that were not sent in the bundle
		for policyID := range allPolicyClusterSetsFromDB {
			err := tx.Where(&models.StatusCompliance{
				PolicyID: policyID,
			}).Delete(&models.StatusCompliance{}).Error
			if err != nil {
				return fmt.Errorf(failedBatchFormat, err)
			}
		}
		// return nil will commit the whole transaction
		return nil
	})
	if err != nil {
		return fmt.Errorf("failed to handle clusters per policy bundle - %w", err)
	}
	logBundleHandlingMessage(syncer.log, bundle, finishBundleHandlingMessage)
	return nil
}

// policyID: { compliance: (cluster1, cluster2), nonCompliance: (cluster3, cluster4), unknowns: (cluster5) }
func convertStatusComplianceToClusterSets(compliancesFromDB []models.StatusCompliance) map[string]*PolicyClustersSets {
	complianceClusterSetsFromDB := make(map[string]*PolicyClustersSets)
	for _, compliance := range compliancesFromDB {
		if _, ok := complianceClusterSetsFromDB[compliance.PolicyID]; !ok {
			complianceClusterSetsFromDB[compliance.PolicyID] = NewPolicyClusterSets()
		}
		complianceClusterSetsFromDB[compliance.PolicyID].AddCluster(compliance.ClusterName, compliance.Compliance)
	}
	return complianceClusterSetsFromDB
}

func handleClustersPerPolicyWithTx(tx *gorm.DB, leafHub, policyID string, bundleClusters []string,
	allClusterFromDB set.Set, complianceStatus database.ComplianceStatus, typedClusters set.Set,
) (set.Set, error) {
	for _, clusterName := range bundleClusters {
		if !allClusterFromDB.Contains(clusterName) {
			err := tx.Create(models.StatusCompliance{
				LeafHubName: leafHub,
				PolicyID:    policyID,
				ClusterName: clusterName,
				Error:       database.ErrorNone,
				Compliance:  complianceStatus,
			}).Error
			if err != nil {
				return nil, err
			}
			continue
		}
		if !typedClusters.Contains(clusterName) {
			err := tx.Model(&models.StatusCompliance{}).Where(&models.StatusCompliance{
				PolicyID:    policyID,
				ClusterName: clusterName,
				LeafHubName: leafHub,
			}).Updates(&models.StatusCompliance{
				Compliance: complianceStatus,
			}).Error
			if err != nil {
				return nil, err
			}
		}
		// either way if status was updated or not, remove from allClustersFromDB to mark this cluster as handled
		allClusterFromDB.Remove(clusterName)
	}
	return allClusterFromDB, nil
}

// if we got to the handler function, then the bundle pre-conditions were satisfied (the version is newer than what
// was already handled and base bundle was already handled successfully)
// we assume that 'ClustersPerPolicy' handler function handles the addition or removal of clusters rows.
// in this handler function, we handle only the existing clusters rows.
func (syncer *CompliancesDBSyncer) handleCompleteComplianceBundle(ctx context.Context,
	bundle bundle.ManagerBundle,
) error {
	logBundleHandlingMessage(syncer.log, bundle, startBundleHandlingMessage)
	leafHubName := bundle.GetLeafHubName()
	db := database.GetGorm()

	var nonCompliancesFromDB []models.StatusCompliance
	err := db.Where("leaf_hub_name = ? AND compliance <> ?", leafHubName, database.Compliant).
		Find(&nonCompliancesFromDB).Error
	if err != nil {
		return err
	}

	allPolicyComplianceRowsFromDB := convertStatusComplianceToClusterSets(nonCompliancesFromDB)

	err = db.Transaction(func(tx *gorm.DB) error {
		for _, object := range bundle.GetObjects() { // every object in bundle is policy compliance status
			policyComplianceStatus, ok := object.(*base.GenericCompleteCompliance)
			if !ok {
				continue // do not handle objects other than PolicyComplianceStatus
			}
			// nonCompliantClusters includes both non Compliant and Unknown clusters
			nonComplianceClusterSetsFromDB, policyExistsInDB := allPolicyComplianceRowsFromDB[policyComplianceStatus.PolicyID]
			if !policyExistsInDB {
				nonComplianceClusterSetsFromDB = NewPolicyClusterSets()
			}
			allNonComplianceClusters := nonComplianceClusterSetsFromDB.GetAllClusters()

			// update in db batch the non Compliant clusters as it was reported by leaf hub
			for _, clusterName := range policyComplianceStatus.NonCompliantClusters { // go over bundle non compliant clusters
				if !nonComplianceClusterSetsFromDB.GetClusters(
					database.NonCompliant).Contains(clusterName) {
					err := updateStatusCompliance(tx, policyComplianceStatus.PolicyID, leafHubName, clusterName,
						database.NonCompliant)
					if err != nil {
						return err
					}
				} // if different need to update, otherwise no need to do anything.
				allNonComplianceClusters.Remove(clusterName) // mark cluster as handled
			}

			// update in db batch the unknown clusters as it was reported by leaf hub
			for _, clusterName := range policyComplianceStatus.UnknownComplianceClusters { // go over bundle unknown clusters
				if !nonComplianceClusterSetsFromDB.GetClusters(database.Unknown).Contains(clusterName) {
					err := updateStatusCompliance(tx, policyComplianceStatus.PolicyID, leafHubName,
						clusterName, database.Unknown)
					if err != nil {
						return err
					}
				} // if different need to update, otherwise no need to do anything.
				allNonComplianceClusters.Remove(clusterName) // mark cluster as handled
			}

			for _, name := range allNonComplianceClusters.ToSlice() {
				clusterName, ok := name.(string)
				if !ok {
					continue
				}
				err := updateStatusCompliance(tx, policyComplianceStatus.PolicyID, leafHubName, clusterName,
					database.Compliant)
				if err != nil {
					return err
				}
			}

			// for policies that are found in the db but not in the bundle - all clusters are Compliant (implicitly)
			delete(allPolicyComplianceRowsFromDB, policyComplianceStatus.PolicyID)
		}

		// update policies not in the bundle - all is Compliant
		for policyID := range allPolicyComplianceRowsFromDB {
			ret := tx.Model(&models.StatusCompliance{}).Where(
				"policy_id = ? AND leaf_hub_name = ?", policyID, leafHubName).
				Updates(&models.StatusCompliance{Compliance: database.Compliant})
			if ret.Error != nil {
				return ret.Error
			}
		}
		// return nil will commit the whole transaction
		return nil
	})

	if err != nil {
		return fmt.Errorf("failed to handle complete compliance bundle - %w", err)
	}

	logBundleHandlingMessage(syncer.log, bundle, finishBundleHandlingMessage)
	return nil
}

func updateStatusCompliance(tx *gorm.DB, policyID string, leafHubName string, clusterName string,
	compliance database.ComplianceStatus,
) error {
	return tx.Model(&models.StatusCompliance{}).Where(&models.StatusCompliance{
		PolicyID:    policyID,
		ClusterName: clusterName,
		LeafHubName: leafHubName,
	}).Updates(&models.StatusCompliance{
		Compliance: compliance,
	}).Error
}

// if we got to the handler function, then the bundle pre-conditions were satisfied.
func (syncer *CompliancesDBSyncer) handleDeltaComplianceBundle(ctx context.Context, bundle bundle.ManagerBundle) error {
	logBundleHandlingMessage(syncer.log, bundle, startBundleHandlingMessage)
	leafHubName := bundle.GetLeafHubName()
	db := database.GetGorm()

	err := db.Transaction(func(tx *gorm.DB) error {
		for _, object := range bundle.GetObjects() { // every object in bundle is policy generic compliance status
			policyGenericComplianceStatus, ok := object.(*base.GenericCompliance)
			if !ok {
				continue // do not handle objects other than PolicyComplianceStatus
			}
			for _, cluster := range policyGenericComplianceStatus.CompliantClusters {
				err := updateStatusCompliance(tx, policyGenericComplianceStatus.PolicyID, leafHubName,
					cluster, database.Compliant)
				if err != nil {
					return err
				}
			}

			for _, cluster := range policyGenericComplianceStatus.NonCompliantClusters {
				err := updateStatusCompliance(tx, policyGenericComplianceStatus.PolicyID, leafHubName,
					cluster, database.NonCompliant)
				if err != nil {
					return err
				}
			}

			for _, cluster := range policyGenericComplianceStatus.UnknownComplianceClusters {
				err := updateStatusCompliance(tx, policyGenericComplianceStatus.PolicyID, leafHubName,
					cluster, database.Unknown)
				if err != nil {
					return err
				}
			}
		}

		// return nil will commit the whole transaction
		return nil
	})
	if err != nil {
		return fmt.Errorf("failed to handle delta compliance bundle - %w", err)
	}

	logBundleHandlingMessage(syncer.log, bundle, finishBundleHandlingMessage)

	return nil
}

// if we got to the handler function, then the bundle pre-conditions are satisfied.
func (syncer *CompliancesDBSyncer) handleMinimalComplianceBundle(ctx context.Context,
	bundle bundle.ManagerBundle,
) error {
	logBundleHandlingMessage(syncer.log, bundle, startBundleHandlingMessage)
	leafHubName := bundle.GetLeafHubName()
	schemaTable := database.StatusSchema + "." + database.MinimalComplianceTable
	db := database.GetGorm()

	policyIDSetFromDB := set.NewSet()

	// Raw SQL
	rows, err := db.Raw(fmt.Sprintf(`SELECT DISTINCT(policy_id) FROM %s WHERE leaf_hub_name = ?`, schemaTable),
		leafHubName).Rows()
	if err != nil {
		return fmt.Errorf("error reading from table %s - %w", schemaTable, err)
	}
	defer rows.Close()

	for rows.Next() {
		var policyID string
		if err := rows.Scan(&policyID); err != nil {
			return fmt.Errorf("error reading from table %s - %w", schemaTable, err)
		}
		policyIDSetFromDB.Add(policyID)
	}

	for _, object := range bundle.GetObjects() { // every object in bundle is minimal policy compliance status.
		minPolicyCompliance, ok := object.(*base.MinimalCompliance)
		if !ok {
			continue // do not handle objects other than MinimalPolicyComplianceStatus.
		}

		ret := db.Clauses(clause.OnConflict{
			Columns:   []clause.Column{{Name: "policy_id"}, {Name: "leaf_hub_name"}},
			UpdateAll: true,
		}).Create(&models.AggregatedCompliance{
			PolicyID:             minPolicyCompliance.PolicyID,
			LeafHubName:          leafHubName,
			AppliedClusters:      minPolicyCompliance.AppliedClusters,
			NonCompliantClusters: minPolicyCompliance.NonCompliantClusters,
		})
		if ret.Error != nil {
			return fmt.Errorf("failed to InsertUpdate minimal compliance of policy '%s', leaf hub '%s' in db - %w",
				minPolicyCompliance.PolicyID, leafHubName, ret.Error)
		}
		// eventually we will be left with policies not in the bundle inside policyIDsFromDB and will use it to remove
		// policies that has to be deleted from the table.
		policyIDSetFromDB.Remove(minPolicyCompliance.PolicyID)
	}

	// remove policies that in the db but were not sent in the bundle (leaf hub sends only living resources).
	for _, object := range policyIDSetFromDB.ToSlice() {
		policyID, ok := object.(string)
		if !ok {
			continue
		}

		ret := db.Where(&models.AggregatedCompliance{
			PolicyID:    policyID,
			LeafHubName: leafHubName,
		}).Delete(&models.AggregatedCompliance{})
		if ret.Error != nil {
			return fmt.Errorf("failed to delete minimal compliance of policy '%s', leaf hub '%s' from db - %w",
				policyID, leafHubName, ret.Error)
		}
	}

	logBundleHandlingMessage(syncer.log, bundle, finishBundleHandlingMessage)
	return nil
}
