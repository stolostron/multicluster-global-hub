// Copyright (c) 2025 Red Hat, Inc.
// Copyright Contributors to the Open Cluster Management project

package hubha

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"sync"
	"time"

	cecontext "github.com/cloudevents/sdk-go/v2/context"
	"k8s.io/apimachinery/pkg/api/meta"
	"k8s.io/apimachinery/pkg/apis/meta/v1/unstructured"
	"k8s.io/apimachinery/pkg/runtime/schema"
	"sigs.k8s.io/controller-runtime/pkg/client"
	"sigs.k8s.io/controller-runtime/pkg/predicate"

	"github.com/stolostron/multicluster-global-hub/pkg/bundle/generic"
	"github.com/stolostron/multicluster-global-hub/pkg/constants"
	"github.com/stolostron/multicluster-global-hub/pkg/transport"
	"github.com/stolostron/multicluster-global-hub/pkg/utils"
)

// HubHAEmitter implements the Emitter interface for Hub HA resource synchronization.
// It tracks changes to resources across all GVKs and bundles them for transmission to the standby hub.
// It is safe for concurrent use from multiple goroutines.
type HubHAEmitter struct {
	producer        transport.Producer
	transportConfig *transport.TransportInternalConfig
	activeHubName   string
	standbyHubName  string
	resourceFilter  *utils.HubHAResourceFilter
	bundle          *generic.GenericBundle[*unstructured.Unstructured]
	mu              sync.Mutex

	// Dynamic lifecycle fields managed by the Hub HA lifecycle controller.
	client          client.Client             // used for self-listing in Resync(nil)
	activeResources []schema.GroupVersionKind // GVKs to list during self-listing
	enabled         bool                      // when false, Update/Delete/Send are no-ops
}

// NewHubHAEmitter creates a new Hub HA emitter. The emitter starts disabled;
// call SetEnabled(true) once the resource controller is running and the hub is active.
func NewHubHAEmitter(
	producer transport.Producer,
	transportConfig *transport.TransportInternalConfig,
	activeHubName string,
	standbyHubName string,
) *HubHAEmitter {
	return &HubHAEmitter{
		producer:        producer,
		transportConfig: transportConfig,
		activeHubName:   activeHubName,
		standbyHubName:  standbyHubName,
		resourceFilter:  utils.NewHubHAResourceFilter(),
		bundle:          generic.NewGenericBundle[*unstructured.Unstructured](),
	}
}

// SetStandbyHub atomically updates the standby hub name (e.g. on failover target change).
func (e *HubHAEmitter) SetStandbyHub(hub string) {
	e.mu.Lock()
	defer e.mu.Unlock()
	e.standbyHubName = hub
}

// SetActiveResources stores the GVK list used for self-listing in Resync(nil).
func (e *HubHAEmitter) SetActiveResources(gvkList []schema.GroupVersionKind) {
	e.mu.Lock()
	defer e.mu.Unlock()
	e.activeResources = gvkList
}

// SetClient stores the Kubernetes client used for self-listing in Resync(nil).
func (e *HubHAEmitter) SetClient(c client.Client) {
	e.mu.Lock()
	defer e.mu.Unlock()
	e.client = c
}

// SetEnabled controls whether the emitter sends events.
// Set to true when the hub is in active role; false when standby or transitioning.
func (e *HubHAEmitter) SetEnabled(enabled bool) {
	e.mu.Lock()
	defer e.mu.Unlock()
	e.enabled = enabled
}

// EventType returns the event type for Hub HA resources.
func (e *HubHAEmitter) EventType() string {
	return constants.HubHAResourcesMsgKey
}

// Predicate returns the predicate for filtering events.
// Filters resources based on labels and namespace rules.
func (e *HubHAEmitter) Predicate() predicate.Predicate {
	return predicate.NewPredicateFuncs(func(obj client.Object) bool {
		// Convert to unstructured to get GVK
		uObj, err := toUnstructured(obj)
		if err != nil {
			log.Errorf("failed to convert object to unstructured in predicate: %v", err)
			return false
		}
		gvk := uObj.GroupVersionKind()
		return e.resourceFilter.ShouldSyncResource(obj, gvk)
	})
}

// Update handles object creation/update events and sends immediately.
// It is a no-op when the emitter is disabled (hub not in active role).
func (e *HubHAEmitter) Update(obj client.Object) error {
	e.mu.Lock()
	defer e.mu.Unlock()

	if !e.enabled {
		return nil
	}

	uObj, err := toUnstructured(obj)
	if err != nil {
		return fmt.Errorf("failed to convert object to unstructured: %w", err)
	}
	gvk := uObj.GroupVersionKind()
	cleanUnstructuredMetadata(uObj)

	e.bundle.Update = append(e.bundle.Update, uObj)
	log.Debugf("syncing update for %s/%s (%s)", uObj.GetNamespace(), uObj.GetName(), gvk.Kind)
	return e.sendBundleUnlocked()
}

// Delete handles object deletion events and sends immediately.
// It is a no-op when the emitter is disabled (hub not in active role).
func (e *HubHAEmitter) Delete(obj client.Object) error {
	e.mu.Lock()
	defer e.mu.Unlock()

	if !e.enabled {
		return nil
	}

	uObj, err := toUnstructured(obj)
	if err != nil {
		return fmt.Errorf("failed to convert object to unstructured: %w", err)
	}
	gvk := uObj.GroupVersionKind()

	for i, existingObj := range e.bundle.Update {
		if existingObj.GetNamespace() == obj.GetNamespace() && existingObj.GetName() == obj.GetName() {
			e.bundle.Update = append(e.bundle.Update[:i], e.bundle.Update[i+1:]...)
			break
		}
	}

	e.bundle.Delete = append(e.bundle.Delete, generic.ObjectMetadata{
		Namespace: obj.GetNamespace(),
		Name:      obj.GetName(),
		Group:     gvk.Group,
		Version:   gvk.Version,
		Kind:      gvk.Kind,
	})
	log.Debugf("syncing delete for %s/%s (%s)", obj.GetNamespace(), obj.GetName(), gvk.Kind)
	return e.sendBundleUnlocked()
}

// selfListTimeout is the maximum time allowed for a single Resync self-list round.
const selfListTimeout = 30 * time.Second

// Resync performs a full resync of all objects.
// If objects is nil, the emitter self-lists using its stored client and activeResources
// (the "ListFunc=nil" pattern for PeriodicSyncer integration).
// The mutex is released before any K8s API calls to avoid blocking Update/Delete/Send.
func (e *HubHAEmitter) Resync(objects []client.Object) error {
	// Phase 1: read fields under lock (short critical section, no I/O).
	e.mu.Lock()
	enabled := e.enabled
	cl := e.client
	gvks := append([]schema.GroupVersionKind(nil), e.activeResources...)
	e.mu.Unlock()

	if !enabled {
		return nil
	}

	if objects == nil {
		ctx, cancel := context.WithTimeout(context.Background(), selfListTimeout)
		defer cancel()
		var err error
		objects, err = e.selfList(ctx, cl, gvks)
		if err != nil {
			return fmt.Errorf("hub HA emitter self-list failed: %w", err)
		}
	}

	// Phase 2: update bundle state under lock (no I/O).
	e.mu.Lock()
	defer e.mu.Unlock()

	// Re-check enabled in case it was toggled while we were listing.
	if !e.enabled {
		return nil
	}

	e.bundle.Clean()
	for _, obj := range objects {
		uObj, err := toUnstructured(obj)
		if err != nil {
			log.Warnf("failed to convert object to unstructured during resync: %v", err)
			continue
		}
		gvk := uObj.GroupVersionKind()
		if !e.resourceFilter.ShouldSyncResource(obj, gvk) {
			continue
		}
		cleanUnstructuredMetadata(uObj)
		e.bundle.Resync = append(e.bundle.Resync, uObj)
	}

	// Bundle is populated; PeriodicSyncer's Send() tick will flush it.
	log.Infof("Hub HA resync: %d objects queued", len(e.bundle.Resync))
	return nil
}

// selfList lists all objects for each GVK. It does not require e.mu to be held.
// Non-IsNoMatchError failures are collected and returned together so the caller
// knows the result may be incomplete.
func (e *HubHAEmitter) selfList(
	ctx context.Context, cl client.Client, gvks []schema.GroupVersionKind,
) ([]client.Object, error) {
	if cl == nil {
		return nil, fmt.Errorf("client not set on HubHAEmitter, call SetClient before using Resync(nil)")
	}
	var all []client.Object
	var listErrs []error
	for _, gvk := range gvks {
		list := &unstructured.UnstructuredList{}
		list.SetGroupVersionKind(schema.GroupVersionKind{
			Group:   gvk.Group,
			Version: gvk.Version,
			Kind:    gvk.Kind + "List",
		})
		if err := cl.List(ctx, list); err != nil {
			if meta.IsNoMatchError(err) {
				log.Debugf("CRD not installed for %s, skipping in self-list resync", gvk.String())
				continue
			}
			listErrs = append(listErrs, fmt.Errorf("list failed for %s: %w", gvk.String(), err))
			continue
		}
		for i := range list.Items {
			all = append(all, &list.Items[i])
		}
	}
	if len(listErrs) > 0 {
		return all, fmt.Errorf("hub HA self-list encountered errors: %w", errors.Join(listErrs...))
	}
	return all, nil
}

// Send sends the current bundle to the standby hub.
// It is a no-op when the emitter is disabled or the bundle is empty.
func (e *HubHAEmitter) Send() error {
	e.mu.Lock()
	defer e.mu.Unlock()
	if !e.enabled {
		return nil
	}
	return e.sendBundleUnlocked()
}

// sendBundleUnlocked sends the bundle without acquiring the lock (caller must hold lock).
func (e *HubHAEmitter) sendBundleUnlocked() error {
	if len(e.bundle.Update) == 0 && len(e.bundle.Delete) == 0 && len(e.bundle.Resync) == 0 {
		return nil
	}

	// Create CloudEvent
	evt := utils.ToCloudEvent(
		constants.HubHAResourcesMsgKey,
		e.activeHubName,  // source = active hub
		e.standbyHubName, // clustername extension = standby hub
		e.bundle,
	)

	// Send to spec topic
	ctx := context.TODO()
	topicCtx := cecontext.WithTopic(ctx, e.transportConfig.KafkaCredential.SpecTopic)
	if err := e.producer.SendEvent(topicCtx, evt); err != nil {
		return fmt.Errorf("failed to send Hub HA bundle: %w", err)
	}

	log.Infof("sent Hub HA bundle: updates=%d, deletes=%d, resync=%d",
		len(e.bundle.Update), len(e.bundle.Delete), len(e.bundle.Resync))

	// Clear bundle after successful send
	e.bundle.Clean()
	return nil
}

// toUnstructured converts a client.Object to *unstructured.Unstructured.
func toUnstructured(obj client.Object) (*unstructured.Unstructured, error) {
	if uObj, ok := obj.(*unstructured.Unstructured); ok {
		return uObj.DeepCopy(), nil
	}

	// Convert via JSON marshaling (works for all types)
	data, err := json.Marshal(obj)
	if err != nil {
		return nil, err
	}

	uObj := &unstructured.Unstructured{}
	if err := json.Unmarshal(data, uObj); err != nil {
		return nil, err
	}

	return uObj, nil
}

// cleanUnstructuredMetadata removes metadata that should not be synced.
func cleanUnstructuredMetadata(obj *unstructured.Unstructured) {
	obj.SetManagedFields(nil)
	obj.SetResourceVersion("")
	obj.SetGeneration(0)
	obj.SetUID("")
}
