// Copyright (c) 2025 Red Hat, Inc.
// Copyright Contributors to the Open Cluster Management project

package hubha

import (
	"context"
	"encoding/json"
	"fmt"
	"sync"

	cecontext "github.com/cloudevents/sdk-go/v2/context"
	"k8s.io/apimachinery/pkg/apis/meta/v1/unstructured"
	"sigs.k8s.io/controller-runtime/pkg/client"
	"sigs.k8s.io/controller-runtime/pkg/predicate"

	"github.com/stolostron/multicluster-global-hub/pkg/bundle/generic"
	"github.com/stolostron/multicluster-global-hub/pkg/constants"
	"github.com/stolostron/multicluster-global-hub/pkg/transport"
	"github.com/stolostron/multicluster-global-hub/pkg/utils"
)

// HubHAEmitter implements the Emitter interface for Hub HA resource synchronization.
// It tracks changes to resources across all GVKs and bundles them for transmission to the standby hub.
type HubHAEmitter struct {
	producer        transport.Producer
	transportConfig *transport.TransportInternalConfig
	activeHubName   string
	standbyHubName  string
	resourceFilter  *utils.HubHAResourceFilter
	bundle          *generic.GenericBundle[*unstructured.Unstructured]
	mu              sync.Mutex
}

// NewHubHAEmitter creates a new Hub HA emitter that handles all Hub HA resources.
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

// EventType returns the event type for Hub HA resources.
func (e *HubHAEmitter) EventType() string {
	return constants.HubHAResourcesMsgKey
}

// Predicate returns the predicate for filtering events.
// We want to watch all events and filter at the Update/Delete level.
func (e *HubHAEmitter) Predicate() predicate.Predicate {
	return predicate.NewPredicateFuncs(func(object client.Object) bool {
		// Accept all objects - filtering happens in Update/Delete methods
		return true
	})
}

// Update handles object creation/update events and sends immediately.
func (e *HubHAEmitter) Update(obj client.Object) error {
	e.mu.Lock()
	defer e.mu.Unlock()

	// Convert to unstructured to get GVK
	uObj, err := toUnstructured(obj)
	if err != nil {
		return fmt.Errorf("failed to convert object to unstructured: %w", err)
	}

	// Get GVK from object
	gvk := uObj.GroupVersionKind()

	// Filter using resource filter
	if !e.resourceFilter.ShouldSyncResource(obj, gvk) {
		return nil
	}

	// Clean metadata
	cleanUnstructuredMetadata(uObj)

	// Add to bundle.Update and send immediately
	e.bundle.Update = append(e.bundle.Update, uObj)
	log.Debugf("syncing update for %s/%s (%s)", uObj.GetNamespace(), uObj.GetName(), gvk.Kind)

	// Send immediately for fast failover
	return e.sendBundleUnlocked()
}

// Delete handles object deletion events and sends immediately.
func (e *HubHAEmitter) Delete(obj client.Object) error {
	e.mu.Lock()
	defer e.mu.Unlock()

	// Convert to unstructured to get GVK
	uObj, err := toUnstructured(obj)
	if err != nil {
		return fmt.Errorf("failed to convert object to unstructured: %w", err)
	}

	// Get GVK from object
	gvk := uObj.GroupVersionKind()

	// Note: We don't filter deletions because when an object is deleted,
	// it doesn't have labels anymore. If it was previously synced (had required labels),
	// we should also sync its deletion. The standby syncer handles NotFound gracefully.

	// Remove from update list if present
	for i, existingObj := range e.bundle.Update {
		if existingObj.GetNamespace() == obj.GetNamespace() && existingObj.GetName() == obj.GetName() {
			e.bundle.Update = append(e.bundle.Update[:i], e.bundle.Update[i+1:]...)
			break
		}
	}

	// Add to bundle.Delete and send immediately
	objMeta := generic.ObjectMetadata{
		Namespace: obj.GetNamespace(),
		Name:      obj.GetName(),
		Group:     gvk.Group,
		Version:   gvk.Version,
		Kind:      gvk.Kind,
	}

	e.bundle.Delete = append(e.bundle.Delete, objMeta)
	log.Debugf("syncing delete for %s/%s (%s)", obj.GetNamespace(), obj.GetName(), gvk.Kind)

	// Send immediately for fast failover
	return e.sendBundleUnlocked()
}

// Resync performs a full resync of all objects.
func (e *HubHAEmitter) Resync(objects []client.Object) error {
	e.mu.Lock()
	defer e.mu.Unlock()

	// Clear existing bundle
	e.bundle.Clean()

	for _, obj := range objects {
		// Convert to unstructured to get GVK
		uObj, err := toUnstructured(obj)
		if err != nil {
			log.Warnf("failed to convert object to unstructured during resync: %v", err)
			continue
		}

		// Get GVK from object
		gvk := uObj.GroupVersionKind()

		// Filter using resource filter
		if !e.resourceFilter.ShouldSyncResource(obj, gvk) {
			continue
		}

		// Clean metadata
		cleanUnstructuredMetadata(uObj)

		// Add to bundle.Resync
		e.bundle.Resync = append(e.bundle.Resync, uObj)
	}

	log.Infof("resynced %d Hub HA objects", len(e.bundle.Resync))
	return nil
}

// Send sends the current bundle to the standby hub.
func (e *HubHAEmitter) Send() error {
	e.mu.Lock()
	defer e.mu.Unlock()
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
		return fmt.Errorf("failed to send Hub HA bundle from %s to %s: %w",
			e.activeHubName, e.standbyHubName, err)
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
