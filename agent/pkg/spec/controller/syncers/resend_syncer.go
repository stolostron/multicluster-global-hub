package syncers

import (
	"encoding/json"

	"github.com/go-logr/logr"
	"github.com/stolostron/multicluster-global-hub/agent/pkg/status/controller/cache"
	ctrl "sigs.k8s.io/controller-runtime"
)

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
	syncer.log.Info("Resync bundle info")
	bundleKeys := []string{}
	if err := json.Unmarshal(payload, &bundleKeys); err != nil {
		syncer.log.Error(err, "Failed to unmarshal bundle keys")
		return err
	}

	for _, bundleKey := range bundleKeys {
		syncer.log.Info("Resync bundle", "key", bundleKey)
		if cache.Cache == nil {
			syncer.log.Info("Cache is nil, do not need to resync info")
			return nil
		}
		_, ok := cache.Cache[bundleKey]
		if !ok {
			syncer.log.Info("Cache is nil, do not need to resync info", "bundle key", bundleKey)
			return nil
		}
		cache.Cache[bundleKey].GetVersion().Incr()
	}
	return nil
}
