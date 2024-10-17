package syncers

import (
	"context"
	"fmt"
	"time"

	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	clusterv1beta2 "open-cluster-management.io/api/cluster/v1beta2"
	ctrl "sigs.k8s.io/controller-runtime"

	"github.com/stolostron/multicluster-global-hub/manager/pkg/spec/controllers/bundle"
	"github.com/stolostron/multicluster-global-hub/manager/pkg/spec/specdb"
	"github.com/stolostron/multicluster-global-hub/manager/pkg/spec/syncers/interval"
	"github.com/stolostron/multicluster-global-hub/pkg/transport"
)

const (
	managedClusterSetBindingsTableName = "managedclustersetbindings"
	managedClusterSetBindingsMsgKey    = "ManagedClusterSetBindings"
)

// AddManagedClusterSetBindingsDBToTransportSyncer adds managed-cluster-set-bindings db to transport syncer to the
// manager.
func AddManagedClusterSetBindingsDBToTransportSyncer(mgr ctrl.Manager, specDB specdb.SpecDB,
	producer transport.Producer, specSyncInterval time.Duration,
) error {
	createObjFunc := func() metav1.Object {
		return &clusterv1beta2.ManagedClusterSetBinding{}
	}
	lastSyncTimestampPtr := &time.Time{}

	if err := mgr.Add(&genericDBToTransportSyncer{
		log:            ctrl.Log.WithName("db-to-transport-syncer-managedclustersetbinding"),
		intervalPolicy: interval.NewExponentialBackoffPolicy(specSyncInterval),
		syncBundleFunc: func(ctx context.Context) (bool, error) {
			return syncObjectsBundle(ctx, producer, managedClusterSetBindingsMsgKey, specDB,
				managedClusterSetBindingsTableName, createObjFunc, bundle.NewBaseObjectsBundle, lastSyncTimestampPtr)
		},
	}); err != nil {
		return fmt.Errorf("failed to add managed-cluster-set-bindings db to transport syncer - %w", err)
	}

	return nil
}
