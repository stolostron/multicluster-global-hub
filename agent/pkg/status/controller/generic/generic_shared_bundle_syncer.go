package generic

import (
	"context"
	"encoding/json"
	"fmt"
	"sync"
	"time"

	"github.com/go-logr/logr"
	ctrl "sigs.k8s.io/controller-runtime"

	"github.com/stolostron/multicluster-global-hub/pkg/bundle"
	"github.com/stolostron/multicluster-global-hub/pkg/constants"
	"github.com/stolostron/multicluster-global-hub/pkg/transport"
)

// proposed by this issue: https://github.com/stolostron/multicluster-global-hub/issues/726
type genericSharedBundleSyncer struct {
	log              logr.Logger
	bundleEntry      *SharedBundleEntry
	finalizerName    string
	producer         transport.Producer
	syncIntervalFunc func() time.Duration
	startOnce        sync.Once
	lock             *sync.Mutex
}

// NewGenericSharedBundleSyncer creates a new instance of genericStatusSyncController and adds it to the manager.
func NewGenericSharedBundleSyncer(mgr ctrl.Manager, producer transport.Producer, bundleEntry *SharedBundleEntry,
	objectCollection []bundle.SharedBundleObject, syncIntervalFunc func() time.Duration,
) error {
	statusSyncCtrl := &genericSharedBundleSyncer{
		log:              ctrl.Log.WithName(bundleEntry.transportBundleKey),
		bundleEntry:      bundleEntry,
		finalizerName:    constants.GlobalHubCleanupFinalizer,
		producer:         producer,
		syncIntervalFunc: syncIntervalFunc,
		lock:             &sync.Mutex{},
	}
	statusSyncCtrl.init()

	for _, handler := range objectCollection {
		err := newObjectHandler(mgr, producer, bundleEntry, handler, statusSyncCtrl.lock)
		if err != nil {
			return err
		}
	}
	return nil
}

func (c *genericSharedBundleSyncer) init() {
	c.startOnce.Do(func() {
		go c.periodicSync()
	})
}

func (c *genericSharedBundleSyncer) periodicSync() {
	currentSyncInterval := c.syncIntervalFunc()
	ticker := time.NewTicker(currentSyncInterval)

	for {
		<-ticker.C // wait for next time interval
		c.syncBundles()

		resolvedInterval := c.syncIntervalFunc()

		// reset ticker if sync interval has changed
		if resolvedInterval != currentSyncInterval {
			currentSyncInterval = resolvedInterval
			ticker.Reset(currentSyncInterval)
			c.log.Info(fmt.Sprintf("sync interval has been reset to %s", currentSyncInterval.String()))
		}
	}
}

func (c *genericSharedBundleSyncer) syncBundles() {
	c.lock.Lock() // make sure bundles are not updated if we're during bundles sync
	defer c.lock.Unlock()

	entry := c.bundleEntry
	// evaluate if bundle has to be sent only if predicate is true.
	if !entry.bundlePredicate() {
		return
	}
	bundleVersion := entry.bundle.GetVersion()

	// send to transport only if bundle has changed.
	if bundleVersion.NewerThan(&entry.lastSentBundleVersion) {
		payloadBytes, err := json.Marshal(entry.bundle)
		if err != nil {
			c.log.Error(err, "marshal entry.bundle error", "entry.bundleKey", entry.transportBundleKey)
			return
		}

		transportMessageKey := entry.transportBundleKey
		if deltaStateBundle, ok := entry.bundle.(bundle.AgentDeltaBundle); ok {
			transportMessageKey = fmt.Sprintf("%s@%d", entry.transportBundleKey, deltaStateBundle.GetTransportationID())
		}

		if err := c.producer.Send(context.TODO(), &transport.Message{
			Key:     transportMessageKey,
			MsgType: constants.StatusBundle,
			Version: entry.bundle.GetVersion().String(),
			Payload: payloadBytes,
		}); err != nil {
			c.log.Error(err, "send transport message error", "key", transportMessageKey)
			return
		}

		// 1. get into the next generation
		// 2. set the lastSentBundleVersion to first version of next generation
		entry.bundle.GetVersion().Next()
		entry.lastSentBundleVersion = *entry.bundle.GetVersion()
	}
}
