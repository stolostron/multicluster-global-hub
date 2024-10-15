package syncer

import (
	"context"
	"fmt"
	"time"

	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	policyv1 "open-cluster-management.io/governance-policy-propagator/api/v1"
	ctrl "sigs.k8s.io/controller-runtime"

	"github.com/stolostron/multicluster-global-hub/manager/pkg/spec/specdb"
	"github.com/stolostron/multicluster-global-hub/manager/pkg/spec/todatabase/bundle"
	"github.com/stolostron/multicluster-global-hub/manager/pkg/spec/totransport/interval"
	"github.com/stolostron/multicluster-global-hub/pkg/transport"
)

const (
	placementBindingsTableName = "placementbindings"
	placementBindingsMsgKey    = "PlacementBindings"
)

// AddPlacementBindingsDBToTransportSyncer adds placement bindings db to transport syncer to the manager.
func AddPlacementBindingsDBToTransportSyncer(mgr ctrl.Manager, specDB specdb.SpecDB, producer transport.Producer,
	specSyncInterval time.Duration,
) error {
	createObjFunc := func() metav1.Object { return &policyv1.PlacementBinding{} }
	lastSyncTimestampPtr := &time.Time{}

	if err := mgr.Add(&genericDBToTransportSyncer{
		log:            ctrl.Log.WithName("db-to-transport-syncer-placementrulebiding"),
		intervalPolicy: interval.NewExponentialBackoffPolicy(specSyncInterval),
		syncBundleFunc: func(ctx context.Context) (bool, error) {
			return syncObjectsBundle(ctx, producer, placementBindingsMsgKey, specDB, placementBindingsTableName,
				createObjFunc, bundle.NewBaseObjectsBundle, lastSyncTimestampPtr)
		},
	}); err != nil {
		return fmt.Errorf("failed to add placement bindings db to transport syncer - %w", err)
	}

	return nil
}
