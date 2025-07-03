package syncers

import (
	"context"
	"encoding/json"

	cloudevents "github.com/cloudevents/sdk-go/v2"
	"go.uber.org/zap"

	"github.com/stolostron/multicluster-global-hub/agent/pkg/configs"
	"github.com/stolostron/multicluster-global-hub/pkg/bundle/version"
	"github.com/stolostron/multicluster-global-hub/pkg/enum"
	"github.com/stolostron/multicluster-global-hub/pkg/logger"
)

var registeredResyncTypes map[string]*version.Version

// resyncer resync the bundle info.
type resyncer struct {
	log *zap.SugaredLogger
}

func NewResyncer() *resyncer {
	return &resyncer{
		log: logger.ZapLogger("status-resyncer"),
	}
}

func (s *resyncer) Sync(ctx context.Context, evt *cloudevents.Event) error {
	payload := evt.Data()
	if registeredResyncTypes == nil {
		s.log.Warn("not register any resource for resync")
		return nil
	}

	eventTypes := []string{}
	if err := json.Unmarshal(payload, &eventTypes); err != nil {
		s.log.Error(err, "failed to unmarshal bundle keys")
		return err
	}

	for _, eventType := range eventTypes {
		s.log.Infow("resyncing event type", "eventType", enum.ShortenEventType(eventType))
		configs.GlobalResyncQueue.Add(eventType)
		// deprecated
		resyncVersion, ok := registeredResyncTypes[eventType]
		if !ok {
			s.log.Infof("event type %s is not registered for resync", eventType)
			continue
		}
		resyncVersion.Incr()
	}
	return nil
}

func EnableResync(evtType string, syncVersion *version.Version) {
	if registeredResyncTypes == nil {
		registeredResyncTypes = make(map[string]*version.Version)
	}
	registeredResyncTypes[evtType] = syncVersion
}

func GetEventVersion(evtType string) *version.Version {
	return registeredResyncTypes[evtType]
}
