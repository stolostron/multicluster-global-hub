package dbsyncer

import (
	"context"
	"fmt"
	"time"

	"github.com/stolostron/hub-of-hubs/manager/pkg/specsyncer/db2transport/bundle"
	"github.com/stolostron/hub-of-hubs/manager/pkg/specsyncer/db2transport/db"
	"github.com/stolostron/hub-of-hubs/manager/pkg/specsyncer/db2transport/intervalpolicy"
	"github.com/stolostron/hub-of-hubs/manager/pkg/specsyncer/db2transport/transport"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	placementrulev1 "open-cluster-management.io/multicloud-operators-subscription/pkg/apis/apps/placementrule/v1"
	ctrl "sigs.k8s.io/controller-runtime"
)

const (
	placementRulesTableName = "placementrules"
	placementRulesMsgKey    = "PlacementRules"
)

// AddPlacementRulesDBToTransportSyncer adds placement rules db to transport syncer to the manager.
func AddPlacementRulesDBToTransportSyncer(mgr ctrl.Manager, specDB db.SpecDB, transportObj transport.Transport,
	specSyncInterval time.Duration,
) error {
	createObjFunc := func() metav1.Object { return &placementrulev1.PlacementRule{} }
	lastSyncTimestampPtr := &time.Time{}

	if err := mgr.Add(&genericDBToTransportSyncer{
		log:            ctrl.Log.WithName("placement-rules-db-to-transport-syncer"),
		intervalPolicy: intervalpolicy.NewExponentialBackoffPolicy(specSyncInterval),
		syncBundleFunc: func(ctx context.Context) (bool, error) {
			return syncObjectsBundle(ctx, transportObj, placementRulesMsgKey, specDB, placementRulesTableName,
				createObjFunc, bundle.NewBaseObjectsBundle, lastSyncTimestampPtr)
		},
	}); err != nil {
		return fmt.Errorf("failed to add placement rules db to transport syncer - %w", err)
	}

	return nil
}
