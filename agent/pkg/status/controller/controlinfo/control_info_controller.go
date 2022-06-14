package controlinfo

import (
	"context"
	"encoding/json"
	"fmt"
	"time"

	"github.com/go-logr/logr"
	ctrl "sigs.k8s.io/controller-runtime"

	"github.com/stolostron/hub-of-hubs/agent/pkg/status/bundle"
	"github.com/stolostron/hub-of-hubs/agent/pkg/status/bundle/controlinfo"
	"github.com/stolostron/hub-of-hubs/agent/pkg/status/controller/syncintervals"
	"github.com/stolostron/hub-of-hubs/agent/pkg/transport/producer"
	configv1 "github.com/stolostron/hub-of-hubs/pkg/apis/config/v1"
	"github.com/stolostron/hub-of-hubs/pkg/constants"
)

const (
	controlInfoLogName = "control-info"
)

// LeafHubControlInfoController manages control info bundle traffic.
type LeafHubControlInfoController struct {
	log                     logr.Logger
	bundle                  bundle.Bundle
	transportBundleKey      string
	transport               producer.Producer
	resolveSyncIntervalFunc syncintervals.ResolveSyncIntervalFunc
}

// AddControlInfoController creates a new instance of control info controller and adds it to the manager.
func AddControlInfoController(mgr ctrl.Manager, transport producer.Producer, leafHubName string, incarnation uint64,
	_ *configv1.Config, syncIntervalsData *syncintervals.SyncIntervals,
) error {
	transportBundleKey := fmt.Sprintf("%s.%s", leafHubName, constants.ControlInfoMsgKey)

	controlInfoCtrl := &LeafHubControlInfoController{
		log:                     ctrl.Log.WithName(controlInfoLogName),
		bundle:                  controlinfo.NewBundle(leafHubName, incarnation),
		transportBundleKey:      transportBundleKey,
		transport:               transport,
		resolveSyncIntervalFunc: syncIntervalsData.GetControlInfo,
	}

	if err := mgr.Add(controlInfoCtrl); err != nil {
		return fmt.Errorf("failed to add controller to the manager - %w", err)
	}

	return nil
}

// Start function starts control info controller.
func (c *LeafHubControlInfoController) Start(ctx context.Context) error {
	c.log.Info("Starting Controller")

	go c.periodicSync(ctx)

	<-ctx.Done() // blocking wait for stop event
	c.log.Info("Stopping Controller")

	return nil
}

func (c *LeafHubControlInfoController) periodicSync(ctx context.Context) {
	currentSyncInterval := c.resolveSyncIntervalFunc()
	ticker := time.NewTicker(currentSyncInterval)

	for {
		select {
		case <-ctx.Done():
			ticker.Stop()
			return

		case <-ticker.C: // wait for next time interval
			c.syncBundle()

			resolvedInterval := c.resolveSyncIntervalFunc()

			// reset ticker if sync interval has changed
			if resolvedInterval != currentSyncInterval {
				currentSyncInterval = resolvedInterval
				ticker.Reset(currentSyncInterval)
				c.log.Info(fmt.Sprintf("sync interval has been reset to %s", currentSyncInterval.String()))
			}
		}
	}
}

func (c *LeafHubControlInfoController) syncBundle() {
	c.bundle.UpdateObject(nil) // increase generation

	payloadBytes, err := json.Marshal(c.bundle)
	if err != nil {
		c.log.Error(fmt.Errorf("sync object from type %s with id %s - %w", constants.StatusBundle, c.transportBundleKey, err), "failed to sync bundle")
	}

	transportMessageKey := c.transportBundleKey
	if deltaStateBundle, ok := c.bundle.(bundle.DeltaStateBundle); ok {
		transportMessageKey = fmt.Sprintf("%s@%d", c.transportBundleKey, deltaStateBundle.GetTransportationID())
	}

	c.transport.SendAsync(&producer.Message{
		Key:     transportMessageKey,
		ID:      c.transportBundleKey,
		MsgType: constants.StatusBundle,
		Version: c.bundle.GetBundleVersion().String(),
		Payload: payloadBytes,
	})
}
