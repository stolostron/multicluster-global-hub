// Copyright (c) 2021 Red Hat, Inc.
// Copyright Contributors to the Open Cluster Management project

package dbsyncer

import (
	"context"
	"fmt"
	"time"

	"github.com/go-logr/logr"
	"k8s.io/apimachinery/pkg/api/errors"
	appsv1 "open-cluster-management.io/multicloud-operators-subscription/pkg/apis/apps/v1"
	appsv1alpha1 "open-cluster-management.io/multicloud-operators-subscription/pkg/apis/apps/v1alpha1"
	ctrl "sigs.k8s.io/controller-runtime"
	"sigs.k8s.io/controller-runtime/pkg/client"

	"github.com/stolostron/multicluster-globalhub/manager/pkg/specsyncer/db2transport/db"
)

func AddSubscriptionStatusStatusDBSyncer(mgr ctrl.Manager, database db.DB,
	statusSyncInterval time.Duration,
) error {
	err := mgr.Add(&genericDBSyncer{
		statusSyncInterval: statusSyncInterval,
		statusSyncFunc: func(ctx context.Context) {
			syncSubscriptionStatuses(ctx,
				ctrl.Log.WithName("subscription-statuses-db-syncer"),
				database,
				mgr.GetClient())
		},
	})
	if err != nil {
		return fmt.Errorf("failed to add subscription statuses syncer to the manager: %w", err)
	}

	return nil
}

func syncSubscriptionStatuses(ctx context.Context, log logr.Logger, database db.DB,
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

		go handleSubscriptionStatus(ctx, log, database, k8sClient, uid, name, namespace)
	}
}

func handleSubscriptionStatus(ctx context.Context, log logr.Logger, database db.DB,
	k8sClient client.Client, specSubscriptionUID string, subscriptionName string, subscriptionNamespace string,
) {
	subscriptionStatus, err := getAggregatedSubscriptionStatuses(ctx, database, subscriptionName,
		subscriptionNamespace)
	if err != nil {
		log.Error(err, "failed to get aggregated subscription-status", "name", subscriptionName,
			"namespace", subscriptionNamespace)

		return
	}

	if subscriptionStatus == nil { // no status resources found in DB
		return
	}

	// set owner-reference so that the subscription-status is deleted when the subscription is
	setOwnerReference(subscriptionStatus, createOwnerReference(appsv1APIGroup, subscriptionKind, subscriptionName,
		specSubscriptionUID))

	if err := updateSubscriptionStatus(ctx, k8sClient, subscriptionStatus); err != nil {
		log.Error(err, "failed to update subscription-status status")
	}
}

// returns aggregated SubscriptionStatus and error.
func getAggregatedSubscriptionStatuses(ctx context.Context, database db.DB,
	subscriptionName string, subscriptionNamespace string,
) (*appsv1alpha1.SubscriptionStatus, error) {
	rows, err := database.GetConn().Query(ctx,
		fmt.Sprintf(`SELECT payload FROM status.%s
			WHERE payload->'metadata'->>'name'=$1 AND payload->'metadata'->>'namespace'=$2`,
			subscriptionStatusesTableName), subscriptionName, subscriptionNamespace)
	if err != nil {
		return nil, fmt.Errorf("error in getting subscription-statuses from DB - %w", err)
	}

	defer rows.Close()

	var subscriptionStatus *appsv1alpha1.SubscriptionStatus

	for rows.Next() {
		var leafHubSubscriptionStatus appsv1alpha1.SubscriptionStatus

		if err := rows.Scan(&leafHubSubscriptionStatus); err != nil {
			return nil, fmt.Errorf("error getting subscription-status from DB - %w", err)
		}

		if subscriptionStatus == nil {
			subscriptionStatus = cleanSubscriptionStatusObject(leafHubSubscriptionStatus)
			continue
		}

		// assuming that cluster names are unique across the hubs, all we need to do is a complete merge
		subscriptionStatus.Statuses.SubscriptionStatus = append(
			subscriptionStatus.Statuses.SubscriptionStatus,
			leafHubSubscriptionStatus.Statuses.SubscriptionStatus...)
	}

	return subscriptionStatus, nil
}

func updateSubscriptionStatus(ctx context.Context, k8sClient client.Client,
	aggregatedSubscriptionStatus *appsv1alpha1.SubscriptionStatus,
) error {
	deployedSubscriptionStatus := &appsv1alpha1.SubscriptionStatus{}

	err := k8sClient.Get(ctx, client.ObjectKey{
		Name:      aggregatedSubscriptionStatus.Name,
		Namespace: aggregatedSubscriptionStatus.Namespace,
	}, deployedSubscriptionStatus)
	if err != nil {
		if errors.IsNotFound(err) { // create CR
			if err := createK8sResource(ctx, k8sClient, aggregatedSubscriptionStatus); err != nil {
				return fmt.Errorf("failed to create subscription-status {name=%s, namespace=%s} - %w",
					aggregatedSubscriptionStatus.Name, aggregatedSubscriptionStatus.Namespace, err)
			}

			return nil
		}

		return fmt.Errorf("failed to get subscription-status {name=%s, namespace=%s} - %w",
			aggregatedSubscriptionStatus.Name, aggregatedSubscriptionStatus.Namespace, err)
	}

	// if object exists, clone and update
	originalSubscriptionStatus := deployedSubscriptionStatus.DeepCopy()

	deployedSubscriptionStatus.Statuses = aggregatedSubscriptionStatus.Statuses

	err = k8sClient.Patch(ctx, deployedSubscriptionStatus,
		client.MergeFrom(originalSubscriptionStatus))
	if err != nil && !errors.IsNotFound(err) {
		return fmt.Errorf("failed to update subscription-status CR (name=%s, namespace=%s): %w",
			deployedSubscriptionStatus.Name, deployedSubscriptionStatus.Namespace, err)
	}

	return nil
}

func cleanSubscriptionStatusObject(subscriptionStatus appsv1alpha1.SubscriptionStatus,
) *appsv1alpha1.SubscriptionStatus {
	clone := subscriptionStatus.DeepCopy()
	// assign annotations
	clone.Annotations = map[string]string{}
	// assign labels
	clone.Labels = map[string]string{}
	clone.Labels[appsv1.AnnotationHosting] = fmt.Sprintf("%s.%s",
		subscriptionStatus.Namespace, subscriptionStatus.Name)

	return clone
}
