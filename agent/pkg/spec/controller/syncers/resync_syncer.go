package syncers

import (
	"context"
	"encoding/json"

	"github.com/go-logr/logr"
	"k8s.io/klog"
	ctrl "sigs.k8s.io/controller-runtime"

	"github.com/stolostron/multicluster-global-hub/pkg/bundle/version"
)

var enabledResyncTypes map[string]*version.Version

// resyncSyncer resync the bundle info.
type resyncSyncer struct {
	log logr.Logger
}

func NewResyncSyncer() *resyncSyncer {
	return &resyncSyncer{
		log: ctrl.Log.WithName("Resync status syncer"),
	}
}

func (syncer *resyncSyncer) Sync(ctx context.Context, payload []byte) error {
	syncer.log.Info("resync resource")
	eventTypes := []string{}
	if err := json.Unmarshal(payload, &eventTypes); err != nil {
		syncer.log.Error(err, "failed to unmarshal bundle keys")
		return err
	}

	for _, eventType := range eventTypes {
		syncer.log.Info("Resync event", "key", eventType)
		if enabledResyncTypes == nil {
			syncer.log.Info("not support to resync any type of resources")
			return nil
		}
		resyncVersion, ok := enabledResyncTypes[eventType]
		if !ok {
			syncer.log.Info("not support to resync the current resource type", "event key", eventType)
			return nil
		}
		resyncVersion.Incr()
	}
	return nil
}

func EnableResyc(evtType string, syncVersion *version.Version) {
	if enabledResyncTypes == nil {
		enabledResyncTypes = make(map[string]*version.Version)
	}
	enabledResyncTypes[evtType] = syncVersion
	klog.Info("support to resync type: ", evtType)
}

func GetEventVersion(evtType string) *version.Version {
	return enabledResyncTypes[evtType]
}
