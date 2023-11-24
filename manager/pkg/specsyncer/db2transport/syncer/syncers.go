package syncer

import (
	"context"
	"encoding/json"
	"fmt"
	"time"

	ctrl "sigs.k8s.io/controller-runtime"

	"github.com/stolostron/multicluster-global-hub/manager/pkg/config"
	"github.com/stolostron/multicluster-global-hub/manager/pkg/specsyncer/db2transport/db"
	"github.com/stolostron/multicluster-global-hub/manager/pkg/specsyncer/db2transport/db/gorm"
	"github.com/stolostron/multicluster-global-hub/manager/pkg/specsyncer/db2transport/syncer/dbsyncer"
	"github.com/stolostron/multicluster-global-hub/pkg/constants"
	"github.com/stolostron/multicluster-global-hub/pkg/transport"
	"github.com/stolostron/multicluster-global-hub/pkg/transport/producer"
)

// AddDB2TransportSyncers adds the controllers that send info from DB to transport layer to the Manager.
func AddDB2TransportSyncers(mgr ctrl.Manager, managerConfig *config.ManagerConfig) error {
	producer, err := producer.NewGenericProducer(managerConfig.TransportConfig)
	if err != nil {
		return fmt.Errorf("failed to init spec transport bridge: %w", err)
	}
	specSyncInterval := managerConfig.SyncerConfig.SpecSyncInterval

	addDBSyncerFunctions := []func(ctrl.Manager, db.SpecDB, transport.Producer, time.Duration) error{
		// dbsyncer.AddHoHConfigDBToTransportSyncer,
		dbsyncer.AddPoliciesDBToTransportSyncer,
		dbsyncer.AddPlacementRulesDBToTransportSyncer,
		dbsyncer.AddPlacementBindingsDBToTransportSyncer,
		dbsyncer.AddApplicationsDBToTransportSyncer,
		dbsyncer.AddSubscriptionsDBToTransportSyncer,
		dbsyncer.AddChannelsDBToTransportSyncer,
		dbsyncer.AddManagedClusterLabelsDBToTransportSyncer,
		dbsyncer.AddPlacementsDBToTransportSyncer,
		dbsyncer.AddManagedClusterSetsDBToTransportSyncer,
		dbsyncer.AddManagedClusterSetBindingsDBToTransportSyncer,
	}
	specDB := gorm.NewGormSpecDB()
	for _, addDBSyncerFunction := range addDBSyncerFunctions {
		if err := addDBSyncerFunction(mgr, specDB, producer, specSyncInterval); err != nil {
			return fmt.Errorf("failed to add DB Syncer: %w", err)
		}
	}
	err = SendSyncAllMsgInfo(producer)
	return err
}

// AddManagedClusterLabelSyncer update the label table by the managed cluster table
func AddManagedClusterLabelSyncer(mgr ctrl.Manager, deletedLabelsTrimmingInterval time.Duration) error {
	if err := dbsyncer.AddManagedClusterLabelsSyncer(mgr, deletedLabelsTrimmingInterval); err != nil {
		return fmt.Errorf("failed to add status watcher: %w", err)
	}
	return nil
}

// SendSyncAllMsgInfo send a constants.ResendMsgKey bundle in manager start.
// When agent get the bundle, it will resend the "resendMsgKeys" bundles to transport
// It is mainly used to handle data lost in upgade scenario.
func SendSyncAllMsgInfo(producer transport.Producer) error {
	lastUpdateTimestamp := time.Now()
	const timeFormat = "2006-01-02_15-04-05.000000"
	ctx := context.Background()

	resendMsgKeys := []string{constants.HubClusterInfoMsgKey}
	payloadBytes, err := json.Marshal(resendMsgKeys)
	if err != nil {
		return err
	}
	if err := producer.Send(ctx, &transport.Message{
		Destination: transport.Broadcast,
		ID:          constants.ResendMsgKey,
		MsgType:     constants.SpecBundle,
		Version:     lastUpdateTimestamp.Format(timeFormat),
		Payload:     payloadBytes,
	}); err != nil {
		return fmt.Errorf("Failed to resend resendbundle")
	}
	return nil
}
