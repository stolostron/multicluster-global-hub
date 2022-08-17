// Copyright (c) 2021 Red Hat, Inc.
// Copyright Contributors to the Open Cluster Management project

package dbsyncer

import (
	"context"
	"fmt"
	"strconv"
	"time"

	"github.com/go-logr/logr"
	"k8s.io/apimachinery/pkg/api/errors"
	appsv1 "open-cluster-management.io/multicloud-operators-subscription/pkg/apis/apps/v1"
	appsv1alpha1 "open-cluster-management.io/multicloud-operators-subscription/pkg/apis/apps/v1alpha1"
	ctrl "sigs.k8s.io/controller-runtime"
	"sigs.k8s.io/controller-runtime/pkg/client"

	"github.com/stolostron/multicluster-global-hub/manager/pkg/specsyncer/db2transport/db"
)

func AddSubscriptionReportDBSyncer(mgr ctrl.Manager, database db.DB,
	statusSyncInterval time.Duration,
) error {
	err := mgr.Add(&genericDBSyncer{
		statusSyncInterval: statusSyncInterval,
		statusSyncFunc: func(ctx context.Context) {
			syncSubscriptionReports(ctx,
				ctrl.Log.WithName("subscription-reports-db-syncer"),
				database,
				mgr.GetClient())
		},
	})
	if err != nil {
		return fmt.Errorf("failed to add subscription reports syncer to the manager: %w", err)
	}

	return nil
}

func syncSubscriptionReports(ctx context.Context, log logr.Logger, database db.DB,
	k8sClient client.Client,
) {
	rows, err := database.GetConn().Query(ctx,
		fmt.Sprintf(`SELECT id, payload->'metadata'->>'name', payload->'metadata'->>'namespace' 
		FROM spec.%s WHERE deleted = FALSE`, subscriptionsSpecTableName))
	if err != nil {
		log.Error(err, "error in getting subscriptions spec")
		return
	}

	for rows.Next() {
		var uid, name, namespace string

		err := rows.Scan(&uid, &name, &namespace)
		if err != nil {
			log.Error(err, "error in select", "table", subscriptionsSpecTableName)
			continue
		}

		go handleSubscriptionReport(ctx, log, database, k8sClient, uid, name, namespace)
	}
}

func handleSubscriptionReport(ctx context.Context, log logr.Logger, database db.DB,
	k8sClient client.Client, specSubscriptionUID string, subscriptionName string, subscriptionNamespace string,
) {
	subscriptionReport, err := getAggregatedSubscriptionReport(ctx, database, subscriptionName,
		subscriptionNamespace)
	if err != nil {
		log.Error(err, "failed to get subscription-report", "name", subscriptionName,
			"namespace", subscriptionNamespace)

		return
	}

	if subscriptionReport == nil { // no status resources found in DB
		return
	}

	// set owner-reference so that the subscription-report is deleted when the subscription is
	setOwnerReference(subscriptionReport, createOwnerReference(appsv1APIGroup, subscriptionKind, subscriptionName,
		specSubscriptionUID))

	if err := updateSubscriptionReport(ctx, k8sClient, subscriptionReport); err != nil {
		log.Error(err, "failed to update subscription-report status")
	}
}

// returns aggregated SubscriptionReport and error.
func getAggregatedSubscriptionReport(ctx context.Context, database db.DB,
	subscriptionName string, subscriptionNamespace string,
) (*appsv1alpha1.SubscriptionReport, error) {
	rows, err := database.GetConn().Query(ctx,
		fmt.Sprintf(`SELECT payload FROM status.%s
			WHERE payload->'metadata'->>'name'=$1 AND payload->'metadata'->>'namespace'=$2`,
			subscriptionReportsStatusTableName), subscriptionName, subscriptionNamespace)
	if err != nil {
		return nil, fmt.Errorf("error in getting subscription-report statuses from DB - %w", err)
	}

	defer rows.Close()

	var subscriptionReport *appsv1alpha1.SubscriptionReport

	for rows.Next() {
		var leafHubSubscriptionReport appsv1alpha1.SubscriptionReport

		if err := rows.Scan(&leafHubSubscriptionReport); err != nil {
			return nil, fmt.Errorf("error getting subscription reports from DB - %w", err)
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

func updateSubscriptionReport(ctx context.Context, k8sClient client.Client,
	aggregatedSubscriptionReport *appsv1alpha1.SubscriptionReport,
) error {
	deployedSubscriptionReport := &appsv1alpha1.SubscriptionReport{}

	err := k8sClient.Get(ctx, client.ObjectKey{
		Name:      aggregatedSubscriptionReport.Name,
		Namespace: aggregatedSubscriptionReport.Namespace,
	}, deployedSubscriptionReport)
	if err != nil {
		if errors.IsNotFound(err) { // create CR
			if err := createK8sResource(ctx, k8sClient, aggregatedSubscriptionReport); err != nil {
				return fmt.Errorf("failed to create subscription-report {name=%s, namespace=%s} - %w",
					aggregatedSubscriptionReport.Name, aggregatedSubscriptionReport.Namespace, err)
			}

			return nil
		}

		return fmt.Errorf("failed to get subscription-report {name=%s, namespace=%s} - %w",
			aggregatedSubscriptionReport.Name, aggregatedSubscriptionReport.Namespace, err)
	}

	// if object exists, clone and update
	originalSubscriptionReport := deployedSubscriptionReport.DeepCopy()

	deployedSubscriptionReport.Summary = aggregatedSubscriptionReport.Summary
	deployedSubscriptionReport.Results = aggregatedSubscriptionReport.Results

	err = k8sClient.Patch(ctx, deployedSubscriptionReport,
		client.MergeFrom(originalSubscriptionReport))
	if err != nil {
		return fmt.Errorf("failed to update subscription-report CR (name=%s, namespace=%s): %w",
			deployedSubscriptionReport.Name, deployedSubscriptionReport.Namespace, err)
	}

	return nil
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

func add(number1 string, number2 string) string {
	return strconv.Itoa(stringToInt(number1) + stringToInt(number2))
}

func stringToInt(numberString string) int {
	if number, err := strconv.Atoi(numberString); err == nil {
		return number
	}

	return 0
}
