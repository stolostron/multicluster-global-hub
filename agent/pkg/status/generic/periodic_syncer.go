package generic

import (
	"context"
	"fmt"
	"time"

	ctrl "sigs.k8s.io/controller-runtime"
	"sigs.k8s.io/controller-runtime/pkg/client"

	"github.com/stolostron/multicluster-global-hub/agent/pkg/configs"
	"github.com/stolostron/multicluster-global-hub/agent/pkg/status/emitters"
)

type EmitterRegistration struct {
	ListFunc func() ([]client.Object, error)
	Emitter  emitters.Emitter
}

type PeriodicSyncer struct {
	syncIntervalFunc     func() time.Duration
	resyncIntervalFunc   func() time.Duration
	nextResyncAt         time.Time
	emitterRegistrations []*EmitterRegistration
}

func AddPeriodicSyncer(mgr ctrl.Manager,
	syncIntervalFunc func() time.Duration, resyncIntervalFunc func() time.Duration,
) (*PeriodicSyncer, error) {
	syncer := &PeriodicSyncer{
		syncIntervalFunc:   syncIntervalFunc,
		resyncIntervalFunc: resyncIntervalFunc,
		nextResyncAt:       time.Now().Add(resyncIntervalFunc()),
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

	for _, reg := range p.emitterRegistrations {
		if reg.Emitter.EventType() == e.Emitter.EventType() {
			log.Warnf("Emitter for event type %s is already registered, skipping", e.Emitter.EventType())
			return
		}
	}

	p.emitterRegistrations = append(p.emitterRegistrations, e)
	log.Infof("Registered emitter for event type: %s", e.Emitter.EventType())
}

func (p *PeriodicSyncer) Resync(ctx context.Context, eventType string) error {
	resynced := false
	for _, reg := range p.emitterRegistrations {
		if reg.Emitter.EventType() != eventType {
			continue
		}

		objects, err := reg.ListFunc()
		if err != nil {
			return fmt.Errorf("failed to list objects for event type %s in Resync: %w", reg.Emitter.EventType(), err)
		}

		if len(objects) == 0 {
			log.Infof("No objects found for event type: %s", reg.Emitter.EventType())
			continue
		}

		if err = reg.Emitter.Resync(objects); err != nil {
			return fmt.Errorf("failed to resync objects for event type %s: %w", reg.Emitter.EventType(), err)
		}
		resynced = true
		log.Infof("Successfully resynced %d objects for event type: %s", len(objects), reg.Emitter.EventType())
	}

	if !resynced {
		log.Infof("No emitter registered for event type: %s", eventType)
		return nil
	}
	return nil
}

func (p *PeriodicSyncer) Start(ctx context.Context) error {
	syncInterval := p.syncIntervalFunc()
	resyncInterval := p.resyncIntervalFunc()
	ticker := time.NewTicker(syncInterval)
	defer ticker.Stop()

	// start with an initial resync for all registered emitters
	for _, reg := range p.emitterRegistrations {
		if err := p.Resync(ctx, reg.Emitter.EventType()); err != nil {
			log.Errorf("failed to resync the event(%s): %v", reg.Emitter.EventType(), err)
		}
	}

	for {
		select {
		case <-ticker.C:

			// resync if the next resync time has passed
			if time.Now().After(p.nextResyncAt) {
				for _, reg := range p.emitterRegistrations {
					if err := p.Resync(ctx, reg.Emitter.EventType()); err != nil {
						log.Errorf("failed to resync the event(%s): %v", reg.Emitter.EventType(), err)
					}
				}
				p.nextResyncAt = time.Now().Add(resyncInterval)
			}

			// resync when the manager request, every tick to resync one
			if eventType := configs.GlobalResyncQueue.Pop(); eventType != "" {
				if err := p.Resync(ctx, eventType); err != nil {
					log.Errorf("failed to resync the request event(%s): %v", eventType, err)
				}
			}

			// send events for all registered emitters
			for _, reg := range p.emitterRegistrations {
				if err := reg.Emitter.Send(); err != nil {
					log.Errorf("failed to sync the event(%s): %v", reg.Emitter.EventType(), err)
				}
			}

			// check if sync or resync intervals have changed
			resolvedInterval := p.syncIntervalFunc()
			if resolvedInterval != syncInterval {
				syncInterval = resolvedInterval
				ticker.Reset(syncInterval)
				log.Infof("sync interval has been reset to %s", syncInterval.String())
			}
			resolvedResyncInterval := p.resyncIntervalFunc()
			if resolvedResyncInterval != resyncInterval {
				resyncInterval = resolvedResyncInterval
				p.nextResyncAt = time.Now().Add(resyncInterval)
				log.Infof("resync interval has been reset to %s", resyncInterval.String())
			}

		case <-ctx.Done():
			log.Info("Stopping periodic syncer...")
			return nil
		}
	}
}
