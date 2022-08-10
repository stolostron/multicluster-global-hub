// Copyright (c) 2021 Red Hat, Inc.
// Copyright Contributors to the Open Cluster Management project

package dbsyncer

import (
	"context"
	"fmt"
	"time"

	"github.com/go-logr/logr"
	"k8s.io/apimachinery/pkg/api/errors"
	placementrulev1 "open-cluster-management.io/multicloud-operators-subscription/pkg/apis/apps/placementrule/v1"
	ctrl "sigs.k8s.io/controller-runtime"
	"sigs.k8s.io/controller-runtime/pkg/client"

	"github.com/stolostron/hub-of-hubs/manager/pkg/specsyncer/db2transport/db"
)

const (
	placementRulesSpecTableName    = "placementrules"
	placementrRulesStatusTableName = "placementrules"
)

func AddPlacementRuleStatusDBSyncer(mgr ctrl.Manager, database db.DB,
	statusSyncInterval time.Duration,
) error {
	err := mgr.Add(&genericDBSyncer{
		statusSyncInterval: statusSyncInterval,
		statusSyncFunc: func(ctx context.Context) {
			syncPlacementRules(ctx,
				ctrl.Log.WithName("placementrule-db-syncer"),
				database,
				mgr.GetClient())
		},
	})
	if err != nil {
		return fmt.Errorf("failed to add placementrules syncer to the manager: %w", err)
	}

	return nil
}

func syncPlacementRules(ctx context.Context, log logr.Logger, database db.DB,
	k8sClient client.Client,
) {
	rows, err := database.GetConn().Query(ctx,
		fmt.Sprintf(`SELECT payload->'metadata'->>'name', payload->'metadata'->>'namespace' 
		FROM spec.%s WHERE deleted = FALSE`, placementRulesSpecTableName))
	if err != nil {
		log.Error(err, "error in getting placementrule spec")
		return
	}

	for rows.Next() {
		var name, namespace string

		err := rows.Scan(&name, &namespace)
		if err != nil {
			log.Error(err, "error in select", "table", placementRulesSpecTableName)
			continue
		}

		go handlePlacementRuleStatus(ctx, log, database, k8sClient, name, namespace)
	}
}

func handlePlacementRuleStatus(ctx context.Context, log logr.Logger, database db.DB,
	k8sClient client.Client, placementRuleName string, placementRuleNamespace string,
) {
	placementRuleStatus, statusEntriesFound, err := getPlacementRuleStatus(ctx, database,
		placementRuleName, placementRuleNamespace)
	if err != nil {
		log.Error(err, "failed to get placementrule status", "name", placementRuleName,
			"namespace", placementRuleNamespace)

		return
	}

	if !statusEntriesFound { // no status resources found in DB - placementrule is never created here
		return
	}

	if err := updatePlacementRuleStatus(ctx, k8sClient, placementRuleName, placementRuleNamespace,
		placementRuleStatus); err != nil {
		log.Error(err, "failed to update placementrule status")
	}
}

// returns aggregated PlacementRuleStatus.
func getPlacementRuleStatus(ctx context.Context, database db.DB,
	placementRuleName string, placementRuleNamespace string,
) (*placementrulev1.PlacementRuleStatus, bool, error) {
	rows, err := database.GetConn().Query(ctx,
		fmt.Sprintf(`SELECT payload FROM status.%s
			WHERE payload->'metadata'->>'name'=$1 AND payload->'metadata'->>'namespace'=$2`,
			placementrRulesStatusTableName), placementRuleName, placementRuleNamespace)
	if err != nil {
		return nil, false, fmt.Errorf("error in getting placementrules from DB - %w", err)
	}

	defer rows.Close()

	// build an aggregated placement-rule
	placementRuleStatus := &placementrulev1.PlacementRuleStatus{}
	statusEntriesFound := false

	for rows.Next() {
		var leafHubPlacementRule placementrulev1.PlacementRule

		if err := rows.Scan(&leafHubPlacementRule); err != nil {
			return nil, false, fmt.Errorf("error getting placementrule from DB - %w", err)
		}

		statusEntriesFound = true

		// assuming that cluster names are unique across the hubs, all we need to do is a complete merge
		placementRuleStatus.Decisions = append(placementRuleStatus.Decisions,
			leafHubPlacementRule.Status.Decisions...)
	}

	return placementRuleStatus, statusEntriesFound, nil
}

func updatePlacementRuleStatus(ctx context.Context, k8sClient client.Client, placementRuleName string,
	placementRuleNamespace string, placementRuleStatus *placementrulev1.PlacementRuleStatus,
) error {
	deployedPlacementRule := &placementrulev1.PlacementRule{}

	err := k8sClient.Get(ctx, client.ObjectKey{
		Name:      placementRuleName,
		Namespace: placementRuleNamespace,
	}, deployedPlacementRule)
	if err != nil {
		if errors.IsNotFound(err) { // CR getting deleted
			return nil
		}

		return fmt.Errorf("failed to get placementrule {name=%s, namespace=%s} - %w",
			placementRuleName, placementRuleNamespace, err)
	}

	// if object exists, clone and update
	originalPlacementRule := deployedPlacementRule.DeepCopy()

	deployedPlacementRule.Status = *placementRuleStatus

	err = k8sClient.Status().Patch(ctx, deployedPlacementRule,
		client.MergeFrom(originalPlacementRule))
	if err != nil && !errors.IsNotFound(err) {
		return fmt.Errorf("failed to update placementrule CR (name=%s, namespace=%s): %w",
			deployedPlacementRule.Name, deployedPlacementRule.Namespace, err)
	}

	return nil
}
