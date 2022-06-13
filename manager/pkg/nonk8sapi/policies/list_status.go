// Copyright (c) 2022 Red Hat, Inc.
// Copyright Contributors to the Open Cluster Management project

package policies

import (
	"context"
	"fmt"
	"net/http"

	"github.com/gin-gonic/gin"
	"github.com/jackc/pgx/v4/pgxpool"
	"github.com/stolostron/hub-of-hubs/manager/pkg/nonk8sapi/authentication"
)

// List Policies Status middleware.
func ListStatus(authorizationURL string, authorizationCABundle []byte, dbConnectionPool *pgxpool.Pool) gin.HandlerFunc {
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

		complianceQuerySql := "SELECT id, cluster_name, leaf_hub_name, error, compliance FROM status.compliance ORDER BY cluster_name"
		fmt.Fprintf(gin.DefaultWriter, "query: %v\n", complianceQuerySql)

		handleRows(ginCtx, complianceQuerySql, dbConnectionPool)
	}
}

func handleRows(ginCtx *gin.Context, query string, dbConnectionPool *pgxpool.Pool) {
	rows, err := dbConnectionPool.Query(context.TODO(), query)
	if err != nil {
		ginCtx.String(http.StatusInternalServerError, "internal error")
		fmt.Fprintf(gin.DefaultWriter, "error in quering policies: %v\n", err)
	}

	policiesStatus := []policyStatus{}
	for rows.Next() {
		var id, cluster_name, leaf_hub_name, errInfo, compliance string
		err := rows.Scan(&id, &cluster_name, &leaf_hub_name, &errInfo, &compliance)
		if err != nil {
			fmt.Fprintf(gin.DefaultWriter, "error in scanning a policy: %v\n", err)
			continue
		}
		policyStatus := policyStatus{
			PolicyId:    id,
			ClusterName: cluster_name,
			LeafHubName: leaf_hub_name,
			ErrorInfo:   errInfo,
			Compliance:  compliance,
		}
		policiesStatus = append(policiesStatus, policyStatus)
	}
	ginCtx.JSON(http.StatusOK, policiesStatus)
}

type policyStatus struct {
	PolicyId    string `json:"id"`
	ClusterName string `json:"clusterName"`
	LeafHubName string `json:"leafHubName"`
	ErrorInfo   string `json:"errorInfo"`
	Compliance  string `json:"compliance"`
}
