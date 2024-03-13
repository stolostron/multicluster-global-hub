package syncers

import (
	"encoding/json"

	"github.com/go-logr/logr"
	"k8s.io/klog"
	ctrl "sigs.k8s.io/controller-runtime"

	"github.com/stolostron/multicluster-global-hub/pkg/bundle/metadata"
)

var supportedResyncTypes map[string]*metadata.BundleVersion

// resyncSyncer resync the bundle info.
type resyncSyncer struct {
	log logr.Logger
}

func NewResyncSyncer() *resyncSyncer {
	return &resyncSyncer{
		log: ctrl.Log.WithName("Resync status syncer"),
	}
}

func (syncer *resyncSyncer) Sync(payload []byte) error {
	syncer.log.Info("resync resource")
	eventTypes := []string{}
	if err := json.Unmarshal(payload, &eventTypes); err != nil {
		syncer.log.Error(err, "failed to unmarshal bundle keys")
		return err
	}

	for _, eventType := range eventTypes {
		syncer.log.Info("Resync event", "key", eventType)
		if supportedResyncTypes == nil {
			syncer.log.Info("not support to resync any type of resources")
			return nil
		}
		resyncVersion, ok := supportedResyncTypes[eventType]
		if !ok {
			syncer.log.Info("not support to resync the current resource type", "event key", eventType)
			return nil
		}
		resyncVersion.Incr()
	}
	return nil
}

func SupportResyc(evtType string, syncVersion *metadata.BundleVersion) {
	if supportedResyncTypes == nil {
		supportedResyncTypes = make(map[string]*metadata.BundleVersion)
	}
	supportedResyncTypes[evtType] = syncVersion
	klog.Info("support to resync type: ", evtType)
}
