// Copyright (c) 2021 Red Hat, Inc.
// Copyright Contributors to the Open Cluster Management project

package dbsyncer

import (
	"context"
	"fmt"
	"time"

	"github.com/go-logr/logr"
	"k8s.io/apimachinery/pkg/api/errors"
	clusterv1beta1 "open-cluster-management.io/api/cluster/v1beta1"
	ctrl "sigs.k8s.io/controller-runtime"
	"sigs.k8s.io/controller-runtime/pkg/client"

	"github.com/stolostron/multicluster-global-hub/manager/pkg/specsyncer/db2transport/db"
)

func AddPlacementStatusDBSyncer(mgr ctrl.Manager, database db.DB,
	statusSyncInterval time.Duration,
) error {
	err := mgr.Add(&genericDBSyncer{
		statusSyncInterval: statusSyncInterval,
		statusSyncFunc: func(ctx context.Context) {
			syncPlacements(ctx,
				ctrl.Log.WithName("placement-db-syncer"),
				database,
				mgr.GetClient())
		},
	})
	if err != nil {
		return fmt.Errorf("failed to add placements syncer to the manager: %w", err)
	}

	return nil
}

func syncPlacements(ctx context.Context, log logr.Logger, database db.DB,
	k8sClient client.Client,
) {
	rows, err := database.GetConn().Query(ctx,
		fmt.Sprintf(`SELECT payload->'metadata'->>'name', payload->'metadata'->>'namespace' 
		FROM spec.%s WHERE deleted = FALSE`, placementsSpecTableName))
	if err != nil {
		log.Error(err, "error in getting placement spec")
		return
	}

	for rows.Next() {
		var name, namespace string

		err := rows.Scan(&name, &namespace)
		if err != nil {
			log.Error(err, "error in select", "table", placementsSpecTableName)
			continue
		}

		go handlePlacementStatus(ctx, log, database, k8sClient, name, namespace)
	}
}

func handlePlacementStatus(ctx context.Context, log logr.Logger, database db.DB,
	k8sClient client.Client, placementName string, placementNamespace string,
) {
	placementStatus, statusEntriesFound, err := getPlacementStatus(ctx, database,
		placementName, placementNamespace)
	if err != nil {
		log.Error(err, "failed to get aggregated placement", "name", placementName, "namespace", placementNamespace)
		return
	}

	if !statusEntriesFound { // no status resources found in DB - placement is never created here
		return
	}

	if err := updatePlacementStatus(ctx, k8sClient, placementName, placementNamespace, placementStatus); err != nil {
		log.Error(err, "failed to update placement status")
	}
}

// returns aggregated PlacementStatus and error.
func getPlacementStatus(ctx context.Context, database db.DB,
	placementName string, placementNamespace string,
) (*clusterv1beta1.PlacementStatus, bool, error) {
	rows, err := database.GetConn().Query(ctx,
		fmt.Sprintf(`SELECT payload FROM status.%s
			WHERE payload->'metadata'->>'name'=$1 AND payload->'metadata'->>'namespace'=$2`,
			placementsStatusTableName), placementName, placementNamespace)
	if err != nil {
		return nil, false, fmt.Errorf("error in getting placements from DB - %w", err)
	}

	defer rows.Close()

	// build an aggregated placement
	placementStatus := clusterv1beta1.PlacementStatus{}
	statusEntriesFound := false

	for rows.Next() {
		var leafHubPlacement clusterv1beta1.Placement

		if err := rows.Scan(&leafHubPlacement); err != nil {
			return nil, false, fmt.Errorf("error getting placement from DB - %w", err)
		}

		statusEntriesFound = true

		// assuming that cluster names are unique across the hubs, all we need to do is a complete merge
		placementStatus.NumberOfSelectedClusters +=
			leafHubPlacement.Status.NumberOfSelectedClusters
	}

	return &placementStatus, statusEntriesFound, nil
}

func updatePlacementStatus(ctx context.Context, k8sClient client.Client,
	placementName string, placementNamespace string, placementStatus *clusterv1beta1.PlacementStatus,
) error {
	deployedPlacement := &clusterv1beta1.Placement{}

	err := k8sClient.Get(ctx, client.ObjectKey{
		Name:      placementName,
		Namespace: placementNamespace,
	}, deployedPlacement)
	if err != nil {
		if errors.IsNotFound(err) { // CR getting deleted
			return nil
		}

		return fmt.Errorf("failed to get placement {name=%s, namespace=%s} - %w",
			placementName, placementNamespace, err)
	}

	// if object exists, clone and update
	originalPlacement := deployedPlacement.DeepCopy()

	deployedPlacement.Status.NumberOfSelectedClusters =
		placementStatus.NumberOfSelectedClusters

	err = k8sClient.Status().Patch(ctx, deployedPlacement, client.MergeFrom(originalPlacement))
	if err != nil && !errors.IsNotFound(err) {
		return fmt.Errorf("failed to update placement CR (name=%s, namespace=%s): %w",
			deployedPlacement.Name, deployedPlacement.Namespace, err)
	}

	return nil
}
