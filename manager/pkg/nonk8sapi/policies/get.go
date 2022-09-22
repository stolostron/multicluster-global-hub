// Copyright (c) 2022 Red Hat, Inc.
// Copyright Contributors to the Open Cluster Management project

package policies

import (
	"context"
	"fmt"
	"net/http"
	"time"

	"github.com/gin-gonic/gin"
	"github.com/jackc/pgx/v4/pgxpool"
	apiextensionsv1 "k8s.io/apiextensions-apiserver/pkg/apis/apiextensions/v1"
	"k8s.io/apiextensions-apiserver/pkg/registry/customresource/tableconvertor"
	apiequality "k8s.io/apimachinery/pkg/api/equality"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/runtime"
	policyv1 "open-cluster-management.io/governance-policy-propagator/api/v1"

	"github.com/stolostron/multicluster-global-hub/manager/pkg/nonk8sapi/util"
)

// GetStatus middleware.
func GetStatus(dbConnectionPool *pgxpool.Pool) gin.HandlerFunc {
	return func(ginCtx *gin.Context) {
		policyID := ginCtx.Param("policyID")
		fmt.Fprintf(gin.DefaultWriter, "getting status for policy: %s\n", policyID)
		fmt.Fprintf(gin.DefaultWriter, "policy query with policy ID: %s\n", policyQuery)
		fmt.Fprintf(gin.DefaultWriter, "policy compliance query with policy ID: %v\n", policyComplianceQuery)
		fmt.Fprintf(gin.DefaultWriter, "policy&placementbinding&placementrule mapping query: %v\n", policyMappingQuery)

		if _, watch := ginCtx.GetQuery("watch"); watch {
			handlePolicyForWatch(ginCtx, dbConnectionPool, policyID, policyQuery,
				policyMappingQuery, policyComplianceQuery)
			return
		}

		handlePolicy(ginCtx, dbConnectionPool, policyID, policyQuery, policyMappingQuery, policyComplianceQuery,
			customResourceColumnDefinitions)
	}
}

func handlePolicyForWatch(ginCtx *gin.Context, dbConnectionPool *pgxpool.Pool, policyID,
	policyQuery, policyMappingQuery, policyComplianceQuery string,
) {
	writer := ginCtx.Writer
	header := writer.Header()
	header.Set("Transfer-Encoding", "chunked")
	header.Set("Content-Type", "application/json")
	writer.WriteHeader(http.StatusOK)

	ticker := time.NewTicker(syncIntervalInSeconds * time.Second)

	ctx, cancelContext := context.WithCancel(context.Background())
	defer cancelContext()

	prePolicy, err := queryPolicyStatus(dbConnectionPool, policyID,
		policyQuery, policyMappingQuery, policyComplianceQuery)
	if err != nil {
		ginCtx.String(http.StatusInternalServerError, ServerInternalErrorMsg)
	}

	// no need to return policy spec
	prePolicy.Spec = policyv1.PolicySpec{}

	// send init watch event
	if err := util.SendWatchEvent(&metav1.WatchEvent{
		Type:   "UPDATED",
		Object: runtime.RawExtension{Object: prePolicy},
	}, writer); err != nil {
		fmt.Fprintf(gin.DefaultWriter, "error in sending watch event: %v\n", err)
	}

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

			doHandlePolicyForWatch(ctx, writer, dbConnectionPool, policyID, policyQuery, policyMappingQuery,
				policyComplianceQuery, prePolicy)
		}
	}
}

func doHandlePolicyForWatch(ctx context.Context, writer gin.ResponseWriter, dbConnectionPool *pgxpool.Pool,
	policyID, policyQuery, policyMappingQuery, policyComplianceQuery string, prePolicy *policyv1.Policy,
) {
	curPolicy, err := queryPolicyStatus(dbConnectionPool, policyID,
		policyQuery, policyMappingQuery, policyComplianceQuery)
	if err != nil {
		fmt.Fprintf(gin.DefaultWriter, "error in getting policy status with policy ID(%s): %v", policyID, err)
	}

	if !apiequality.Semantic.DeepDerivative(curPolicy.Status, prePolicy.Status) {
		// no need to return policy spec
		curPolicy.Spec = policyv1.PolicySpec{}

		if err := util.SendWatchEvent(&metav1.WatchEvent{
			Type:   "UPDATED",
			Object: runtime.RawExtension{Object: curPolicy},
		}, writer); err != nil {
			fmt.Fprintf(gin.DefaultWriter, "error in sending watch event: %v\n", err)
		}

		// set policy
		prePolicy = curPolicy
	}

	writer.(http.Flusher).Flush()
}

func handlePolicy(ginCtx *gin.Context, dbConnectionPool *pgxpool.Pool, policyID, policyQuery, policyMappingQuery,
	policyComplianceQuery string, customResourceColumnDefinitions []apiextensionsv1.CustomResourceColumnDefinition,
) {
	policy, err := queryPolicyStatus(dbConnectionPool, policyID,
		policyQuery, policyMappingQuery, policyComplianceQuery)
	if err != nil {
		ginCtx.String(http.StatusInternalServerError, ServerInternalErrorMsg)
	}

	// no need to return policy spec
	policy.Spec = policyv1.PolicySpec{}

	if util.ShouldReturnAsTable(ginCtx) {
		fmt.Fprintf(gin.DefaultWriter, "returning policy as table...\n")

		tableConvertor, err := tableconvertor.New(customResourceColumnDefinitions)
		if err != nil {
			fmt.Fprintf(gin.DefaultWriter, "error in creating table convertor: %v\n", err)
			return
		}

		table, err := tableConvertor.ConvertToTable(context.TODO(), policy, nil)
		if err != nil {
			fmt.Fprintf(gin.DefaultWriter, "error in converting to table: %v\n", err)
			return
		}

		table.Kind = "Table"
		table.APIVersion = metav1.SchemeGroupVersion.String()
		ginCtx.JSON(http.StatusOK, table)

		return
	}

	ginCtx.JSON(http.StatusOK, policy)
}

func queryPolicyStatus(dbConnectionPool *pgxpool.Pool, policyID, policyQuery, policyMappingQuery,
	policyComplianceQuery string,
) (*policyv1.Policy, error) {
	var err error
	policy := &policyv1.Policy{}

	policyMatches, err = getPolicyMatches(dbConnectionPool, policyMappingQuery)
	if err != nil {
		fmt.Fprintf(gin.DefaultWriter, QueryPolicyMappingFailureFormatMsg, err)
		return policy, err
	}

	err = dbConnectionPool.QueryRow(context.TODO(), policyQuery, policyID).Scan(policy)
	if err != nil {
		fmt.Fprintf(gin.DefaultWriter, QueryPolicyFailureFormatMsg, err)
		return policy, err
	}

	compliancePerClusterStatuses, hasNonCompliantClusters, err := getComplianceStatus(dbConnectionPool,
		policyComplianceQuery, policyID)
	if err != nil {
		fmt.Fprintf(gin.DefaultWriter, QueryPolicyComplianceFailureFormatMsg, err)
		return policy, err
	}

	return assemblePolicyStatus(policy, policyMatches, compliancePerClusterStatuses, hasNonCompliantClusters), nil
}
