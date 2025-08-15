// Copyright (c) 2022 Red Hat, Inc.
// Copyright Contributors to the Open Cluster Management project

package policies

import (
	"context"
	"encoding/json"
	"fmt"
	"net/http"
	"time"

	"github.com/gin-gonic/gin"
	apiextensionsv1 "k8s.io/apiextensions-apiserver/pkg/apis/apiextensions/v1"
	"k8s.io/apiextensions-apiserver/pkg/registry/customresource/tableconvertor"
	apiequality "k8s.io/apimachinery/pkg/api/equality"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/apis/meta/v1/unstructured"
	"k8s.io/apimachinery/pkg/runtime"
	policyv1 "open-cluster-management.io/governance-policy-propagator/api/v1"

	"github.com/stolostron/multicluster-global-hub/manager/pkg/restapis/util"
	"github.com/stolostron/multicluster-global-hub/pkg/database"
)

// GetPolicyStatus godoc
// @summary get policy status
// @description get status with a given policy
// @accept json
// @produce json
// @param        policyID    path    string    true    "Policy ID"
// @success      200  {object}  policyv1.Policy
// @failure      400
// @failure      401
// @failure      403
// @failure      404
// @failure      500
// @failure      503
// @security     ApiKeyAuth
// @router /policy/{policyID}/status [get]
func GetPolicyStatus() gin.HandlerFunc {
	return func(ginCtx *gin.Context) {
		policyID := ginCtx.Param("policyID")
		_, _ = fmt.Fprintf(gin.DefaultWriter, "getting status for policy: %s\n", policyID)
		_, _ = fmt.Fprintf(gin.DefaultWriter, "policy query with policy ID: %s\n", policyQuery)
		_, _ = fmt.Fprintf(gin.DefaultWriter, "policy compliance query with policy ID: %v\n", policyComplianceQuery)
		_, _ = fmt.Fprintf(gin.DefaultWriter, "policy&placementbinding&placementrule mapping query: %v\n", policyMappingQuery)

		if _, watch := ginCtx.GetQuery("watch"); watch {
			handlePolicyForWatch(ginCtx, policyID, policyQuery,
				policyMappingQuery, policyComplianceQuery)
			return
		}

		handlePolicy(ginCtx, policyID, policyQuery, policyMappingQuery, policyComplianceQuery,
			customResourceColumnDefinitions)
	}
}

func handlePolicyForWatch(ginCtx *gin.Context, policyID, policyQuery, policyMappingQuery, policyComplianceQuery string,
) {
	writer := ginCtx.Writer
	header := writer.Header()
	header.Set("Transfer-Encoding", "chunked")
	header.Set("Content-Type", "application/json")
	writer.WriteHeader(http.StatusOK)

	ticker := time.NewTicker(syncIntervalInSeconds * time.Second)

	ctx, cancelContext := context.WithCancel(context.Background())
	defer cancelContext()

	preUnstrPolicy, err := queryPolicyStatus(policyID, policyQuery, policyMappingQuery, policyComplianceQuery)
	if err != nil {
		ginCtx.String(http.StatusInternalServerError, ServerInternalErrorMsg)
	}

	// no need to return policy spec
	delete(preUnstrPolicy.Object, "spec")

	// send init watch event
	if err := util.SendWatchEvent(&metav1.WatchEvent{
		Type:   "UPDATED",
		Object: runtime.RawExtension{Object: preUnstrPolicy},
	}, writer); err != nil {
		_, _ = fmt.Fprintf(gin.DefaultWriter, "error in sending watch event: %v\n", err)
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

			doHandlePolicyForWatch(ctx, writer, policyID, policyQuery, policyMappingQuery,
				policyComplianceQuery, preUnstrPolicy)
		}
	}
}

func doHandlePolicyForWatch(ctx context.Context, writer gin.ResponseWriter, policyID,
	policyQuery, policyMappingQuery, policyComplianceQuery string, preUnstrPolicy *unstructured.Unstructured,
) {
	curUnstrPolicy, err := queryPolicyStatus(policyID, policyQuery, policyMappingQuery, policyComplianceQuery)
	if err != nil {
		_, _ = fmt.Fprintf(gin.DefaultWriter, "error in getting policy status with policy ID(%s): %v", policyID, err)
	}

	curPolicyStatusObj := curUnstrPolicy.Object["status"].(map[string]interface{})
	prePolicyStatusObj := preUnstrPolicy.Object["status"].(map[string]interface{})
	if !apiequality.Semantic.DeepDerivative(curPolicyStatusObj, prePolicyStatusObj) {
		// no need to return policy spec
		delete(curUnstrPolicy.Object, "spec")

		if err := util.SendWatchEvent(&metav1.WatchEvent{
			Type:   "UPDATED",
			Object: runtime.RawExtension{Object: curUnstrPolicy},
		}, writer); err != nil {
			_, _ = fmt.Fprintf(gin.DefaultWriter, "error in sending watch event: %v\n", err)
		}

		// set policy
		preUnstrPolicy = curUnstrPolicy
	}

	writer.(http.Flusher).Flush()
}

func handlePolicy(ginCtx *gin.Context, policyID, policyQuery, policyMappingQuery,
	policyComplianceQuery string, customResourceColumnDefinitions []apiextensionsv1.CustomResourceColumnDefinition,
) {
	unstrPolicy, err := queryPolicyStatus(policyID,
		policyQuery, policyMappingQuery, policyComplianceQuery)
	if err != nil {
		ginCtx.String(http.StatusInternalServerError, ServerInternalErrorMsg)
	}

	// no need to return unstrPolicy spec
	delete(unstrPolicy.Object, "spec")

	if util.ShouldReturnAsTable(ginCtx) {
		_, _ = fmt.Fprintf(gin.DefaultWriter, "returning policy as table...\n")

		tableConvertor, err := tableconvertor.New(customResourceColumnDefinitions)
		if err != nil {
			_, _ = fmt.Fprintf(gin.DefaultWriter, "error in creating table convertor: %v\n", err)
			return
		}

		table, err := tableConvertor.ConvertToTable(context.TODO(), unstrPolicy, nil)
		if err != nil {
			_, _ = fmt.Fprintf(gin.DefaultWriter, "error in converting to table: %v\n", err)
			return
		}

		table.Kind = "Table"
		table.APIVersion = metav1.SchemeGroupVersion.String()
		ginCtx.JSON(http.StatusOK, table)

		return
	}

	ginCtx.JSON(http.StatusOK, unstrPolicy)
}

func queryPolicyStatus(policyID, policyQuery, policyMappingQuery,
	policyComplianceQuery string,
) (*unstructured.Unstructured, error) {
	var err error
	policy := &policyv1.Policy{}

	policyMatches, err = getPolicyMatches(policyMappingQuery)
	if err != nil {
		_, _ = fmt.Fprintf(gin.DefaultWriter, QueryPolicyMappingFailureFormatMsg, err)
		return &unstructured.Unstructured{}, err
	}

	db := database.GetGorm()
	var payload []byte
	err = db.Raw(policyQuery, policyID).Row().Scan(&payload)
	if err != nil {
		_, _ = fmt.Fprintf(gin.DefaultWriter, QueryPolicyFailureFormatMsg, err)
		return &unstructured.Unstructured{}, err
	}
	err = json.Unmarshal(payload, policy)
	if err != nil {
		return &unstructured.Unstructured{}, err
	}

	compliancePerClusterStatuses, hasNonCompliantClusters, err := getComplianceStatus(
		policyComplianceQuery, policyID)
	if err != nil {
		_, _ = fmt.Fprintf(gin.DefaultWriter, QueryPolicyComplianceFailureFormatMsg, err)
		return &unstructured.Unstructured{}, err
	}

	unstrPolicy, err := assemblePolicyStatus(policy, policyMatches,
		compliancePerClusterStatuses, hasNonCompliantClusters)

	return &unstrPolicy, err
}
