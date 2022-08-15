// Copyright (c) 2020 Red Hat, Inc.
// Copyright Contributors to the Open Cluster Management project

package db2status

import (
	"fmt"
	"time"

	ctrl "sigs.k8s.io/controller-runtime"

	"github.com/stolostron/multicluster-globalhub/manager/pkg/specsyncer/db2transport/db"
	"github.com/stolostron/multicluster-globalhub/manager/pkg/statussyncer/db2status/dbsyncer"
)

// AddDBSyncers adds all the DBSyncers to the Manager.
func AddDBSyncers(mgr ctrl.Manager, database db.DB, statusSyncInterval time.Duration) error {
	addDBSyncerFunctions := []func(ctrl.Manager, db.DB, time.Duration) error{
		dbsyncer.AddPolicyDBSyncer,
		dbsyncer.AddPlacementRuleStatusDBSyncer,
		dbsyncer.AddPlacementStatusDBSyncer,
		dbsyncer.AddPlacementDecisionDBSyncer,
		dbsyncer.AddSubscriptionStatusStatusDBSyncer,
		dbsyncer.AddSubscriptionReportDBSyncer,
	}

	for _, addDBSyncerFunction := range addDBSyncerFunctions {
		if err := addDBSyncerFunction(mgr, database, statusSyncInterval); err != nil {
			return fmt.Errorf("failed to add DB Syncer: %w", err)
		}
	}

	return nil
}
