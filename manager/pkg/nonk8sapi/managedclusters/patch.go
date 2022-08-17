// Copyright (c) 2021 Red Hat, Inc.
// Copyright Contributors to the Open Cluster Management project

package managedclusters

import (
	"context"
	"errors"
	"fmt"
	"net/http"
	"strings"

	"github.com/gin-gonic/gin"
	"github.com/jackc/pgx/v4/pgxpool"

	"github.com/stolostron/multicluster-global-hub/manager/pkg/nonk8sapi/authentication"
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

// Patch middleware.
func Patch(authorizationURL string, authorizationCABundle []byte,
	dbConnectionPool *pgxpool.Pool,
) gin.HandlerFunc {
	return func(ginCtx *gin.Context) {
		user, isCorrectType := ginCtx.MustGet(authentication.UserKey).(string)
		if !isCorrectType {
			fmt.Fprintf(gin.DefaultWriter, "unable to get user from context")

			user = "Unknown"
		}

		groups, isCorrectType := ginCtx.MustGet(authentication.GroupsKey).([]string)
		if !isCorrectType {
			fmt.Fprintf(gin.DefaultWriter, "unable to get groups from context")

			groups = []string{}
		}

		cluster := ginCtx.Param("cluster")

		fmt.Fprintf(gin.DefaultWriter, "patch for cluster: %s\n", cluster)

		hubCluster := ginCtx.Query("hubCluster")

		fmt.Fprintf(gin.DefaultWriter, "patch for hub cluster: %s\n", hubCluster)

		if !isAuthorized(user, groups, authorizationURL, authorizationCABundle,
			dbConnectionPool, cluster, hubCluster) {
			ginCtx.JSON(http.StatusForbidden, gin.H{
				"status": "the current user cannot patch the cluster",
			})
		}

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
			err = updateLabels(cluster, hubCluster, labelsToAdd, labelsToRemove, dbConnectionPool)
			if err == nil {
				break
			}

			retryAttempts--
		}

		if err != nil {
			ginCtx.String(http.StatusInternalServerError, "internal error")
			fmt.Fprintf(gin.DefaultWriter, "error in updating managed cluster labels: %v\n", err)
		}
	}
}

func updateLabels(cluster, hubCluster string, labelsToAdd map[string]string, labelsToRemove map[string]struct{},
	dbConnectionPool *pgxpool.Pool,
) error {
	if len(labelsToAdd) == 0 && len(labelsToRemove) == 0 {
		return nil
	}

	rows, err := dbConnectionPool.Query(context.TODO(),
		"SELECT labels, deleted_label_keys, version from spec.managed_clusters_labels WHERE managed_cluster_name = $1 AND leaf_hub_name = $2",
		cluster, hubCluster)
	if err != nil {
		return fmt.Errorf("failed to read from managed_clusters_labels: %w", err)
	}
	defer rows.Close()

	if !rows.Next() { // insert the labels
		_, err := dbConnectionPool.Exec(context.TODO(),
			`INSERT INTO spec.managed_clusters_labels (leaf_hub_name, managed_cluster_name, labels,
			deleted_label_keys, version, updated_at) values($1, $2, $3::jsonb, $4::jsonb, 0, now())`,
			hubCluster, cluster, labelsToAdd, getKeys(labelsToRemove))
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

	err = updateRow(cluster, hubCluster, labelsToAdd, currentLabelsToAdd, labelsToRemove, getMap(currentLabelsToRemoveSlice),
		version, dbConnectionPool)
	if err != nil {
		return fmt.Errorf("failed to update managed_clusters_labels table: %w", err)
	}

	// assumimg there is a single row
	if rows.Next() {
		fmt.Fprintf(gin.DefaultWriter, "Warning: more than one row for cluster %s\n", cluster)
	}

	return nil
}

func updateRow(cluster, hubCluster string, labelsToAdd map[string]string, currentLabelsToAdd map[string]string,
	labelsToRemove map[string]struct{}, currentLabelsToRemove map[string]struct{},
	version int64, dbConnectionPool *pgxpool.Pool,
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

	commandTag, err := dbConnectionPool.Exec(context.TODO(),
		`UPDATE spec.managed_clusters_labels SET
		labels = $1::jsonb,
		deleted_label_keys = $2::jsonb,
		version = version + 1,
		updated_at = now()
		WHERE managed_cluster_name=$3 AND leaf_hub_name=$4 AND version=$5`,
		newLabelsToAdd, getKeys(newLabelsToRemove), cluster, hubCluster, version)
	if err != nil {
		return fmt.Errorf("failed to update a row: %w", err)
	}

	if commandTag.RowsAffected() == 0 {
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

func isAuthorized(user string, groups []string, authorizationURL string, authorizationCABundle []byte,
	dbConnectionPool *pgxpool.Pool, cluster, hubCluster string,
) bool {
	query := fmt.Sprintf(
		"SELECT COUNT(payload) from status.managed_clusters WHERE payload -> 'metadata' ->> 'name' = '%s' AND leaf_hub_name = '%s' AND %s",
		cluster, hubCluster, filterByAuthorization(user, groups, authorizationURL,
			authorizationCABundle, gin.DefaultWriter))

	var count int64

	err := dbConnectionPool.QueryRow(context.TODO(), query).Scan(&count)
	if err != nil {
		fmt.Fprintf(gin.DefaultWriter, "error in quering managed clusters: %v\n", err)
		return false
	}

	return count > 0
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
