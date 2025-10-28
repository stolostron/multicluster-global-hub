// Copyright (c) 2022 Red Hat, Inc.
// Copyright Contributors to the Open Cluster Management project

package policies

import (
	"context"
	"database/sql"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"strings"
	"time"

	set "github.com/deckarep/golang-set"
	"github.com/gin-gonic/gin"
	corev1 "k8s.io/api/core/v1"
	apiextensionsv1 "k8s.io/apiextensions-apiserver/pkg/apis/apiextensions/v1"
	"k8s.io/apiextensions-apiserver/pkg/registry/customresource/tableconvertor"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/apis/meta/v1/unstructured"
	"k8s.io/apimachinery/pkg/runtime"
	"k8s.io/apimachinery/pkg/runtime/schema"
	policyv1 "open-cluster-management.io/governance-policy-propagator/api/v1"

	"github.com/stolostron/multicluster-global-hub/manager/pkg/restapis/util"
	"github.com/stolostron/multicluster-global-hub/pkg/database"
	"github.com/stolostron/multicluster-global-hub/pkg/database/models"
)

var dbEnumToPolicyComplianceStateMap = map[string]policyv1.ComplianceState{
	dbEnumCompliant:    policyv1.Compliant,
	dbEnumNonCompliant: policyv1.NonCompliant,
}

type policySummary struct {
	ComplianceClusterNumber    int32 `json:"complianceClusterNumber,omitempty"`
	NonComplianceClusterNumber int32 `json:"nonComplianceClusterNumber,omitempty"`
}

type policyMatch struct {
	policy           string
	placementrule    string
	placementbinding string
}

var (
	policyMatches                   = []*policyMatch{}
	customResourceColumnDefinitions = util.GetCustomResourceColumnDefinitions(crdName, policyv1.GroupVersion.Version)
)

// ListPolicies godoc
// @summary list policies
// @description list policies
// @accept json
// @produce json
// @param        labelSelector    query     string  false  "list policies by label selector"
// @param        limit            query     int     false  "maximum policy number to receive"
// @param        continue         query     string  false  "continue token to request next request"
// @success      200  {object}    policyv1.PolicyList
// @failure      400
// @failure      401
// @failure      403
// @failure      404
// @failure      500
// @failure      503
// @security     ApiKeyAuth
// @router /policies [get]
func ListPolicies() gin.HandlerFunc {
	return func(ginCtx *gin.Context) {
		labelSelector := ginCtx.Query("labelSelector")

		selectorInSql := ""
		if labelSelector != "" {
			var err error
			selectorInSql, err = util.ParseLabelSelector(labelSelector)
			if err != nil {
				_, _ = fmt.Fprintf(gin.DefaultWriter, "failed to parse label selector: %s\n", err.Error())
				return
			}
		}

		_, _ = fmt.Fprintf(gin.DefaultWriter, "parsed selector: %s\n", selectorInSql)

		limit := ginCtx.Query("limit")
		_, _ = fmt.Fprintf(gin.DefaultWriter, "limit: %v\n", limit)

		continueToken := ginCtx.Query("continue")

		lastPolicyName, lastPolicyUID := "", ""
		if continueToken != "" {
			_, _ = fmt.Fprintf(gin.DefaultWriter, "continue: %v\n", continueToken)

			var err error
			lastPolicyName, lastPolicyUID, err = util.DecodeContinue(continueToken)
			if err != nil {
				_, _ = fmt.Fprintf(gin.DefaultWriter, "failed to decode continue token: %s\n", err.Error())
				return
			}
		}

		_, _ = fmt.Fprintf(gin.DefaultWriter,
			"last returned policy name: %s, last returned policy] UID: %s\n",
			lastPolicyName,
			lastPolicyUID)

		// build query condition for paging
		LastResourceCompareCondition := fmt.Sprintf(
			"(payload -> 'metadata' ->> 'name', payload -> 'metadata' ->> 'uid') > ('%s', '%s') ",
			lastPolicyName,
			lastPolicyUID)

		// policy list query order by name and uid
		policyListQuery := "SELECT id, payload FROM spec.policies WHERE deleted = FALSE AND " +
			LastResourceCompareCondition +
			selectorInSql +
			" ORDER BY (payload -> 'metadata' ->> 'name', payload -> 'metadata' ->> 'uid')"

		// add limit
		if limit != "" {
			policyListQuery += fmt.Sprintf(" LIMIT %s", limit)
		}

		// last policy order by name and uid query
		lastPolicyQuery := "SELECT id, payload FROM spec.policies WHERE deleted = FALSE " +
			"ORDER BY (payload -> 'metadata' ->> 'name', payload -> 'metadata' ->> 'uid') DESC LIMIT 1"

		_, _ = fmt.Fprintf(gin.DefaultWriter, "last policy query: %v\n", lastPolicyQuery)
		_, _ = fmt.Fprintf(gin.DefaultWriter, "policy list query: %v\n", policyListQuery)
		_, _ = fmt.Fprintf(gin.DefaultWriter, "policy compliance query with policy ID: %v\n", policyComplianceQuery)
		_, _ = fmt.Fprintf(gin.DefaultWriter, "policy&placementbinding&placementrule mapping query: %v\n", policyMappingQuery)

		if _, watch := ginCtx.GetQuery("watch"); watch {
			handlePoliciesForWatch(ginCtx, policyListQuery, policyMappingQuery, policyComplianceQuery)
			return
		}

		handlePolicies(ginCtx, policyListQuery, lastPolicyQuery, policyMappingQuery,
			policyComplianceQuery, customResourceColumnDefinitions)
	}
}

func handlePoliciesForWatch(ginCtx *gin.Context, policyListQuery, policyMappingQuery,
	policyComplianceQuery string,
) {
	writer := ginCtx.Writer
	header := writer.Header()
	header.Set("Transfer-Encoding", "chunked")
	header.Set("Content-Type", "application/json")
	writer.WriteHeader(http.StatusOK)

	ticker := time.NewTicker(syncIntervalInSeconds * time.Second)

	ctx, cancelContext := context.WithCancel(context.Background())
	defer cancelContext()

	preAddedPolicies := set.NewSet()

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

			doHandlePoliciesForWatch(ctx, writer, policyListQuery, policyMappingQuery,
				policyComplianceQuery, preAddedPolicies)
		}
	}
}

func doHandlePoliciesForWatch(ctx context.Context, writer gin.ResponseWriter,
	policyListQuery, policyMappingQuery, policyComplianceQuery string, preAddedPolicies set.Set,
) {
	var err error
	policyMatches, err = getPolicyMatches(ctx, policyMappingQuery)
	if err != nil {
		_, _ = fmt.Fprintf(gin.DefaultWriter, QueryPolicyMappingFailureFormatMsg, err)
	}
	db := database.GetGorm()
	policyRows, err := db.WithContext(ctx).Raw(policyListQuery).Rows()
	if err != nil {
		_, _ = fmt.Fprintf(gin.DefaultWriter, QueryPoliciesFailureFormatMsg, err)
	}

	defer func() { _ = policyRows.Close() }()

	addedPolicies := set.NewSet()
	for policyRows.Next() {
		policyID, policy := "", &policyv1.Policy{}
		var payload []byte
		if err := policyRows.Scan(&policyID, &payload); err != nil {
			_, _ = fmt.Fprintf(gin.DefaultWriter, "error in scanning a policy: %v\n", err)
			continue
		}

		if err := json.Unmarshal(payload, policy); err != nil {
			_, _ = fmt.Fprintf(gin.DefaultWriter, "error in scanning a policyPayload: %v\n", err)
			continue
		}

		addedPolicies.Add(policyID + "/" + policy.GetName())
		if err := sendPolicyWatchEvent(ctx, writer, policy, "ADDED",
			policyComplianceQuery, policyID); err != nil {
			_, _ = fmt.Fprintf(gin.DefaultWriter, "error in sending watch event: %v\n", err)
		}
	}

	policiesToDelete := preAddedPolicies.Difference(addedPolicies)

	policiesToDeleteIterator := policiesToDelete.Iterator()
	for policyToDelete := range policiesToDeleteIterator.C {
		policyToDeleteAsString, ok := policyToDelete.(string)
		if !ok {
			continue
		}

		preAddedPolicies.Remove(policyToDeleteAsString)
		policyToDeleteAsStrList := strings.Split(policyToDeleteAsString, "/")
		policyID := policyToDeleteAsStrList[0]
		policyNameName := policyToDeleteAsStrList[1]

		policyInstanceToDelete := &policyv1.Policy{}
		policyInstanceToDelete.SetGroupVersionKind(schema.GroupVersionKind{
			Group:   policyv1.GroupVersion.Group,
			Version: policyv1.GroupVersion.Version,
			Kind:    "Policy",
		})
		policyInstanceToDelete.SetName(policyNameName)
		if err := sendPolicyWatchEvent(ctx, writer, policyInstanceToDelete, "DELETED",
			policyComplianceQuery, policyID); err != nil {
			_, _ = fmt.Fprintf(gin.DefaultWriter, "error in sending watch event: %v\n", err)
		}
	}

	policiesToAdd := addedPolicies.Difference(preAddedPolicies)

	policiesToAddIterator := policiesToAdd.Iterator()
	for policyToAdd := range policiesToAddIterator.C {
		policyToAddAsString, ok := policyToAdd.(string)
		if !ok {
			continue
		}

		preAddedPolicies.Add(policyToAddAsString)
	}

	writer.(http.Flusher).Flush()
}

func sendPolicyWatchEvent(ctx context.Context, writer io.Writer, policy *policyv1.Policy, eventType,
	policyComplianceQuery, policyID string,
) error {
	// add policy placement
	policy.Status.Placement = []*policyv1.Placement{}
	for _, pm := range policyMatches {
		if pm.policy == policy.GetName() {
			policy.Status.Placement = append(policy.Status.Placement, &policyv1.Placement{
				PlacementRule:    pm.placementrule,
				PlacementBinding: pm.placementbinding,
			})
		}
	}

	compliancePerClusterStatuses, hasNonCompliantClusters, err := getComplianceStatus(
		ctx, policyComplianceQuery, policyID)
	if err != nil {
		return fmt.Errorf("error in querying compliance status of a policy with UID: %s - %w", policyID, err)
	}

	policy.Status.Status = compliancePerClusterStatuses
	policy.Status.ComplianceState = ""

	if hasNonCompliantClusters {
		policy.Status.ComplianceState = policyv1.NonCompliant
	} else if len(compliancePerClusterStatuses) > 0 {
		policy.Status.ComplianceState = policyv1.Compliant
	}

	return util.SendWatchEvent(&metav1.WatchEvent{
		Type:   eventType,
		Object: runtime.RawExtension{Object: policy},
	}, writer)
}

func handlePolicies(ginCtx *gin.Context, policyListQuery, lastPolicyQuery,
	policyMappingQuery, policyComplianceQuery string,
	customResourceColumnDefinitions []apiextensionsv1.CustomResourceColumnDefinition,
) {
	db := database.GetGorm()
	ctx := ginCtx.Request.Context()
	lastPolicy := &policyv1.Policy{}
	lastPolicyID := ""
	var lastPolicyPayload []byte
	err := db.WithContext(ctx).Raw(lastPolicyQuery).Row().Scan(&lastPolicyID, &lastPolicyPayload)
	if err != nil && err != sql.ErrNoRows {
		ginCtx.String(http.StatusInternalServerError, ServerInternalErrorMsg)
		_, _ = fmt.Fprintf(gin.DefaultWriter, "error in querying last policy: %v\n", err)
		return
	}

	if err == nil {
		if e := json.Unmarshal(lastPolicyPayload, lastPolicy); e != nil {
			_, _ = fmt.Fprintf(gin.DefaultWriter, "error in querying last policy payload: %v\n", e)
			return
		}
	}

	policyMatches, err = getPolicyMatches(ctx, policyMappingQuery)
	if err != nil {
		ginCtx.String(http.StatusInternalServerError, ServerInternalErrorMsg)
		_, _ = fmt.Fprintf(gin.DefaultWriter, QueryPolicyMappingFailureFormatMsg, err)
	}

	policyRows, err := db.WithContext(ctx).Raw(policyListQuery).Rows()
	if err != nil {
		ginCtx.String(http.StatusInternalServerError, ServerInternalErrorMsg)
		_, _ = fmt.Fprintf(gin.DefaultWriter, QueryPoliciesFailureFormatMsg, err)
	}
	defer func() { _ = policyRows.Close() }()

	unstrPolicyList := &unstructured.UnstructuredList{
		Object: map[string]interface{}{
			"kind":       "PolicyList",
			"apiVersion": "policy.open-cluster-management.io/v1",
		},
		Items: []unstructured.Unstructured{},
	}
	policyName, policyUID := "", ""
	for policyRows.Next() {
		var policyPayload []byte
		if err := policyRows.Scan(&policyUID, &policyPayload); err != nil {
			_, _ = fmt.Fprintf(gin.DefaultWriter, "error in scanning a policy: %v\n", err)
			continue
		}
		policy := &policyv1.Policy{}
		if err := json.Unmarshal(policyPayload, policy); err != nil {
			_, _ = fmt.Fprintf(gin.DefaultWriter, "error in Unmarshal a policyPayload : %v\n", err)
			continue
		}

		compliancePerClusterStatuses, hasNonCompliantClusters, err := getComplianceStatus(ctx,
			policyComplianceQuery, policyUID)
		if err != nil {
			_, _ = fmt.Fprintf(gin.DefaultWriter, QueryPolicyComplianceFailureFormatMsg, err)
			continue
		}

		unstrPolicy, err := assemblePolicyStatus(policy, policyMatches,
			compliancePerClusterStatuses, hasNonCompliantClusters)
		if err != nil {
			_, _ = fmt.Fprintf(gin.DefaultWriter, "error in assemble status: %v\n", err)
			continue
		}

		unstrPolicyList.Items = append(unstrPolicyList.Items, unstrPolicy)
		policyName = policy.GetName()
	}

	if policyUID != "" &&
		lastPolicyID != "" &&
		policyUID != lastPolicyID &&
		policyName != "" &&
		lastPolicy.GetName() != "" &&
		policyName != lastPolicy.GetName() {
		continueToken, err := util.EncodeContinue(policyName, policyUID)
		if err != nil {
			_, _ = fmt.Fprintf(gin.DefaultWriter, "error in encoding the continue token: %v\n", err)
			return
		}

		unstrPolicyList.SetContinue(continueToken)
	}

	if util.ShouldReturnAsTable(ginCtx) {
		_, _ = fmt.Fprintf(gin.DefaultWriter, "Returning as table...\n")

		tableConvertor, err := tableconvertor.New(customResourceColumnDefinitions)
		if err != nil {
			_, _ = fmt.Fprintf(gin.DefaultWriter, "error in creating table convertor: %v\n", err)
			return
		}

		policiesList, err := wrapObjectsInList(unstrPolicyList.Items)
		if err != nil {
			_, _ = fmt.Fprintf(gin.DefaultWriter, "error in wrapping policies in a list: %v\n", err)
			return
		}

		table, err := tableConvertor.ConvertToTable(context.TODO(), policiesList, nil)
		if err != nil {
			_, _ = fmt.Fprintf(gin.DefaultWriter, "error in converting to table: %v\n", err)
			return
		}

		table.Kind = "Table"
		table.APIVersion = metav1.SchemeGroupVersion.String()
		ginCtx.JSON(http.StatusOK, table)

		return
	}

	ginCtx.JSON(http.StatusOK, unstrPolicyList)
}

// getPolicyMatches returns array of policy & placementbinding & placementrule mapping and error.
func getPolicyMatches(ctx context.Context, policyMappingQuery string) ([]*policyMatch, error) {
	policyMatches := []*policyMatch{}

	db := database.GetGorm()
	policyMatchRows, err := db.WithContext(ctx).Raw(policyMappingQuery).Rows()
	if err != nil {
		return policyMatches,
			fmt.Errorf("error in querying policy & placementbinding & placementrule mapping: - %w", err)
	}

	defer func() { _ = policyMatchRows.Close() }()

	for policyMatchRows.Next() {
		var policyName, placementbindingName, placementruleName string
		if err := policyMatchRows.Scan(&policyName, &placementbindingName, &placementruleName); err != nil {
			_, _ = fmt.Fprintf(gin.DefaultWriter,
				"error in reading policy & placementbinding & placementrule match table rows: %v\n", err)
			continue
		}

		policyMatches = append(policyMatches, &policyMatch{policyName, placementruleName, placementbindingName})
	}

	return policyMatches, nil
}

// getComplianceStatus returns array of CompliancePerClusterStatus,
// whether the policy has any NonCompliant cluster, and error.
func getComplianceStatus(ctx context.Context, policyComplianceQuery, policyID string,
) ([]*policyv1.CompliancePerClusterStatus, bool, error) {
	compliancePerClusterStatuses := []*policyv1.CompliancePerClusterStatus{}
	hasNonCompliantClusters := false

	db := database.GetGorm()
	var statusCompliances []models.StatusCompliance
	err := db.WithContext(ctx).Where(&models.StatusCompliance{
		PolicyID: policyID,
	}).Order("leaf_hub_name asc").Order("cluster_name").Find(&statusCompliances).Error
	if err != nil {
		return compliancePerClusterStatuses, hasNonCompliantClusters,
			fmt.Errorf("error in querying policy  status compliance: - %w", err)
	}

	policyComplianceRows, err := db.WithContext(ctx).Raw(policyComplianceQuery, policyID).Rows()
	if err != nil {
		return compliancePerClusterStatuses, hasNonCompliantClusters,
			fmt.Errorf("error in querying policy compliances: - %w", err)
	}
	defer func() { _ = policyComplianceRows.Close() }()

	for policyComplianceRows.Next() {
		var clusterName, leafHubName, complianceInDB string
		if err := policyComplianceRows.Scan(&clusterName, &leafHubName, &complianceInDB); err != nil {
			return []*policyv1.CompliancePerClusterStatus{}, false, err
		}

		compliance := dbEnumToPolicyComplianceStateMap[complianceInDB]
		if compliance == policyv1.NonCompliant {
			hasNonCompliantClusters = true
		}

		compliancePerClusterStatuses = append(compliancePerClusterStatuses, &policyv1.CompliancePerClusterStatus{
			ComplianceState:  compliance,
			ClusterName:      clusterName,
			ClusterNamespace: clusterName,
		})
	}

	return compliancePerClusterStatuses, hasNonCompliantClusters, nil
}

func assemblePolicyStatus(policy *policyv1.Policy, policyMatches []*policyMatch,
	compliancePerClusterStatuses []*policyv1.CompliancePerClusterStatus, hasNonCompliantClusters bool,
) (unstructured.Unstructured, error) {
	policy.Status.Placement = []*policyv1.Placement{}
	for _, pm := range policyMatches {
		if pm.policy == policy.GetName() {
			policy.Status.Placement = append(policy.Status.Placement, &policyv1.Placement{
				PlacementRule:    pm.placementrule,
				PlacementBinding: pm.placementbinding,
			})
		}
	}

	policy.Status.Status = compliancePerClusterStatuses
	policy.Status.ComplianceState = ""

	if hasNonCompliantClusters {
		policy.Status.ComplianceState = policyv1.NonCompliant
	} else if len(compliancePerClusterStatuses) > 0 {
		policy.Status.ComplianceState = policyv1.Compliant
	}

	unstrPolicyObj, err := runtime.DefaultUnstructuredConverter.ToUnstructured(policy)
	if err != nil {
		return unstructured.Unstructured{}, err
	}

	unstrPolicy := unstructured.Unstructured{
		Object: unstrPolicyObj,
	}

	policyStatusObj := unstrPolicy.Object["status"].(map[string]interface{})

	var complianceClusterNumber, nonComplianceClusterNumber int32 = 0, 0
	for _, compliancePerClusterStatus := range compliancePerClusterStatuses {
		switch compliancePerClusterStatus.ComplianceState {
		case policyv1.Compliant:
			complianceClusterNumber += 1
		case policyv1.NonCompliant:
			nonComplianceClusterNumber += 1
		}
	}

	// policy status summary information
	policyStatusObj["summary"] = policySummary{
		ComplianceClusterNumber:    complianceClusterNumber,
		NonComplianceClusterNumber: nonComplianceClusterNumber,
	}

	return unstrPolicy, nil
}

func wrapObjectsInList(uns []unstructured.Unstructured) (*corev1.List, error) {
	list := &corev1.List{
		TypeMeta: metav1.TypeMeta{
			Kind:       "List",
			APIVersion: "v1",
		},
		ListMeta: metav1.ListMeta{},
	}

	for _, instrObj := range uns {
		// adopted from
		// https://github.com/kubernetes/kubectl/blob/4da03973dd2fcd4645f20ac669d8a73cb017ff39/pkg/cmd/get/get.go#L786
		instrObjBytes, err := json.Marshal(&instrObj) // #nosec G601
		if err != nil {
			return nil, fmt.Errorf("failed to marshall object: %w", err)
		}

		convertedObj, err := runtime.Decode(unstructured.UnstructuredJSONScheme, instrObjBytes)
		if err != nil {
			return nil, fmt.Errorf("failed to decode with unstructured JSON scheme : %w", err)
		}

		list.Items = append(list.Items, runtime.RawExtension{Object: convertedObj})
	}

	return list, nil
}
