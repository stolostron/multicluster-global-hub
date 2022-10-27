// Copyright (c) 2021 Red Hat, Inc.
// Copyright Contributors to the Open Cluster Management project

package managedclusters

import (
	"context"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"time"

	set "github.com/deckarep/golang-set"
	"github.com/gin-gonic/gin"
	"github.com/jackc/pgx/v4"
	"github.com/jackc/pgx/v4/pgxpool"
	corev1 "k8s.io/api/core/v1"
	apiextensionsv1 "k8s.io/apiextensions-apiserver/pkg/apis/apiextensions/v1"
	"k8s.io/apiextensions-apiserver/pkg/registry/customresource/tableconvertor"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/apis/meta/v1/unstructured"
	"k8s.io/apimachinery/pkg/runtime"
	"k8s.io/apimachinery/pkg/runtime/schema"
	clusterv1 "open-cluster-management.io/api/cluster/v1"

	"github.com/stolostron/multicluster-global-hub/manager/pkg/nonk8sapi/util"
)

const (
	serverInternalErrorMsg                      = "internal error"
	syncIntervalInSeconds                       = 4
	onlyPatchOfLabelsIsImplemented              = "only patch of labels is currently implemented"
	onlyAddOrRemoveAreImplemented               = "only add or remove operations are currently implemented"
	noRowsAffectedByOptimisticConcurrencyUpdate = "no rows were affected by an optimistic-concurrency update query"
	optimisticConcurrencyRetryAttempts          = 5
	crdName                                     = "managedclusters.cluster.open-cluster-management.io"
)

// ListManagedClusters godoc
// @summary list managed clusters
// @description list managed clusters
// @accept json
// @produce json
// @param        labelSelector    query     string  false  "list managed clusters by label selector"
// @param        limit            query     int     false  "maximum managed cluster number to receive"
// @param        continue         query     string  false  "continue token to request next request"
// @success      200  {object}    clusterv1.ManagedClusterList
// @failure      400
// @failure      401
// @failure      403
// @failure      404
// @failure      500
// @failure      503
// @security     ApiKeyAuth
// @router /managedclusters [get]
func ListManagedClusters(dbConnectionPool *pgxpool.Pool) gin.HandlerFunc {
	customResourceColumnDefinitions := util.GetCustomResourceColumnDefinitions(crdName,
		clusterv1.GroupVersion.Version)

	return func(ginCtx *gin.Context) {
		labelSelector := ginCtx.Query("labelSelector")

		selectorInSql := ""

		if labelSelector != "" {
			var err error
			selectorInSql, err = util.ParseLabelSelector(labelSelector)
			if err != nil {
				fmt.Fprintf(gin.DefaultWriter, "failed to parse label selector: %s\n", err.Error())
				return
			}
		}

		fmt.Fprintf(gin.DefaultWriter, "parsed selector: %s\n", selectorInSql)

		limit := ginCtx.Query("limit")
		fmt.Fprintf(gin.DefaultWriter, "limit: %v\n", limit)

		lastManagedClusterName, lastManagedClusterUID := "", ""

		continueToken := ginCtx.Query("continue")
		if continueToken != "" {
			fmt.Fprintf(gin.DefaultWriter, "continue: %v\n", continueToken)

			var err error
			lastManagedClusterName, lastManagedClusterUID, err = util.DecodeContinue(continueToken)
			if err != nil {
				fmt.Fprintf(gin.DefaultWriter, "failed to decode continue token: %s\n", err.Error())
				return
			}
		}

		fmt.Fprintf(gin.DefaultWriter,
			"last returned managed cluster name: %s, last returned managed cluster UID: %s\n",
			lastManagedClusterName,
			lastManagedClusterUID)

		// build query condition for paging
		LastResourceCompareCondition := fmt.Sprintf(
			"(payload -> 'metadata' ->> 'name', payload -> 'metadata' ->> 'uid') > ('%s', '%s') ",
			lastManagedClusterName,
			lastManagedClusterUID)

		// managed cluster list query order by name and uid with limit if set
		managedClusterListQuery := "SELECT payload FROM status.managed_clusters WHERE " +
			LastResourceCompareCondition +
			selectorInSql +
			" ORDER BY (payload -> 'metadata' ->> 'name', payload -> 'metadata' ->> 'uid')"

		// add limit
		if limit != "" {
			managedClusterListQuery += fmt.Sprintf(" LIMIT %s", limit)
		}

		fmt.Fprintf(gin.DefaultWriter, "managedcluster list query: %v\n", managedClusterListQuery)

		if _, watch := ginCtx.GetQuery("watch"); watch {
			handleRowsForWatch(ginCtx, managedClusterListQuery, dbConnectionPool)
			return
		}

		// last managed cluster query order by name and uid
		lastManagedClusterQuery := "SELECT payload FROM status.managed_clusters " +
			"ORDER BY (payload -> 'metadata' ->> 'name', payload -> 'metadata' ->> 'uid') DESC LIMIT 1"

		handleRows(ginCtx, managedClusterListQuery, lastManagedClusterQuery, dbConnectionPool,
			customResourceColumnDefinitions)
	}
}

func handleRowsForWatch(ginCtx *gin.Context, managedClusterListQuery string, dbConnectionPool *pgxpool.Pool) {
	writer := ginCtx.Writer
	header := writer.Header()
	header.Set("Transfer-Encoding", "chunked")
	header.Set("Content-Type", "application/json")
	writer.WriteHeader(http.StatusOK)

	ticker := time.NewTicker(syncIntervalInSeconds * time.Second)

	ctx, cancelContext := context.WithCancel(context.Background())
	defer cancelContext()

	// TODO - add deleted field to the status.managed_clusters table
	// instead of holding the previously added managed clusters by memory
	// and calculating the deleted clusters
	preAddedManagedClusterNames := set.NewSet()

	for {
		select {
		case <-writer.CloseNotify():
			ticker.Stop()
			cancelContext()

			return
		case <-ticker.C:
			if ginCtx.Err() != nil || ginCtx.IsAborted() {
				ticker.Stop()
				cancelContext()

				return
			}

			doHandleRowsForWatch(ctx, writer, managedClusterListQuery, dbConnectionPool, preAddedManagedClusterNames)
		}
	}
}

func doHandleRowsForWatch(ctx context.Context, writer io.Writer, managedClusterListQuery string,
	dbConnectionPool *pgxpool.Pool, preAddedManagedClusterNames set.Set,
) {
	rows, err := dbConnectionPool.Query(ctx, managedClusterListQuery)
	if err != nil {
		fmt.Fprintf(gin.DefaultWriter, "error in quering managed cluster list: %v\n", err)
	}

	addedManagedClusterNames := set.NewSet()

	for rows.Next() {
		managedCluster := &clusterv1.ManagedCluster{}

		err := rows.Scan(managedCluster)
		if err != nil {
			continue
		}

		addedManagedClusterNames.Add(managedCluster.GetName())
		if err := util.SendWatchEvent(&metav1.WatchEvent{
			Type:   "ADDED",
			Object: runtime.RawExtension{Object: managedCluster},
		}, writer); err != nil {
			fmt.Fprintf(gin.DefaultWriter, "error in sending watch event: %v\n", err)
		}
	}

	managedClusterNamesToDelete := preAddedManagedClusterNames.Difference(addedManagedClusterNames)

	managedClusterNamesToDeleteIterator := managedClusterNamesToDelete.Iterator()
	for managedClusterNameToDelete := range managedClusterNamesToDeleteIterator.C {
		managedClusterNameToDeleteAsString, ok := managedClusterNameToDelete.(string)
		if !ok {
			continue
		}

		preAddedManagedClusterNames.Remove(managedClusterNameToDeleteAsString)

		managedClusterToDelete := &clusterv1.ManagedCluster{}
		managedClusterToDelete.SetGroupVersionKind(schema.GroupVersionKind{
			Group:   clusterv1.GroupVersion.Group,
			Version: clusterv1.GroupVersion.Version,
			Kind:    "ManagedCluster",
		})
		managedClusterToDelete.SetName(managedClusterNameToDeleteAsString)
		if err := util.SendWatchEvent(&metav1.WatchEvent{
			Type:   "DELETED",
			Object: runtime.RawExtension{Object: managedClusterToDelete},
		}, writer); err != nil {
			fmt.Fprintf(gin.DefaultWriter, "error in sending watch event: %v\n", err)
		}
	}

	managedClusterNamesToAdd := addedManagedClusterNames.Difference(preAddedManagedClusterNames)

	managedClusterNamesToAddIterator := managedClusterNamesToAdd.Iterator()
	for managedClusterNameToAdd := range managedClusterNamesToAddIterator.C {
		managedClusterNameToAddAsString, ok := managedClusterNameToAdd.(string)
		if !ok {
			continue
		}

		preAddedManagedClusterNames.Add(managedClusterNameToAddAsString)
	}

	writer.(http.Flusher).Flush()
}

func handleRows(ginCtx *gin.Context, managedClusterListQuery, lastManagedClusterQuery string,
	dbConnectionPool *pgxpool.Pool, customResourceColumnDefinitions []apiextensionsv1.CustomResourceColumnDefinition,
) {
	lastManagedCluster := &clusterv1.ManagedCluster{}
	err := dbConnectionPool.QueryRow(context.TODO(), lastManagedClusterQuery).Scan(lastManagedCluster)
	if err != nil && err != pgx.ErrNoRows {
		ginCtx.String(http.StatusInternalServerError, serverInternalErrorMsg)
		fmt.Fprintf(gin.DefaultWriter, "error in quering last managed cluster: %v\n", err)
		return
	}

	rows, err := dbConnectionPool.Query(context.TODO(), managedClusterListQuery)
	if err != nil {
		ginCtx.String(http.StatusInternalServerError, serverInternalErrorMsg)
		fmt.Fprintf(gin.DefaultWriter, "error in quering managed clusters: %v\n", err)
	}

	managedClusterList := &clusterv1.ManagedClusterList{
		TypeMeta: metav1.TypeMeta{
			Kind:       "ManagedClusterList",
			APIVersion: "cluster.open-cluster-management.io/v1",
		},
		Items: []clusterv1.ManagedCluster{},
	}
	lastManagedClusterName, lastManagedClusterUID := "", ""
	for rows.Next() {
		managedCluster := clusterv1.ManagedCluster{}
		err := rows.Scan(&managedCluster)
		if err != nil {
			fmt.Fprintf(gin.DefaultWriter, "error in scanning a managed cluster: %v\n", err)
			continue
		}

		managedClusterList.Items = append(managedClusterList.Items, managedCluster)
		lastManagedClusterName = managedCluster.GetName()
		lastManagedClusterUID = string(managedCluster.GetUID())
	}

	if lastManagedClusterName != "" &&
		lastManagedCluster.GetName() != "" &&
		lastManagedClusterName != lastManagedCluster.GetName() &&
		lastManagedClusterUID != "" &&
		string(lastManagedCluster.GetUID()) != "" &&
		lastManagedClusterUID != string(lastManagedCluster.GetUID()) {
		continueToken, err := util.EncodeContinue(lastManagedClusterName, lastManagedClusterUID)
		if err != nil {
			ginCtx.String(http.StatusInternalServerError, serverInternalErrorMsg)
			fmt.Fprintf(gin.DefaultWriter, "error in encoding the continue token: %v\n", err)
			return
		}

		managedClusterList.SetContinue(continueToken)
	}

	if util.ShouldReturnAsTable(ginCtx) {
		fmt.Fprintf(gin.DefaultWriter, "Returning as table...\n")

		tableConvertor, err := tableconvertor.New(customResourceColumnDefinitions)
		if err != nil {
			fmt.Fprintf(gin.DefaultWriter, "error in creating table convertor: %v\n", err)
			return
		}

		managedClustersList, err := wrapObjectsInList(managedClusterList.Items)
		if err != nil {
			fmt.Fprintf(gin.DefaultWriter, "error in wrapping managed clusters in a list: %v\n", err)
			return
		}

		table, err := tableConvertor.ConvertToTable(context.TODO(), managedClustersList, nil)
		if err != nil {
			fmt.Fprintf(gin.DefaultWriter, "error in converting to table: %v\n", err)
			return
		}

		table.Kind = "Table"
		table.APIVersion = metav1.SchemeGroupVersion.String()
		ginCtx.JSON(http.StatusOK, table)

		return
	}

	ginCtx.JSON(http.StatusOK, managedClusterList)
}

func wrapObjectsInList(managedClusters []clusterv1.ManagedCluster) (*corev1.List, error) {
	list := &corev1.List{
		TypeMeta: metav1.TypeMeta{
			Kind:       "List",
			APIVersion: "v1",
		},
		ListMeta: metav1.ListMeta{},
	}

	for _, managedCluster := range managedClusters {
		// adopted from
		// https://github.com/kubernetes/kubectl/blob/4da03973dd2fcd4645f20ac669d8a73cb017ff39/pkg/cmd/get/get.go#L786
		managedClusterData, err := json.Marshal(managedCluster)
		if err != nil {
			return nil, fmt.Errorf("failed to marshall object: %w", err)
		}

		convertedObj, err := runtime.Decode(unstructured.UnstructuredJSONScheme, managedClusterData)
		if err != nil {
			return nil, fmt.Errorf("failed to decode with unstructured JSON scheme : %w", err)
		}

		list.Items = append(list.Items, runtime.RawExtension{Object: convertedObj})
	}

	return list, nil
}
