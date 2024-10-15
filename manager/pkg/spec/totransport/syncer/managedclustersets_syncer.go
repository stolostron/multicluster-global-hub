package syncer

import (
	"context"
	"fmt"
	"time"

	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	clusterv1beta2 "open-cluster-management.io/api/cluster/v1beta2"
	ctrl "sigs.k8s.io/controller-runtime"

	"github.com/stolostron/multicluster-global-hub/manager/pkg/spec/controller/bundle"
	"github.com/stolostron/multicluster-global-hub/manager/pkg/spec/specdb"
	"github.com/stolostron/multicluster-global-hub/manager/pkg/spec/totransport/interval"
	"github.com/stolostron/multicluster-global-hub/pkg/transport"
)

const (
	managedClusterSetsTableName = "managedclustersets"
	managedClusterSetsMsgKey    = "ManagedClusterSets"
)

// AddManagedClusterSetsDBToTransportSyncer adds managed-cluster-sets db to transport syncer to the manager.
func AddManagedClusterSetsDBToTransportSyncer(mgr ctrl.Manager, specDB specdb.SpecDB, producer transport.Producer,
	specSyncInterval time.Duration,
) error {
	createObjFunc := func() metav1.Object { return &clusterv1beta2.ManagedClusterSet{} }
	lastSyncTimestampPtr := &time.Time{}

	if err := mgr.Add(&genericDBToTransportSyncer{
		log:            ctrl.Log.WithName("db-to-transport-syncer-managedclusterset"),
		intervalPolicy: interval.NewExponentialBackoffPolicy(specSyncInterval),
		syncBundleFunc: func(ctx context.Context) (bool, error) {
			return syncObjectsBundle(ctx, producer, managedClusterSetsMsgKey, specDB, managedClusterSetsTableName,
				createObjFunc, bundle.NewBaseObjectsBundle, lastSyncTimestampPtr)
		},
	}); err != nil {
		return fmt.Errorf("failed to add managed-cluster-sets db to transport syncer - %w", err)
	}

	return nil
}
