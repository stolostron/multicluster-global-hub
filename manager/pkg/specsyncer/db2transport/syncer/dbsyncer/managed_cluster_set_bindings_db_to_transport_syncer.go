package dbsyncer

import (
	"context"
	"fmt"
	"time"

	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	clusterv1beta1 "open-cluster-management.io/api/cluster/v1beta1"
	ctrl "sigs.k8s.io/controller-runtime"

	"github.com/stolostron/multicluster-global-hub/manager/pkg/specsyncer/db2transport/bundle"
	"github.com/stolostron/multicluster-global-hub/manager/pkg/specsyncer/db2transport/db"
	"github.com/stolostron/multicluster-global-hub/manager/pkg/specsyncer/db2transport/intervalpolicy"
	"github.com/stolostron/multicluster-global-hub/manager/pkg/specsyncer/db2transport/transport"
)

const (
	managedClusterSetBindingsTableName = "managedclustersetbindings"
	managedClusterSetBindingsMsgKey    = "ManagedClusterSetBindings"
)

// AddManagedClusterSetBindingsDBToTransportSyncer adds managed-cluster-set-bindings db to transport syncer to the
// manager.
func AddManagedClusterSetBindingsDBToTransportSyncer(mgr ctrl.Manager, specDB db.SpecDB,
	transportObj transport.Transport, specSyncInterval time.Duration,
) error {
	createObjFunc := func() metav1.Object {
		return &clusterv1beta1.ManagedClusterSetBinding{}
	}
	lastSyncTimestampPtr := &time.Time{}

	if err := mgr.Add(&genericDBToTransportSyncer{
		log:            ctrl.Log.WithName("managed-cluster-set-bindings-db-to-transport-syncer"),
		intervalPolicy: intervalpolicy.NewExponentialBackoffPolicy(specSyncInterval),
		syncBundleFunc: func(ctx context.Context) (bool, error) {
			return syncObjectsBundle(ctx, transportObj, managedClusterSetBindingsMsgKey, specDB,
				managedClusterSetBindingsTableName, createObjFunc, bundle.NewBaseObjectsBundle, lastSyncTimestampPtr)
		},
	}); err != nil {
		return fmt.Errorf("failed to add managed-cluster-set-bindings db to transport syncer - %w", err)
	}

	return nil
}
