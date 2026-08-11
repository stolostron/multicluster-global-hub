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
// Update/Delete send immediately after a successful size-aware add (flush-first when the bundle
// would exceed MaxBundleBytes). Resync sends size-capped batches plus per-GVK ResyncMetadata
// inventory frames (InventoryBegin / Complete) so standby can safely clean stale resources.
// It is safe for concurrent use from multiple goroutines.
type HubHAEmitter struct {
	producer        transport.Producer
	transportConfig *transport.TransportInternalConfig
	activeHubName   string
	standbyHubName  string
	resourceFilter  *utils.HubHAResourceFilter
	bundle          *generic.GenericBundle[*unstructured.Unstructured]
	mu              sync.Mutex
	// deltaGen increments on every successful Update/Delete send. Resync captures it
	// before listing and aborts/retries emission if a delta landed after the snapshot.
	deltaGen uint64

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
		bundle: generic.NewGenericBundle(
			generic.WithKeyFunc(func(obj *unstructured.Unstructured) string {
				gvk := obj.GroupVersionKind()
				return fmt.Sprintf("%s/%s/%s/%s/%s",
					gvk.Group, gvk.Version, gvk.Kind, obj.GetNamespace(), obj.GetName())
			}),
		),
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

// Update handles object creation/update events with size-aware sending.
// When the bundle would exceed MaxBundleBytes, the current bundle is flushed first,
// then the new item is added and sent (fast failover for deltas).
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

	added, err := e.bundle.AddUpdate(uObj)
	if err != nil {
		return fmt.Errorf("failed to add update for %s/%s (%s): %w",
			uObj.GetNamespace(), uObj.GetName(), gvk.Kind, err)
	}
	if !added {
		if err := e.sendDeltaUnlocked(); err != nil {
			return err
		}
		added, err = e.bundle.AddUpdate(uObj)
		if err != nil {
			return fmt.Errorf("failed to add update for %s/%s (%s) after flush: %w",
				uObj.GetNamespace(), uObj.GetName(), gvk.Kind, err)
		}
		if !added {
			return fmt.Errorf("update object too large: %s/%s (%s)",
				uObj.GetNamespace(), uObj.GetName(), gvk.Kind)
		}
	}

	log.Debugw("syncing Hub HA update",
		"namespace", uObj.GetNamespace(), "name", uObj.GetName(), "kind", gvk.Kind)
	return e.sendDeltaUnlocked()
}

// Delete handles object deletion events with size-aware sending.
// When the bundle would exceed MaxBundleBytes, the current bundle is flushed first,
// then the delete is added and sent (fast failover for deltas).
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
		if existingObj.GetNamespace() == obj.GetNamespace() &&
			existingObj.GetName() == obj.GetName() &&
			existingObj.GroupVersionKind() == gvk {
			e.bundle.Update = append(e.bundle.Update[:i], e.bundle.Update[i+1:]...)
			break
		}
	}

	meta := generic.ObjectMetadata{
		Namespace: obj.GetNamespace(),
		Name:      obj.GetName(),
		Group:     gvk.Group,
		Version:   gvk.Version,
		Kind:      gvk.Kind,
	}
	added, err := e.bundle.AddDelete(meta)
	if err != nil {
		return fmt.Errorf("failed to add delete for %s/%s (%s): %w",
			obj.GetNamespace(), obj.GetName(), gvk.Kind, err)
	}
	if !added {
		if err := e.sendDeltaUnlocked(); err != nil {
			return err
		}
		added, err = e.bundle.AddDelete(meta)
		if err != nil {
			return fmt.Errorf("failed to add delete for %s/%s (%s) after flush: %w",
				obj.GetNamespace(), obj.GetName(), gvk.Kind, err)
		}
		if !added {
			return fmt.Errorf("delete metadata too large: %s/%s (%s)",
				obj.GetNamespace(), obj.GetName(), gvk.Kind)
		}
	}

	log.Debugw("syncing Hub HA delete",
		"namespace", obj.GetNamespace(), "name", obj.GetName(), "kind", gvk.Kind)
	return e.sendDeltaUnlocked()
}

// selfListTimeout is the maximum time allowed for a single Resync self-list round.
const selfListTimeout = 30 * time.Second

// maxResyncAttempts caps retries when Update/Delete invalidates a self-list snapshot.
const maxResyncAttempts = 5

// errResyncStaleSnapshot indicates a delta was sent after the resync inventory was captured.
var errResyncStaleSnapshot = errors.New("hub HA resync snapshot invalidated by concurrent delta")

// Resync performs a size-aware full resync.
// If objects is nil, the emitter self-lists using its stored client and activeResources
// (the "ListFunc=nil" pattern for PeriodicSyncer integration).
// Objects are sent in MaxBundleBytes batches. After each GVK, a ResyncMetadata bundle is
// sent so the standby can delete stale resources of that type.
//
// Self-list snapshots are validated against deltaGen: if Update/Delete sends a delta
// between the list and emission, Resync(nil) retries with a fresh list. Caller-provided
// object lists cannot be refreshed and return an error so the PeriodicSyncer can retry.
func (e *HubHAEmitter) Resync(objects []client.Object) error {
	provided := objects != nil
	var lastErr error
	for attempt := 1; attempt <= maxResyncAttempts; attempt++ {
		snapshot := objects
		if !provided {
			snapshot = nil
		}
		lastErr = e.resyncOnce(snapshot)
		if lastErr == nil {
			return nil
		}
		if !errors.Is(lastErr, errResyncStaleSnapshot) {
			return lastErr
		}
		if provided {
			return fmt.Errorf("%w: caller-provided snapshot cannot be refreshed", lastErr)
		}
		log.Infow("Hub HA resync retrying after concurrent delta",
			"attempt", attempt, "maxAttempts", maxResyncAttempts)
	}
	return fmt.Errorf("%w after %d attempts: %v", errResyncStaleSnapshot, maxResyncAttempts, lastErr)
}

func (e *HubHAEmitter) resyncOnce(objects []client.Object) error {
	// Phase 1: read fields under lock (short critical section, no I/O).
	e.mu.Lock()
	enabled := e.enabled
	cl := e.client
	gvks := append([]schema.GroupVersionKind(nil), e.activeResources...)
	gen := e.deltaGen
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

	// Group objects by GVK for per-type ResyncMetadata cleanup on standby.
	grouped := make(map[schema.GroupVersionKind][]*unstructured.Unstructured)
	var order []schema.GroupVersionKind
	for _, obj := range objects {
		uObj, err := toUnstructured(obj)
		if err != nil {
			return fmt.Errorf("failed to convert resync object to unstructured: %w", err)
		}
		gvk := uObj.GroupVersionKind()
		if !e.resourceFilter.ShouldSyncResource(obj, gvk) {
			continue
		}
		cleanUnstructuredMetadata(uObj)
		if _, ok := grouped[gvk]; !ok {
			order = append(order, gvk)
		}
		grouped[gvk] = append(grouped[gvk], uObj)
	}

	e.mu.Lock()
	defer e.mu.Unlock()

	// Re-check enabled in case it was toggled while we were listing.
	if !e.enabled {
		return nil
	}
	// A delta sent after the snapshot was taken must not be followed by a stale
	// resync recreate / inventory that marks the deleted object as live.
	if e.deltaGen != gen {
		return errResyncStaleSnapshot
	}

	e.bundle.Clean()
	totalObjects := 0

	// Include configured activeResources so empty inventories still emit a completion frame
	// (standby can delete all stale objects of that GVK).
	gvkOrder := make([]schema.GroupVersionKind, 0, len(gvks)+len(order))
	seenGVK := make(map[schema.GroupVersionKind]bool, len(gvks)+len(order))
	for _, gvk := range gvks {
		if seenGVK[gvk] {
			continue
		}
		seenGVK[gvk] = true
		gvkOrder = append(gvkOrder, gvk)
	}
	for _, gvk := range order {
		if seenGVK[gvk] {
			continue
		}
		seenGVK[gvk] = true
		gvkOrder = append(gvkOrder, gvk)
	}

	for _, gvk := range gvkOrder {
		objs := grouped[gvk]
		metadataList := make([]generic.ObjectMetadata, 0, len(objs))

		for _, uObj := range objs {
			added, err := e.bundle.AddResync(uObj)
			if err != nil {
				return fmt.Errorf("failed to add resync object %s/%s (%s): %w",
					uObj.GetNamespace(), uObj.GetName(), gvk.Kind, err)
			}
			if !added {
				if err := e.sendBundleUnlocked(); err != nil {
					return err
				}
				added, err = e.bundle.AddResync(uObj)
				if err != nil {
					return fmt.Errorf("failed to add resync object %s/%s (%s) after flush: %w",
						uObj.GetNamespace(), uObj.GetName(), gvk.Kind, err)
				}
				if !added {
					return fmt.Errorf("resync object too large: %s/%s (%s)",
						uObj.GetNamespace(), uObj.GetName(), gvk.Kind)
				}
			}

			metadataList = append(metadataList, generic.ObjectMetadata{
				Namespace: uObj.GetNamespace(),
				Name:      uObj.GetName(),
				Group:     gvk.Group,
				Version:   gvk.Version,
				Kind:      gvk.Kind,
			})
			totalObjects++
		}

		// Flush remaining resync objects for this GVK, then send inventory frames.
		if err := e.sendBundleUnlocked(); err != nil {
			return err
		}
		if err := e.sendResyncMetadataInventory(gvk, metadataList); err != nil {
			return err
		}
	}

	log.Infow("Hub HA resync completed", "objects", totalObjects, "gvks", len(gvkOrder))
	return nil
}

// sendResyncMetadataInventory sends per-GVK inventory frames that fit MaxBundleBytes.
// The first frame includes InventoryBegin; the last includes Complete. Empty inventories
// still emit a begin+complete frame so standby can delete all stale objects of that type.
// Caller must hold e.mu and the working bundle must be empty.
func (e *HubHAEmitter) sendResyncMetadataInventory(
	gvk schema.GroupVersionKind, metas []generic.ObjectMetadata,
) error {
	if len(metas) == 0 {
		return e.sendMetadataFrame(gvk, nil, true, true)
	}

	begin := true
	for i := 0; i < len(metas); {
		best := -1
		low, high := i+1, len(metas)
		for low <= high {
			mid := (low + high) / 2
			isLast := mid == len(metas)
			fits, err := metadataFrameFits(gvk, metas[i:mid], begin, isLast)
			if err != nil {
				return fmt.Errorf("failed to size resync metadata frame for %s: %w", gvk.Kind, err)
			}
			if fits {
				best = mid
				low = mid + 1
			} else {
				high = mid - 1
			}
		}
		if best < 0 {
			return fmt.Errorf("resync metadata entry too large for %s: %s/%s",
				gvk.Kind, metas[i].Namespace, metas[i].Name)
		}
		isLast := best == len(metas)
		if err := e.sendMetadataFrame(gvk, metas[i:best], begin, isLast); err != nil {
			return err
		}
		begin = false
		i = best
	}
	return nil
}

func metadataFrame(
	gvk schema.GroupVersionKind, metas []generic.ObjectMetadata, begin, complete bool,
) []generic.ObjectMetadata {
	frame := make([]generic.ObjectMetadata, 0, len(metas)+2)
	if begin {
		frame = append(frame, generic.ObjectMetadata{
			Group: gvk.Group, Version: gvk.Version, Kind: gvk.Kind, InventoryBegin: true,
		})
	}
	frame = append(frame, metas...)
	if complete {
		frame = append(frame, generic.ObjectMetadata{
			Group: gvk.Group, Version: gvk.Version, Kind: gvk.Kind, Complete: true,
		})
	}
	return frame
}

func metadataFrameFits(
	gvk schema.GroupVersionKind, metas []generic.ObjectMetadata, begin, complete bool,
) (bool, error) {
	tmp := generic.NewGenericBundle[*unstructured.Unstructured]()
	tmp.ResyncMetadata = metadataFrame(gvk, metas, begin, complete)
	size, err := tmp.Size()
	if err != nil {
		return false, fmt.Errorf("failed to marshal resync metadata frame: %w", err)
	}
	return size <= generic.MaxBundleBytes, nil
}

func (e *HubHAEmitter) sendMetadataFrame(
	gvk schema.GroupVersionKind, metas []generic.ObjectMetadata, begin, complete bool,
) error {
	frame := metadataFrame(gvk, metas, begin, complete)
	if err := e.bundle.AddResyncMetadata(frame); err != nil {
		return fmt.Errorf("failed to add resync metadata for %s: %w", gvk.Kind, err)
	}
	return e.sendBundleUnlocked()
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
	return e.sendDeltaUnlocked()
}

// sendDeltaUnlocked sends the current bundle and bumps deltaGen so in-flight Resync
// snapshots are invalidated. Caller must hold e.mu.
func (e *HubHAEmitter) sendDeltaUnlocked() error {
	if e.bundle.IsEmpty() {
		return nil
	}
	if err := e.sendBundleUnlocked(); err != nil {
		return err
	}
	e.deltaGen++
	return nil
}

// sendBundleUnlocked sends the bundle without acquiring the lock (caller must hold lock).
func (e *HubHAEmitter) sendBundleUnlocked() error {
	if e.bundle.IsEmpty() {
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

	log.Infow("sent Hub HA bundle",
		"updates", len(e.bundle.Update),
		"deletes", len(e.bundle.Delete),
		"resync", len(e.bundle.Resync),
		"resync_metadata", len(e.bundle.ResyncMetadata))

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
