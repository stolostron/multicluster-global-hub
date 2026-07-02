package generic

import (
	"context"
	"fmt"
	"sync"
	"time"

	ctrl "sigs.k8s.io/controller-runtime"
	"sigs.k8s.io/controller-runtime/pkg/client"

	"github.com/stolostron/multicluster-global-hub/agent/pkg/configs"
	"github.com/stolostron/multicluster-global-hub/agent/pkg/status/emitters"
	"github.com/stolostron/multicluster-global-hub/agent/pkg/status/syncers/configmap"
	"github.com/stolostron/multicluster-global-hub/pkg/enum"
)

// EmitterRegistration registers a bundle with the periodic syncer.
// ListFunc is optional: if nil, the emitter is expected to handle its own listing
// internally via Emitter.Resync(nil).
type EmitterRegistration struct {
	ListFunc func() ([]client.Object, error)
	Emitter  emitters.Emitter
}

// SyncState holds scheduling metadata for a registered emitter.
type SyncState struct {
	Registration *EmitterRegistration
	NextResyncAt time.Time
	NextSyncAt   time.Time
}

// PeriodicSyncer drives delta-sync and periodic full-resync for all registered emitters.
// It is safe for concurrent Register/Unregister calls after the manager has started.
type PeriodicSyncer struct {
	mu         sync.Mutex
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

// Register adds an emitter to the periodic syncer. ListFunc may be nil for emitters
// that self-list inside their Resync(nil) implementation (e.g. Hub HA emitter).
func (p *PeriodicSyncer) Register(e *EmitterRegistration) {
	if e.Emitter == nil {
		log.Warnf("Emitter registration is invalid (nil emitter), skipping")
		return
	}

	p.mu.Lock()
	defer p.mu.Unlock()

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
	configs.GlobalResyncQueue.Add(e.Emitter.EventType())
	log.Infof("registered emitter for event type: %s", enum.ShortenEventType(e.Emitter.EventType()))
}

// CountRegistered returns the number of currently registered emitters.
// Intended for use in tests.
func (p *PeriodicSyncer) CountRegistered() int {
	p.mu.Lock()
	defer p.mu.Unlock()
	return len(p.syncStates)
}

// Unregister removes the emitter for the given eventType from the periodic syncer.
// It is a no-op if no emitter is registered for that event type.
func (p *PeriodicSyncer) Unregister(eventType string) {
	p.mu.Lock()
	defer p.mu.Unlock()

	for i, state := range p.syncStates {
		if state.Registration.Emitter.EventType() == eventType {
			p.syncStates = append(p.syncStates[:i], p.syncStates[i+1:]...)
			log.Infof("unregistered emitter for event type: %s", enum.ShortenEventType(eventType))
			return
		}
	}
	log.Warnf("no emitter registered for event type %s, nothing to unregister", eventType)
}

func (p *PeriodicSyncer) Resync(ctx context.Context, eventType string) error {
	p.mu.Lock()
	snapshot := make([]*SyncState, len(p.syncStates))
	copy(snapshot, p.syncStates)
	p.mu.Unlock()

	resynced := false
	for _, state := range snapshot {
		registeredType := state.Registration.Emitter.EventType()
		if registeredType != eventType {
			continue
		}

		var err error
		if state.Registration.ListFunc != nil {
			objects, listErr := state.Registration.ListFunc()
			if listErr != nil {
				return fmt.Errorf("failed to list objects for event type %s in Resync: %w", registeredType, listErr)
			}
			if len(objects) == 0 {
				log.Infof("No objects found for event type: %s", registeredType)
				return nil
			}
			err = state.Registration.Emitter.Resync(objects)
			if err == nil {
				log.Infof("resynced %d objects for event type: %s", len(objects), enum.ShortenEventType(registeredType))
			}
		} else {
			// Emitter handles its own listing internally via Resync(nil)
			err = state.Registration.Emitter.Resync(nil)
			if err == nil {
				log.Infof("resynced (self-listing) for event type: %s", enum.ShortenEventType(registeredType))
			}
		}

		if err != nil {
			return fmt.Errorf("failed to resync objects for event type %s: %w", registeredType, err)
		}
		resynced = true
		state.NextResyncAt = time.Now().Add(configmap.GetResyncInterval(enum.EventType(registeredType)))
	}

	if !resynced {
		log.Infof("No emitter registered for event type: %s", eventType)
	}
	return nil
}

func (p *PeriodicSyncer) Start(ctx context.Context) error {
	ticker := time.NewTicker(1 * time.Second)
	defer ticker.Stop()

	for {
		select {
		case <-ticker.C:
			if eventType := configs.GlobalResyncQueue.Pop(); eventType != "" {
				log.Infof("resyncing event type: %s", enum.ShortenEventType(eventType))
				if err := p.Resync(ctx, eventType); err != nil {
					log.Errorf("failed to resync the request event(%s): %v", eventType, err)
				}
			}

			// Snapshot the states to avoid holding the lock during I/O operations.
			// SyncState pointers are stable, so NextSyncAt/NextResyncAt updates
			// on the originals are safe even after the snapshot copy.
			p.mu.Lock()
			snapshot := make([]*SyncState, len(p.syncStates))
			copy(snapshot, p.syncStates)
			p.mu.Unlock()

			for _, state := range snapshot {
				eventType := state.Registration.Emitter.EventType()
				if eventType == "" {
					continue
				}

				if time.Now().After(state.NextSyncAt) {
					if err := state.Registration.Emitter.Send(); err != nil {
						log.Errorf("failed to sync the event(%s): %v", enum.ShortenEventType(eventType), err)
					}
					state.NextSyncAt = time.Now().Add(configmap.GetSyncInterval(enum.EventType(eventType)))
				}

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
