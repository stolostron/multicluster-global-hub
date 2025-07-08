package generic

import (
	"context"
	"fmt"
	"time"

	ctrl "sigs.k8s.io/controller-runtime"
	"sigs.k8s.io/controller-runtime/pkg/client"

	"github.com/stolostron/multicluster-global-hub/agent/pkg/configs"
	"github.com/stolostron/multicluster-global-hub/agent/pkg/status/emitters"
	"github.com/stolostron/multicluster-global-hub/agent/pkg/status/syncers/configmap"
	"github.com/stolostron/multicluster-global-hub/pkg/enum"
)

// EmitterRegistration registers a bundle with the periodic syncer,
// providing both the delta emitter and the full data listing function.
type EmitterRegistration struct {
	ListFunc func() ([]client.Object, error)
	Emitter  emitters.Emitter
}

type SyncState struct {
	Registration *EmitterRegistration
	NextResyncAt time.Time
	NextSyncAt   time.Time
}

type PeriodicSyncer struct {
	syncStates []*SyncState
}

func AddPeriodicSyncer(mgr ctrl.Manager) (*PeriodicSyncer, error) {
	syncer := &PeriodicSyncer{
		syncStates: []*SyncState{},
	}
	if err := mgr.Add(syncer); err != nil {
		return nil, err
	}
	return syncer, nil
}

// Register adds an emitter to the periodic syncer.
func (p *PeriodicSyncer) Register(e *EmitterRegistration) {
	if e.ListFunc == nil || e.Emitter == nil {
		log.Warnf("Emitter registration for event type %v is invalid, skipping", e)
		return
	}

	for _, state := range p.syncStates {
		if state.Registration.Emitter.EventType() == e.Emitter.EventType() {
			log.Warnf("Emitter for event type %s is already registered, skipping", e.Emitter.EventType())
			return
		}
	}

	nextSyncAt := time.Now().Add(configmap.GetSyncInterval(enum.EventType(e.Emitter.EventType())))
	nextResyncAt := time.Now().Add(configmap.GetResyncInterval(enum.EventType(e.Emitter.EventType())))

	p.syncStates = append(p.syncStates, &SyncState{
		Registration: e,
		NextSyncAt:   nextSyncAt,
		NextResyncAt: nextResyncAt,
	})
	log.Infof("Registered emitter for event type: %s", e.Emitter.EventType())
}

func (p *PeriodicSyncer) Resync(ctx context.Context, eventType string) error {
	resynced := false
	for _, state := range p.syncStates {
		registeredType := state.Registration.Emitter.EventType()
		if registeredType != eventType {
			continue
		}

		objects, err := state.Registration.ListFunc()
		if err != nil {
			return fmt.Errorf("failed to list objects for event type %s in Resync: %w", registeredType, err)
		}

		if len(objects) == 0 {
			log.Infof("No objects found for event type: %s", registeredType)
			continue
		}

		if err = state.Registration.Emitter.Resync(objects); err != nil {
			return fmt.Errorf("failed to resync objects for event type %s: %w", registeredType, err)
		}
		resynced = true
		// Update the next resync time for this emitter
		state.NextResyncAt = time.Now().Add(configmap.GetResyncInterval(enum.EventType(registeredType)))
		log.Infof("Successfully resynced %d objects for event type: %s", len(objects), registeredType)
	}

	if !resynced {
		log.Infof("No emitter registered for event type: %s", eventType)
		return nil
	}
	return nil
}

func (p *PeriodicSyncer) Start(ctx context.Context) error {
	ticker := time.NewTicker(1 * time.Second)
	defer ticker.Stop()

	// start with an initial resync for all registered emitters
	for _, state := range p.syncStates {
		configs.GlobalResyncQueue.Add(state.Registration.Emitter.EventType())
	}

	for {
		select {
		case <-ticker.C:

			// resync when the manager request, every tick to resync one
			if eventType := configs.GlobalResyncQueue.Pop(); eventType != "" {
				log.Infof("Resyncing event type: %s", eventType)
				if err := p.Resync(ctx, eventType); err != nil {
					log.Errorf("failed to resync the request event(%s): %v", eventType, err)
				}
			}

			// sync all registered emitters
			for _, state := range p.syncStates {
				eventType := state.Registration.Emitter.EventType()
				if eventType == "" {
					log.Warnf("Emitter for event type %s is not registered, skipping", eventType)
					continue
				}

				// delta sync if the next sync time has passed
				if time.Now().After(state.NextSyncAt) {
					log.Debugf("Syncing event type: %s", enum.ShortenEventType(eventType))
					if err := state.Registration.Emitter.Send(); err != nil {
						log.Errorf("failed to sync the event(%s): %v", enum.ShortenEventType(eventType), err)
					}
					state.NextSyncAt = time.Now().Add(configmap.GetSyncInterval(enum.EventType(eventType)))
				}

				// check if the next resync time has passed
				if time.Now().After(state.NextResyncAt) {
					log.Infof("Resyncing event type: %s", eventType)
					if err := p.Resync(ctx, eventType); err != nil {
						log.Errorf("failed to resync the event(%s): %v", eventType, err)
					}
					state.NextResyncAt = time.Now().Add(configmap.GetResyncInterval(enum.EventType(eventType)))
				}
			}

		case <-ctx.Done():
			log.Info("Stopping periodic syncer...")
			return nil
		}
	}
}
