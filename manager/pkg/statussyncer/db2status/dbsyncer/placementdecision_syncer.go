// Copyright (c) 2021 Red Hat, Inc.
// Copyright Contributors to the Open Cluster Management project

package dbsyncer

import (
	"context"
	"fmt"
	"time"

	"github.com/go-logr/logr"
	"github.com/stolostron/hub-of-hubs/manager/pkg/specsyncer/db2transport/db"
	"k8s.io/apimachinery/pkg/api/errors"
	clusterv1beta1 "open-cluster-management.io/api/cluster/v1beta1"
	ctrl "sigs.k8s.io/controller-runtime"
	"sigs.k8s.io/controller-runtime/pkg/client"
)

func AddPlacementDecisionDBSyncer(mgr ctrl.Manager, database db.DB,
	statusSyncInterval time.Duration,
) error {
	err := mgr.Add(&genericDBSyncer{
		statusSyncInterval: statusSyncInterval,
		statusSyncFunc: func(ctx context.Context) {
			syncPlacementDecisions(ctx,
				ctrl.Log.WithName("placement-decisions-db-syncer"),
				database,
				mgr.GetClient())
		},
	})
	if err != nil {
		return fmt.Errorf("failed to add placement-decisions syncer to the manager: %w", err)
	}

	return nil
}

func syncPlacementDecisions(ctx context.Context, log logr.Logger, database db.DB,
	k8sClient client.Client,
) {
	rows, err := database.GetConn().Query(ctx,
		fmt.Sprintf(`SELECT id, payload->'metadata'->>'name', payload->'metadata'->>'namespace' 
		FROM spec.%s WHERE deleted = FALSE`, placementsSpecTableName))
	if err != nil {
		log.Error(err, "error in getting placement spec")
		return
	}

	for rows.Next() {
		var uid, name, namespace string

		err := rows.Scan(&uid, &name, &namespace)
		if err != nil {
			log.Error(err, "error in select", "table", placementsSpecTableName)
			continue
		}

		go handlePlacementDecision(ctx, log, database, k8sClient, uid, name, namespace)
	}
}

func handlePlacementDecision(ctx context.Context, log logr.Logger, database db.DB,
	k8sClient client.Client, specPlacementUID string, placementName string, placementNamespace string,
) {
	placementDecision, err := getAggregatedPlacementDecisions(ctx, database, placementName,
		placementNamespace)
	if err != nil {
		log.Error(err, "failed to get aggregated placement-decision", "name", placementName,
			"namespace", placementNamespace)

		return
	}

	if placementDecision == nil { // no status resources found in DB - if deleted then k8s handles it by owner-reference
		return
	}

	// set owner-reference so that the placement-decision is deleted when the placement is
	setOwnerReference(placementDecision, createOwnerReference(clusterv1beta1APIGroup, placementKind, placementName,
		specPlacementUID))

	if err := updatePlacementDecision(ctx, k8sClient, placementDecision); err != nil {
		log.Error(err, "failed to update placement-decision")
	}
}

// returns aggregated PlacementDecision and error.
func getAggregatedPlacementDecisions(ctx context.Context, database db.DB,
	placementName string, placementNamespace string,
) (*clusterv1beta1.PlacementDecision, error) {
	rows, err := database.GetConn().Query(ctx,
		fmt.Sprintf(`SELECT payload FROM status.%s
			WHERE payload->'metadata'->>'name' like $1 AND payload->'metadata'->>'namespace'=$2`,
			placementDecisionsStatusTableName), fmt.Sprintf("%s%%", placementName), placementNamespace)
	if err != nil {
		return nil, fmt.Errorf("error in getting placement-decisions from DB - %w", err)
	}

	defer rows.Close()

	// build an aggregated placement
	var aggregatedPlacementDecision *clusterv1beta1.PlacementDecision

	for rows.Next() {
		var leafHubPlacementDecision clusterv1beta1.PlacementDecision

		if err := rows.Scan(&leafHubPlacementDecision); err != nil {
			return nil, fmt.Errorf("error getting placement-decision from DB - %w", err)
		}

		if leafHubPlacementDecision.Labels[clusterv1beta1.PlacementLabel] != placementName {
			continue // could be a PlacementDecision generated for a PlacementRule.
		}

		if aggregatedPlacementDecision == nil {
			aggregatedPlacementDecision = leafHubPlacementDecision.DeepCopy()
			continue
		}

		// assuming that cluster names are unique across the hubs, all we need to do is a complete merge
		aggregatedPlacementDecision.Status.Decisions = append(aggregatedPlacementDecision.Status.Decisions,
			leafHubPlacementDecision.Status.Decisions...)
	}

	return aggregatedPlacementDecision, nil
}

func updatePlacementDecision(ctx context.Context, k8sClient client.Client,
	aggregatedPlacementDecision *clusterv1beta1.PlacementDecision,
) error {
	deployedPlacementDecision := &clusterv1beta1.PlacementDecision{}

	err := k8sClient.Get(ctx, client.ObjectKey{
		Name:      aggregatedPlacementDecision.Name,
		Namespace: aggregatedPlacementDecision.Namespace,
	}, deployedPlacementDecision)
	if err != nil {
		if errors.IsNotFound(err) {
			if err := createK8sResource(ctx, k8sClient, aggregatedPlacementDecision); err != nil {
				return fmt.Errorf("failed to create placement-decision {name=%s, namespace=%s} - %w",
					aggregatedPlacementDecision.Name, aggregatedPlacementDecision.Namespace, err)
			}

			return nil
		}

		return fmt.Errorf("failed to get placement-decision {name=%s, namespace=%s} - %w",
			aggregatedPlacementDecision.Name, aggregatedPlacementDecision.Namespace, err)
	}

	// if object exists, clone and update
	originalPlacementDecision := deployedPlacementDecision.DeepCopy()

	deployedPlacementDecision.Status.Decisions = aggregatedPlacementDecision.Status.Decisions

	err = k8sClient.Status().Patch(ctx, deployedPlacementDecision, client.MergeFrom(originalPlacementDecision))
	if err != nil && !errors.IsNotFound(err) {
		return fmt.Errorf("failed to update placement-decision CR (name=%s, namespace=%s): %w",
			deployedPlacementDecision.Name, deployedPlacementDecision.Namespace, err)
	}

	return nil
}
