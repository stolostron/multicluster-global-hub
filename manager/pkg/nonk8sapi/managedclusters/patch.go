// Copyright (c) 2021 Red Hat, Inc.
// Copyright Contributors to the Open Cluster Management project

package managedclusters

import (
	"errors"
	"fmt"
	"net/http"
	"strings"

	"github.com/gin-gonic/gin"

	"github.com/stolostron/multicluster-global-hub/pkg/database"
)

var (
	errOnlyPatchOfLabelsIsImplemented   = errors.New(onlyPatchOfLabelsIsImplemented)
	errOnlyAddOrRemoveAreImplemented    = errors.New(onlyAddOrRemoveAreImplemented)
	errOptimisticConcurrencyWriteFailed = errors.New(noRowsAffectedByOptimisticConcurrencyUpdate)
)

type patch struct {
	Op    string `json:"op" binding:"required"`
	Path  string `json:"path" binding:"required"`
	Value string `json:"value"`
}

// PatchManagedCluster godoc
// @summary patch managed cluster label
// @description patch label for a given managed cluster
// @accept json
// @produce json
// @param        clusterID    path    string    true    "Managed Cluster ID"
// @param        patch        body    patch     true    "JSON patch that operators on managed cluster label"
// @success      200
// @failure      400
// @failure      401
// @failure      403
// @failure      404
// @failure      500
// @failure      503
// @security     ApiKeyAuth
// @router /managedcluster/{clusterID} [patch]
func PatchManagedCluster() gin.HandlerFunc {
	return func(ginCtx *gin.Context) {
		clusterID := ginCtx.Param("clusterID")

		fmt.Fprintf(gin.DefaultWriter, "patch for cluster with ID: %s\n", clusterID)

		db := database.GetGorm()
		var leafHubName, managedClusterName string
		if err := db.Raw(`SELECT leaf_hub_name, payload->'metadata'->>'name' FROM status.managed_clusters 
			WHERE cluster_id = ?`, clusterID).Row().Scan(&leafHubName, &managedClusterName); err != nil {
			fmt.Fprintf(gin.DefaultWriter, "failed to get leaf hub and manged cluster name: %s\n", err.Error())
			return
		}

		fmt.Fprintf(gin.DefaultWriter, "patch for managed cluster: %s -leaf hub: %s\n",
			managedClusterName, leafHubName)

		var patches []patch

		err := ginCtx.BindJSON(&patches)
		if err != nil {
			fmt.Fprintf(gin.DefaultWriter, "failed to bind: %s\n", err.Error())
			return
		}

		labelsToAdd, labelsToRemove, err := getLabels(ginCtx, patches)
		if err != nil {
			fmt.Fprintf(gin.DefaultWriter, "failed to get labels: %s\n", err.Error())
			return
		}

		fmt.Fprintf(gin.DefaultWriter, "labels to add: %v\n", labelsToAdd)
		fmt.Fprintf(gin.DefaultWriter, "labels to remove: %v\n", labelsToRemove)

		retryAttempts := optimisticConcurrencyRetryAttempts

		for retryAttempts > 0 {
			err = updateLabels(clusterID, leafHubName, managedClusterName, labelsToAdd,
				labelsToRemove)
			if err == nil {
				break
			}

			retryAttempts--
		}

		if err != nil {
			ginCtx.String(http.StatusInternalServerError, "internal error")
			fmt.Fprintf(gin.DefaultWriter, "error in updating managed cluster labels: %v\n", err)
		}

		ginCtx.String(http.StatusOK, "managed cluster label patched")
	}
}

func updateLabels(clusterID, leafHubName, managedClusterName string, labelsToAdd map[string]string,
	labelsToRemove map[string]struct{},
) error {
	if len(labelsToAdd) == 0 && len(labelsToRemove) == 0 {
		return nil
	}
	db := database.GetGorm()
	rows, err := db.Raw("SELECT labels, deleted_label_keys, version from spec.managed_clusters_labels WHERE id = ?",
		clusterID).Rows()
	if err != nil {
		return fmt.Errorf("failed to read from managed_clusters_labels: %w", err)
	}
	defer rows.Close()

	if !rows.Next() { // insert the labels
		err := db.Exec(`INSERT INTO spec.managed_clusters_labels (id, leaf_hub_name, managed_cluster_name, labels,
			deleted_label_keys, version, updated_at) values(?, ?, ?, ?, ?, 0, now())`,
			clusterID, leafHubName, managedClusterName, labelsToAdd, getKeys(labelsToRemove)).Error
		if err != nil {
			return fmt.Errorf("failed to insert into the managed_clusters_labels table: %w", err)
		}

		return nil
	}

	var (
		currentLabelsToAdd         map[string]string
		currentLabelsToRemoveSlice []string
		version                    int64
	)

	err = rows.Scan(&currentLabelsToAdd, &currentLabelsToRemoveSlice, &version)
	if err != nil {
		return fmt.Errorf("failed to scan a row: %w", err)
	}

	err = updateRow(clusterID, labelsToAdd, currentLabelsToAdd, labelsToRemove,
		getMap(currentLabelsToRemoveSlice), version)
	if err != nil {
		return fmt.Errorf("failed to update managed_clusters_labels table: %w", err)
	}

	// assumimg there is a single row
	if rows.Next() {
		fmt.Fprintf(gin.DefaultWriter, "Warning: more than one row for cluster with ID %s\n", clusterID)
	}

	return nil
}

func updateRow(clusterID string, labelsToAdd, currentLabelsToAdd map[string]string,
	labelsToRemove, currentLabelsToRemove map[string]struct{}, version int64,
) error {
	newLabelsToAdd := make(map[string]string)
	newLabelsToRemove := make(map[string]struct{})

	for key := range currentLabelsToRemove {
		if _, keyToBeAdded := labelsToAdd[key]; !keyToBeAdded {
			newLabelsToRemove[key] = struct{}{}
		}
	}

	for key := range labelsToRemove {
		newLabelsToRemove[key] = struct{}{}
	}

	for key, value := range currentLabelsToAdd {
		if _, keyToBeRemoved := labelsToRemove[key]; !keyToBeRemoved {
			newLabelsToAdd[key] = value
		}
	}

	for key, value := range labelsToAdd {
		newLabelsToAdd[key] = value
	}
	db := database.GetGorm()
	result := db.Exec(`UPDATE spec.managed_clusters_labels SET
		labels = ?,
		deleted_label_keys = ?,
		version = version + 1,
		updated_at = now()
		WHERE id= ? AND version= ?`,
		newLabelsToAdd, getKeys(newLabelsToRemove), clusterID, version)
	if result.Error != nil {
		return fmt.Errorf("failed to update a row: %w", result.Error)
	}

	if result.RowsAffected == 0 {
		return fmt.Errorf("failed to update a row: %w", errOptimisticConcurrencyWriteFailed)
	}
	return nil
}

func getMap(aSlice []string) map[string]struct{} {
	mapToReturn := make(map[string]struct{}, len(aSlice))

	for _, key := range aSlice {
		mapToReturn[key] = struct{}{}
	}

	return mapToReturn
}

// from https://stackoverflow.com/q/21362950
func getKeys(aMap map[string]struct{}) []string {
	keys := make([]string, len(aMap))
	index := 0

	for key := range aMap {
		keys[index] = key
		index++
	}

	return keys
}

func getLabels(ginCtx *gin.Context, patches []patch) (map[string]string, map[string]struct{}, error) {
	labelsToAdd := make(map[string]string)
	labelsToRemove := make(map[string]struct{})

	// from https://datatracker.ietf.org/doc/html/rfc6902:
	// Evaluation of a JSON Patch document begins against a target JSON
	// document.  Operations are applied sequentially in the order they
	// appear in the array.  Each operation in the sequence is applied to
	// the target document; the resulting document becomes the target of the
	// next operation.  Evaluation continues until all operations are
	// successfully applied or until an error condition is encountered.

	for _, aPatch := range patches {
		rawLabel := strings.TrimPrefix(aPatch.Path, "/metadata/labels/")

		if rawLabel == aPatch.Path {
			ginCtx.JSON(http.StatusNotImplemented, gin.H{
				"status": onlyPatchOfLabelsIsImplemented,
			})

			return nil, nil, errOnlyPatchOfLabelsIsImplemented
		}

		label := strings.Replace(rawLabel, "~1", "/", 1)
		if aPatch.Op == "add" {
			delete(labelsToRemove, label)

			labelsToAdd[label] = aPatch.Value

			continue
		}

		if aPatch.Op == "remove" {
			delete(labelsToAdd, label)

			labelsToRemove[label] = struct{}{}

			continue
		}

		ginCtx.JSON(http.StatusNotImplemented, gin.H{
			"status": onlyAddOrRemoveAreImplemented,
		})

		return nil, nil, errOnlyAddOrRemoveAreImplemented
	}

	return labelsToAdd, labelsToRemove, nil
}
