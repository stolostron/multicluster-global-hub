// Copyright (c) 2021 Red Hat, Inc.
// Copyright Contributors to the Open Cluster Management project

package managedclusters

import (
	"context"
	"fmt"
	"net/http"

	"github.com/gin-gonic/gin"
	"github.com/jackc/pgx/v4"
	"github.com/jackc/pgx/v4/pgxpool"
	"k8s.io/apiextensions-apiserver/pkg/registry/customresource/tableconvertor"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	clusterv1 "open-cluster-management.io/api/cluster/v1"

	"github.com/stolostron/multicluster-global-hub/manager/pkg/nonk8sapi/util"
)

func GetManagedCluster(dbConnectionPool *pgxpool.Pool) gin.HandlerFunc {
	return func(ginCtx *gin.Context) {
		clusterID := ginCtx.Param("clusterID")

		managedCluster := &clusterv1.ManagedCluster{}
		err := dbConnectionPool.QueryRow(context.TODO(), `SELECT payload
		FROM status.managed_clusters WHERE cluster_id=$1`, clusterID).Scan(managedCluster)
		if err != nil {
			if err != pgx.ErrNoRows {
				ginCtx.String(http.StatusInternalServerError, serverInternalErrorMsg)
				fmt.Fprintf(gin.DefaultWriter, "error in quering the managed cluster: %v\n", err)
				return
			} else {
				ginCtx.JSON(http.StatusOK, err)
				return
			}
		}

		if util.ShouldReturnAsTable(ginCtx) {
			fmt.Fprintf(gin.DefaultWriter, "Returning as table...\n")

			tableConvertor, err := tableconvertor.New(util.GetCustomResourceColumnDefinitions(crdName,
				clusterv1.GroupVersion.Version))
			if err != nil {
				fmt.Fprintf(gin.DefaultWriter, "error in creating table convertor: %v\n", err)
				return
			}

			table, err := tableConvertor.ConvertToTable(context.TODO(), managedCluster, nil)
			if err != nil {
				fmt.Fprintf(gin.DefaultWriter, "error in converting to table: %v\n", err)
				return
			}

			table.Kind = "Table"
			table.APIVersion = metav1.SchemeGroupVersion.String()
			ginCtx.JSON(http.StatusOK, table)

			return
		}

		ginCtx.JSON(http.StatusOK, managedCluster)
	}
}
