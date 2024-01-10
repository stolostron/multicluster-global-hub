package specsyncer

import (
	"context"
	"encoding/json"
	"fmt"

	ctrl "sigs.k8s.io/controller-runtime"

	"github.com/stolostron/multicluster-global-hub/manager/pkg/config"
	specsyncer "github.com/stolostron/multicluster-global-hub/manager/pkg/specsyncer/db2transport/syncer"
	"github.com/stolostron/multicluster-global-hub/manager/pkg/specsyncer/spec2db"
	"github.com/stolostron/multicluster-global-hub/manager/pkg/specsyncer/spec2db/controller"
	"github.com/stolostron/multicluster-global-hub/pkg/constants"
	"github.com/stolostron/multicluster-global-hub/pkg/transport"
)

func AddGlobalResourceSpecSyncers(mgr ctrl.Manager,
	managerConfig *config.ManagerConfig,
	producer transport.Producer) error {
	if err := spec2db.AddSpec2DBControllers(mgr); err != nil {
		return fmt.Errorf("failed to add spec-to-db controllers: %w", err)
	}

	if err := specsyncer.AddDB2TransportSyncers(mgr, managerConfig, producer); err != nil {
		return fmt.Errorf("failed to add db-to-transport syncers: %w", err)
	}

	if err := specsyncer.AddManagedClusterLabelSyncer(mgr,
		managerConfig.SyncerConfig.DeletedLabelsTrimmingInterval); err != nil {
		return fmt.Errorf("failed to add status db watchers: %w", err)
	}

	return nil
}

func AddBasicSpecSyncers(mgr ctrl.Manager) error {
	return controller.AddManagedHubController(mgr)
}

// SendSyncAllMsgInfo send a constants.ResyncMsgKey bundle in manager start.
// When agent get the bundle, it will resend the "resendMsgKeys" bundles to transport
// It is mainly used to handle data lost in upgade scenario.
func SendSyncAllMsgInfo(producer transport.Producer) error {
	ctx := context.Background()
	resendMsgKeys := []string{constants.HubClusterInfoMsgKey}
	payloadBytes, err := json.Marshal(resendMsgKeys)
	if err != nil {
		return err
	}
	if err := producer.Send(ctx, &transport.Message{
		Key:         constants.ResyncMsgKey,
		Destination: transport.Broadcast,
		MsgType:     constants.SpecBundle,
		Payload:     payloadBytes,
	}); err != nil {
		return fmt.Errorf("Failed to resend resendbundle")
	}
	return nil
}
