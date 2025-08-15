// Copyright (c) 2022 Red Hat, Inc.
// Copyright Contributors to the Open Cluster Management project

package subscriptions

import (
	"context"
	"encoding/json"
	"fmt"
	"net/http"
	"strconv"

	"github.com/gin-gonic/gin"
	apiextensionsv1 "k8s.io/apiextensions-apiserver/pkg/apis/apiextensions/v1"
	"k8s.io/apiextensions-apiserver/pkg/registry/customresource/tableconvertor"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	appsv1 "open-cluster-management.io/multicloud-operators-subscription/pkg/apis/apps/v1"
	appsv1alpha1 "open-cluster-management.io/multicloud-operators-subscription/pkg/apis/apps/v1alpha1"

	"github.com/stolostron/multicluster-global-hub/manager/pkg/restapis/util"
	"github.com/stolostron/multicluster-global-hub/pkg/database"
)

const (
	subscriptionRepostCRDName = "subscriptionreports.apps.open-cluster-management.io"
	subscriptionStatusCRDName = "subscriptionstatuses.apps.open-cluster-management.io"
	subscriptionQuery         = `SELECT payload->'metadata'->>'name', payload->'metadata'->>'namespace' 
		FROM spec.subscriptions WHERE deleted = FALSE AND id = ?`
	subscriptionReportQuery = `SELECT payload FROM status.subscription_reports
		WHERE payload->'metadata'->>'name'= ? AND payload->'metadata'->>'namespace' = ?`
)

var subReportCustomResourceColumnDefinitions = util.GetCustomResourceColumnDefinitions(subscriptionRepostCRDName,
	appsv1alpha1.SchemeGroupVersion.Version)

// GetSubscriptionReport godoc
// @summary get application subscription report
// @description get report for a given application subscription
// @accept json
// @produce json
// @param        subscriptionID    path    string    true    "Subscription ID"
// @success      200  {object}     appsv1alpha1.SubscriptionReport
// @failure      400
// @failure      401
// @failure      403
// @failure      404
// @failure      500
// @failure      503
// @security     ApiKeyAuth
// @router /subscriptionreport/{subscriptionID} [get]
func GetSubscriptionReport() gin.HandlerFunc {
	return func(ginCtx *gin.Context) {
		subscriptionID := ginCtx.Param("subscriptionID")
		_, _ = fmt.Fprintf(gin.DefaultWriter, "getting subscription report for subscription: %s\n", subscriptionID)
		_, _ = fmt.Fprintf(gin.DefaultWriter, "subscription query with subscription ID: %s\n", subscriptionQuery)
		_, _ = fmt.Fprintf(gin.DefaultWriter, "subscription report query with subscription name and namespace: %v\n",
			subscriptionReportQuery)

		handleSubscriptionReport(ginCtx, subscriptionID,
			subscriptionQuery, subscriptionReportQuery,
			subReportCustomResourceColumnDefinitions)
	}
}

func handleSubscriptionReport(ginCtx *gin.Context, subscriptionID, subscriptionQuery,
	subscriptionReportQuery string, customResourceColumnDefinitions []apiextensionsv1.CustomResourceColumnDefinition,
) {
	subscriptionReport, err := getAggregatedSubscriptionReport(subscriptionID,
		subscriptionQuery, subscriptionReportQuery)
	if err != nil {
		ginCtx.String(http.StatusInternalServerError, serverInternalErrorMsg)
	}

	if util.ShouldReturnAsTable(ginCtx) {
		_, _ = fmt.Fprintf(gin.DefaultWriter, "returning subscription as table...\n")

		tableConvertor, err := tableconvertor.New(customResourceColumnDefinitions)
		if err != nil {
			_, _ = fmt.Fprintf(gin.DefaultWriter, "error in creating table convertor: %v\n", err)
			return
		}

		table, err := tableConvertor.ConvertToTable(context.TODO(), subscriptionReport, nil)
		if err != nil {
			_, _ = fmt.Fprintf(gin.DefaultWriter, "error in converting to table: %v\n", err)
			return
		}

		table.Kind = "Table"
		table.APIVersion = metav1.SchemeGroupVersion.String()
		ginCtx.JSON(http.StatusOK, table)

		return
	}

	ginCtx.JSON(http.StatusOK, subscriptionReport)
}

// returns aggregated SubscriptionReport and error.
func getAggregatedSubscriptionReport(subscriptionID, subscriptionQuery,
	subscriptionReportQuery string,
) (*appsv1alpha1.SubscriptionReport, error) {
	var subscriptionReport *appsv1alpha1.SubscriptionReport
	var subName, subNamespace string
	db := database.GetGorm()
	err := db.Raw(subscriptionQuery, subscriptionID).Row().Scan(&subName, &subNamespace)
	if err != nil {
		_, _ = fmt.Fprintf(gin.DefaultWriter, "error in querying subscription with subscription ID(%s): %v\n", subscriptionID, err)
		return nil, err
	}

	rows, err := db.Raw(subscriptionReportQuery, subName, subNamespace).Rows()
	if err != nil {
		return nil, fmt.Errorf("error in querying subscription-report for subscription(%s/%s): %v",
			subNamespace, subName, err)
	}
	defer func() { _ = rows.Close() }()

	for rows.Next() {
		leafHubSubscriptionReport := appsv1alpha1.SubscriptionReport{}
		var payload []byte
		if err := rows.Scan(&payload); err != nil {
			return nil, fmt.Errorf("error getting subscription report payload for leaf hub: %v", err)
		}

		if err = json.Unmarshal(payload, &leafHubSubscriptionReport); err != nil {
			return nil, fmt.Errorf("error getting subscription report for leaf hub: %v", err)
		}

		// if not updated yet, clone a report from DB and clean it
		if subscriptionReport == nil {
			subscriptionReport = cleanSubscriptionReportObject(leafHubSubscriptionReport)
			continue
		}

		// update aggregated summary
		updateSubscriptionReportSummary(&subscriptionReport.Summary, &leafHubSubscriptionReport.Summary)
		// update results - assuming that MC names are unique across leaf-hubs, we only need to merge
		subscriptionReport.Results = append(subscriptionReport.Results, leafHubSubscriptionReport.Results...)
	}

	return subscriptionReport, nil
}

func cleanSubscriptionReportObject(subscriptionReport appsv1alpha1.SubscriptionReport,
) *appsv1alpha1.SubscriptionReport {
	clone := subscriptionReport.DeepCopy()
	// assign annotations
	clone.Annotations = map[string]string{}
	// assign labels
	clone.Labels = map[string]string{}
	clone.Labels[appsv1.AnnotationHosting] = fmt.Sprintf("%s.%s",
		subscriptionReport.Namespace, subscriptionReport.Name)

	return clone
}

func updateSubscriptionReportSummary(aggregatedSummary *appsv1alpha1.SubscriptionReportSummary,
	reportSummary *appsv1alpha1.SubscriptionReportSummary,
) {
	aggregatedSummary.Deployed = add(aggregatedSummary.Deployed, reportSummary.Deployed)

	aggregatedSummary.InProgress = add(aggregatedSummary.InProgress, reportSummary.InProgress)

	aggregatedSummary.Failed = add(aggregatedSummary.Failed, reportSummary.Failed)

	aggregatedSummary.PropagationFailed = add(aggregatedSummary.PropagationFailed, reportSummary.PropagationFailed)

	aggregatedSummary.Clusters = add(aggregatedSummary.Clusters, reportSummary.Clusters)
}

func add(number1 string, number2 string) string {
	return strconv.Itoa(stringToInt(number1) + stringToInt(number2))
}

func stringToInt(numberString string) int {
	if number, err := strconv.Atoi(numberString); err == nil {
		return number
	}

	return 0
}
