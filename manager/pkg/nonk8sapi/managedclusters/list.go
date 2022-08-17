// Copyright (c) 2021 Red Hat, Inc.
// Copyright Contributors to the Open Cluster Management project

package managedclusters

import (
	"context"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"strings"
	"time"

	set "github.com/deckarep/golang-set"
	"github.com/gin-gonic/gin"
	"github.com/jackc/pgx/v4/pgxpool"
	corev1 "k8s.io/api/core/v1"
	apiextensionsv1 "k8s.io/apiextensions-apiserver/pkg/apis/apiextensions/v1"
	"k8s.io/apiextensions-apiserver/pkg/registry/customresource/tableconvertor"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/apis/meta/v1/unstructured"
	"k8s.io/apimachinery/pkg/runtime"
	"k8s.io/apimachinery/pkg/runtime/schema"
	clusterv1 "open-cluster-management.io/api/cluster/v1"

	"github.com/stolostron/multicluster-global-hub/manager/pkg/nonk8sapi/authentication"
	"github.com/stolostron/multicluster-global-hub/manager/pkg/nonk8sapi/util"
)

const (
	syncIntervalInSeconds                       = 4
	onlyPatchOfLabelsIsImplemented              = "only patch of labels is currently implemented"
	onlyAddOrRemoveAreImplemented               = "only add or remove operations are currently implemented"
	noRowsAffectedByOptimisticConcurrencyUpdate = "no rows were affected by an optimistic-concurrency update query"
	optimisticConcurrencyRetryAttempts          = 5
	crdName                                     = "managedclusters.cluster.open-cluster-management.io"
)

// List middleware.
func List(authorizationURL string, authorizationCABundle []byte,
	dbConnectionPool *pgxpool.Pool,
) gin.HandlerFunc {
	customResourceColumnDefinitions := util.GetCustomResourceColumnDefinitions(crdName,
		clusterv1.GroupVersion.Version)

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

		fmt.Fprintf(gin.DefaultWriter, "got authenticated user: %v\n", user)
		fmt.Fprintf(gin.DefaultWriter, "user groups: %v\n", groups)

		query := sqlQuery(user, groups, authorizationURL, authorizationCABundle)
		fmt.Fprintf(gin.DefaultWriter, "query: %v\n", query)

		if _, watch := ginCtx.GetQuery("watch"); watch {
			handleRowsForWatch(ginCtx, query, dbConnectionPool)
			return
		}

		handleRows(ginCtx, query, dbConnectionPool, customResourceColumnDefinitions)
	}
}

func sqlQuery(user string, groups []string, authorizationURL string, authorizationCABundle []byte) string {
	return "SELECT payload FROM status.managed_clusters WHERE TRUE AND " +
		filterByAuthorization(user, groups, authorizationURL, authorizationCABundle, gin.DefaultWriter) +
		" ORDER BY payload -> 'metadata' ->> 'name'"
}

func handleRowsForWatch(ginCtx *gin.Context, query string, dbConnectionPool *pgxpool.Pool) {
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
	previouslyAddedManagedClusterNames := set.NewSet()

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

			doHandleRowsForWatch(ctx, writer, query, dbConnectionPool, previouslyAddedManagedClusterNames)
		}
	}
}

func doHandleRowsForWatch(ctx context.Context, writer io.Writer, query string, dbConnectionPool *pgxpool.Pool,
	previouslyAddedManagedClusterNames set.Set,
) {
	rows, err := dbConnectionPool.Query(ctx, query)
	if err != nil {
		fmt.Fprintf(gin.DefaultWriter, "error in quering managed clusters: %v\n", err)
	}

	addedManagedClusterNames := set.NewSet()

	for rows.Next() {
		managedCluster := &clusterv1.ManagedCluster{}

		err := rows.Scan(managedCluster)
		if err != nil {
			continue
		}

		addedManagedClusterNames.Add(managedCluster.GetName())
		sendWatchEvent(&metav1.WatchEvent{
			Type:   "ADDED",
			Object: runtime.RawExtension{Object: managedCluster},
		}, writer)
	}

	managedClusterNamesToDelete := previouslyAddedManagedClusterNames.Difference(addedManagedClusterNames)

	managedClusterNamesToDeleteIterator := managedClusterNamesToDelete.Iterator()
	for managedClusterNameToDelete := range managedClusterNamesToDeleteIterator.C {
		managedClusterNameToDeleteAsString, ok := managedClusterNameToDelete.(string)
		if !ok {
			continue
		}

		previouslyAddedManagedClusterNames.Remove(managedClusterNameToDeleteAsString)

		managedClusterToDelete := &clusterv1.ManagedCluster{}
		managedClusterToDelete.SetGroupVersionKind(schema.GroupVersionKind{
			Group:   clusterv1.GroupVersion.Group,
			Version: clusterv1.GroupVersion.Version,
			Kind:    "ManagedCluster",
		})
		managedClusterToDelete.SetName(managedClusterNameToDeleteAsString)
		sendWatchEvent(&metav1.WatchEvent{
			Type:   "DELETED",
			Object: runtime.RawExtension{Object: managedClusterToDelete},
		},
			writer)
	}

	managedClusterNamesToAdd := addedManagedClusterNames.Difference(previouslyAddedManagedClusterNames)

	managedClusterNamesToAddIterator := managedClusterNamesToAdd.Iterator()
	for managedClusterNameToAdd := range managedClusterNamesToAddIterator.C {
		managedClusterNameToAddAsString, ok := managedClusterNameToAdd.(string)
		if !ok {
			continue
		}

		previouslyAddedManagedClusterNames.Add(managedClusterNameToAddAsString)
	}

	writer.(http.Flusher).Flush()
}

func sendWatchEvent(watchEvent *metav1.WatchEvent, writer io.Writer) {
	json, err := json.Marshal(watchEvent)
	if err != nil {
		fmt.Fprintf(gin.DefaultWriter, "error in json marshalling: %v\n", err)
		return
	}

	_, err = writer.Write(json)

	if err != nil {
		fmt.Fprintf(gin.DefaultWriter, "error in writing response: %v\n", err)
		return
	}

	_, err = writer.Write([]byte("\n"))

	if err != nil {
		fmt.Fprintf(gin.DefaultWriter, "error in writing response: %v\n", err)
		return
	}
}

func handleRows(ginCtx *gin.Context, query string, dbConnectionPool *pgxpool.Pool,
	customResourceColumnDefinitions []apiextensionsv1.CustomResourceColumnDefinition,
) {
	rows, err := dbConnectionPool.Query(context.TODO(), query)
	if err != nil {
		ginCtx.String(http.StatusInternalServerError, "internal error")
		fmt.Fprintf(gin.DefaultWriter, "error in quering managed clusters: %v\n", err)
	}

	managedClusters := []*clusterv1.ManagedCluster{}

	for rows.Next() {
		managedCluster := &clusterv1.ManagedCluster{}

		err := rows.Scan(managedCluster)
		if err != nil {
			fmt.Fprintf(gin.DefaultWriter, "error in scanning a managed cluster: %v\n", err)
			continue
		}

		managedClusters = append(managedClusters, managedCluster)
	}

	if shouldReturnAsTable(ginCtx) {
		fmt.Fprintf(gin.DefaultWriter, "Returning as table...\n")

		tableConvertor, err := tableconvertor.New(customResourceColumnDefinitions)
		if err != nil {
			fmt.Fprintf(gin.DefaultWriter, "error in creating table convertor: %v\n", err)
			return
		}

		managedClustersList, err := wrapInList(managedClusters)
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

	ginCtx.JSON(http.StatusOK, managedClusters)
}

func wrapInList(managedClusters []*clusterv1.ManagedCluster) (*corev1.List, error) {
	list := corev1.List{
		TypeMeta: metav1.TypeMeta{
			Kind:       "List",
			APIVersion: "v1",
		},
		ListMeta: metav1.ListMeta{},
	}

	for _, cluster := range managedClusters {
		// adopted from
		// https://github.com/kubernetes/kubectl/blob/4da03973dd2fcd4645f20ac669d8a73cb017ff39/pkg/cmd/get/get.go#L786
		clusterData, err := json.Marshal(cluster)
		if err != nil {
			return nil, fmt.Errorf("failed to marshall cluster: %w", err)
		}

		convertedCluster, err := runtime.Decode(unstructured.UnstructuredJSONScheme, clusterData)
		if err != nil {
			return nil, fmt.Errorf("failed to decode: %w", err)
		}

		list.Items = append(list.Items, runtime.RawExtension{Object: convertedCluster})
	}

	return &list, nil
}

func shouldReturnAsTable(ginCtx *gin.Context) bool {
	acceptTableHeader := fmt.Sprintf("application/json;as=Table;v=%s;g=%s",
		metav1.SchemeGroupVersion.Version, metav1.GroupName)

	// implement the real negotiation logic here (with weights)
	// see https://www.w3.org/Protocols/rfc2616/rfc2616-sec14.html
	for _, accepted := range strings.Split(ginCtx.GetHeader("Accept"), ",") {
		if strings.HasPrefix(accepted, acceptTableHeader) {
			return true
		}
	}

	return false
}
