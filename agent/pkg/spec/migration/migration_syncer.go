package migration

import (
	"context"
	"encoding/json"

	cloudevents "github.com/cloudevents/sdk-go/v2"

	"github.com/stolostron/multicluster-global-hub/pkg/constants"
)

// MigrationSyncer is a wrapper that routes migration CloudEvents to the appropriate syncer.
// All migration events use a single event type (ManagedClusterMigrationType).
// Routing logic:
//   - source == "global-hub" + payload has "toHub" field → MigrationSourceSyncer (spec for source hub)
//   - source == "global-hub" + no "toHub" field          → MigrationTargetSyncer (spec for target hub)
//   - source != "global-hub"                             → MigrationTargetSyncer (resource deploying)
type MigrationSyncer struct {
	sourceSyncer *MigrationSourceSyncer
	targetSyncer *MigrationTargetSyncer
}

func NewMigrationSyncer(sourceSyncer *MigrationSourceSyncer, targetSyncer *MigrationTargetSyncer) *MigrationSyncer {
	return &MigrationSyncer{
		sourceSyncer: sourceSyncer,
		targetSyncer: targetSyncer,
	}
}

func (s *MigrationSyncer) Sync(ctx context.Context, evt *cloudevents.Event) error {
	// Resource or deploying events from another hub → target syncer
	if evt.Source() != constants.CloudEventGlobalHubClusterName {
		return s.targetSyncer.Sync(ctx, evt)
	}

	// Spec events from global hub → check payload to determine source or target
	var raw map[string]json.RawMessage
	if err := json.Unmarshal(evt.Data(), &raw); err == nil {
		if _, hasToHub := raw["toHub"]; hasToHub {
			return s.sourceSyncer.Sync(ctx, evt)
		}
	}

	return s.targetSyncer.Sync(ctx, evt)
}
