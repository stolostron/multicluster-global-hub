// Copyright (c) 2025 Red Hat, Inc.
// Copyright Contributors to the Open Cluster Management project

package hubha

import (
	"context"
	stderrors "errors"
	"fmt"
	"sync"

	cloudevents "github.com/cloudevents/sdk-go/v2"
	"k8s.io/apimachinery/pkg/api/errors"
	"k8s.io/apimachinery/pkg/api/meta"
	"k8s.io/apimachinery/pkg/apis/meta/v1/unstructured"
	"k8s.io/apimachinery/pkg/runtime/schema"
	"k8s.io/client-go/util/retry"
	clusterv1 "open-cluster-management.io/api/cluster/v1"
	"sigs.k8s.io/controller-runtime/pkg/client"
	"sigs.k8s.io/controller-runtime/pkg/controller/controllerutil"

	"github.com/stolostron/multicluster-global-hub/pkg/bundle/generic"
	"github.com/stolostron/multicluster-global-hub/pkg/constants"
	"github.com/stolostron/multicluster-global-hub/pkg/utils"
)

// HubHAStandbySyncer receives ACM resources from active hub and applies them to standby hub
type HubHAStandbySyncer struct {
	client         client.Client
	resourceFilter *utils.HubHAResourceFilter

	mu sync.Mutex
	// pendingResyncSessions accumulates live object metadata per GVK until cleanup after
	// Complete succeeds. Partial ResyncMetadata frames must not trigger stale cleanup.
	pendingResyncSessions map[string]*resyncInventorySession
}

// resyncInventorySession tracks one per-GVK inventory across size-split frames.
type resyncInventorySession struct {
	items     map[string]generic.ObjectMetadata
	beginSeen bool
}

func NewHubHAStandbySyncer(c client.Client) *HubHAStandbySyncer {
	return &HubHAStandbySyncer{
		client:                c,
		resourceFilter:        utils.NewHubHAResourceFilter(),
		pendingResyncSessions: make(map[string]*resyncInventorySession),
	}
}

// Sync processes CloudEvents containing Hub HA resources from active hub
func (s *HubHAStandbySyncer) Sync(ctx context.Context, evt *cloudevents.Event) error {
	// Only process Hub HA resource events
	if evt.Type() != constants.HubHAResourcesMsgKey {
		return nil
	}

	source := evt.Source()
	if source == "" || source == constants.CloudEventGlobalHubClusterName {
		log.Warnw("dropping Hub HA resource event with untrusted source", "source", source, "type", evt.Type())
		return nil
	}

	log.Infof("standby hub received Hub HA resources from active hub: %s", source)

	// Unmarshal the bundle
	bundle := generic.NewGenericBundle[*unstructured.Unstructured]()
	if err := evt.DataAs(bundle); err != nil {
		return fmt.Errorf("failed to unmarshal Hub HA resource bundle: %w", err)
	}

	sourceHub := source
	var syncErrs []error

	// Apply created resources
	for _, obj := range bundle.Create {
		if err := s.createResource(ctx, obj, sourceHub); err != nil {
			log.Errorf("failed to create resource %s/%s from active hub %s: %v",
				obj.GetNamespace(), obj.GetName(), sourceHub, err)
			syncErrs = append(syncErrs, err)
		}
	}

	// Apply updated resources
	for _, obj := range bundle.Update {
		if err := s.updateResource(ctx, obj, sourceHub); err != nil {
			log.Errorf("failed to update resource %s/%s from active hub %s: %v",
				obj.GetNamespace(), obj.GetName(), sourceHub, err)
			syncErrs = append(syncErrs, err)
		}
	}

	// Handle resync (same as create/update)
	for _, obj := range bundle.Resync {
		if err := s.updateResource(ctx, obj, sourceHub); err != nil {
			log.Errorf("failed to resync resource %s/%s from active hub %s: %v",
				obj.GetNamespace(), obj.GetName(), sourceHub, err)
			syncErrs = append(syncErrs, err)
		}
	}

	// Handle deleted resources
	for _, meta := range bundle.Delete {
		if err := s.deleteResource(ctx, &meta, sourceHub); err != nil {
			log.Errorf("failed to delete resource %s/%s from active hub %s: %v",
				meta.Namespace, meta.Name, sourceHub, err)
			syncErrs = append(syncErrs, err)
		}
	}

	// Handle resync metadata: accumulate inventory frames; cleanup only after Complete.
	if len(bundle.ResyncMetadata) > 0 {
		if err := s.handleResyncMetadata(ctx, bundle.ResyncMetadata, sourceHub); err != nil {
			log.Errorw("failed to handle resync metadata", "sourceHub", sourceHub, "error", err)
			syncErrs = append(syncErrs, err)
		}
	}

	log.Infof("standby hub processed Hub HA bundle from %s: created=%d, updated=%d, "+
		"resynced=%d, deleted=%d, resync_metadata=%d",
		sourceHub, len(bundle.Create), len(bundle.Update), len(bundle.Resync), len(bundle.Delete),
		len(bundle.ResyncMetadata))

	if len(syncErrs) > 0 {
		return fmt.Errorf("failed to apply Hub HA bundle from %s: %w", sourceHub, stderrors.Join(syncErrs...))
	}

	return nil
}

func (s *HubHAStandbySyncer) createResource(ctx context.Context, obj *unstructured.Unstructured,
	sourceHub string,
) error {
	log.Infof("creating resource from active hub %s: %s/%s (%s)",
		sourceHub, obj.GetNamespace(), obj.GetName(), obj.GetKind())

	// Clean ownerReferences to avoid permission issues
	// Owner resources may not exist on standby hub, and ownership will be
	// recreated by controllers on standby if needed
	obj.SetOwnerReferences(nil)

	// For ManagedCluster resources, set hubAcceptsClient to false
	// This prevents standby hub from accepting client connections while active hub is healthy
	gvk := obj.GroupVersionKind()
	if gvk.Group == clusterv1.GroupName && gvk.Kind == "ManagedCluster" {
		if err := s.setHubAcceptsClient(obj, false); err != nil {
			return fmt.Errorf("failed to set ManagedCluster hubAcceptsClient: %w", err)
		}
	}

	err := retry.RetryOnConflict(retry.DefaultRetry, func() error {
		_, err := controllerutil.CreateOrUpdate(ctx, s.client, obj, func() error {
			// Resource will be created or updated as needed
			return nil
		})
		return err
	})
	if err != nil {
		return fmt.Errorf("failed to create/update resource: %w", err)
	}

	log.Debugf("successfully created/updated resource %s/%s from active hub %s",
		obj.GetNamespace(), obj.GetName(), sourceHub)
	return nil
}

func (s *HubHAStandbySyncer) updateResource(ctx context.Context, obj *unstructured.Unstructured,
	sourceHub string,
) error {
	log.Debugf("updating resource from active hub %s: %s/%s (%s)",
		sourceHub, obj.GetNamespace(), obj.GetName(), obj.GetKind())

	// Clean ownerReferences to avoid permission issues
	// Owner resources may not exist on standby hub, and ownership will be
	// recreated by controllers on standby if needed
	obj.SetOwnerReferences(nil)
	// Status is hub-local; including it in a full Update can fail ManagedCluster apply.
	unstructured.RemoveNestedField(obj.Object, "status")

	// For ManagedCluster resources, set hubAcceptsClient to false
	// This prevents standby hub from accepting client connections while active hub is healthy
	gvk := obj.GroupVersionKind()
	if gvk.Group == clusterv1.GroupName && gvk.Kind == "ManagedCluster" {
		if err := s.setHubAcceptsClient(obj, false); err != nil {
			return fmt.Errorf("failed to set ManagedCluster hubAcceptsClient: %w", err)
		}
	}

	err := retry.RetryOnConflict(retry.DefaultRetry, func() error {
		existing := &unstructured.Unstructured{}
		existing.SetGroupVersionKind(obj.GroupVersionKind())

		if err := s.client.Get(ctx, client.ObjectKeyFromObject(obj), existing); err != nil {
			// Resource doesn't exist, create it
			return s.client.Create(ctx, obj)
		}

		// Preserve standby identity and controller-owned finalizers.
		obj.SetResourceVersion(existing.GetResourceVersion())
		obj.SetUID(existing.GetUID())
		obj.SetFinalizers(existing.GetFinalizers())

		return s.client.Update(ctx, obj)
	})
	if err != nil {
		return fmt.Errorf("failed to update resource: %w", err)
	}

	return nil
}

func (s *HubHAStandbySyncer) deleteResource(ctx context.Context, meta *generic.ObjectMetadata, sourceHub string) error {
	log.Infof("deleting Hub HA resource from active hub %s: %s/%s (%s)",
		sourceHub, meta.Namespace, meta.Name, meta.Kind)

	// Validate GVK information
	if meta.Kind == "" {
		log.Warnf("cannot delete resource %s/%s - missing Kind in metadata", meta.Namespace, meta.Name)
		return fmt.Errorf("missing Kind in ObjectMetadata for %s/%s", meta.Namespace, meta.Name)
	}

	// Construct unstructured object for deletion
	obj := &unstructured.Unstructured{}
	obj.SetGroupVersionKind(schema.GroupVersionKind{
		Group:   meta.Group,
		Version: meta.Version,
		Kind:    meta.Kind,
	})
	obj.SetNamespace(meta.Namespace)
	obj.SetName(meta.Name)

	// Delete the resource
	err := retry.RetryOnConflict(retry.DefaultRetry, func() error {
		return s.client.Delete(ctx, obj)
	})
	if err != nil {
		if errors.IsNotFound(err) {
			log.Debugf("resource %s/%s already deleted from standby hub", meta.Namespace, meta.Name)
			return nil
		}
		return fmt.Errorf("failed to delete resource: %w", err)
	}

	log.Infof("successfully deleted resource %s/%s from standby hub", meta.Namespace, meta.Name)
	return nil
}

// handleResyncMetadata processes inventory frames for a GVK.
// Frames may include InventoryBegin (start/reset session) and/or Complete (run stale cleanup).
// Markerless middle frames append to an existing session; without a session they are treated
// as a legacy single-shot inventory. The session is retained until cleanup succeeds so a
// retried Complete frame still has the full inventory. Complete without an observed
// InventoryBegin is ignored (not treated as an empty inventory).
func (s *HubHAStandbySyncer) handleResyncMetadata(
	ctx context.Context, metadata []generic.ObjectMetadata, sourceHub string,
) error {
	hasBegin := false
	hasComplete := false
	var gvk schema.GroupVersionKind
	for _, m := range metadata {
		if m.Kind != "" {
			gvk = schema.GroupVersionKind{Group: m.Group, Version: m.Version, Kind: m.Kind}
		}
		if m.InventoryBegin {
			hasBegin = true
		}
		if m.Complete {
			hasComplete = true
		}
	}

	if gvk.Kind == "" {
		log.Warnw("no valid GVK found in resync metadata", "sourceHub", sourceHub)
		return nil
	}
	gvkKey := gvk.String()

	s.mu.Lock()
	if s.pendingResyncSessions == nil {
		s.pendingResyncSessions = make(map[string]*resyncInventorySession)
	}
	session, sessionExists := s.pendingResyncSessions[gvkKey]

	// Markerless frame: append to an in-flight session, otherwise legacy single-shot cleanup.
	if !hasBegin && !hasComplete {
		if !sessionExists {
			s.mu.Unlock()
			return s.cleanupStaleResources(ctx, metadata, sourceHub)
		}
		s.appendInventoryEntries(session, metadata)
		s.mu.Unlock()
		return nil
	}

	if hasBegin {
		session = &resyncInventorySession{
			items:     make(map[string]generic.ObjectMetadata),
			beginSeen: true,
		}
		s.pendingResyncSessions[gvkKey] = session
		sessionExists = true
	}

	if !sessionExists {
		// Complete (or other markers) without an open session / InventoryBegin must not
		// run empty-inventory cleanup.
		s.mu.Unlock()
		if hasComplete {
			log.Warnw("ignoring ResyncMetadata Complete without InventoryBegin",
				"sourceHub", sourceHub, "gvk", gvk.String())
		}
		return nil
	}

	s.appendInventoryEntries(session, metadata)

	if !hasComplete {
		s.mu.Unlock()
		return nil
	}

	if !session.beginSeen {
		s.mu.Unlock()
		log.Warnw("ignoring ResyncMetadata Complete without InventoryBegin",
			"sourceHub", sourceHub, "gvk", gvk.String())
		return nil
	}

	// Copy inventory for cleanup; keep the session until cleanup succeeds so retries
	// of the Complete frame still see the full accumulated set.
	full := make([]generic.ObjectMetadata, 0, len(session.items)+1)
	for _, m := range session.items {
		full = append(full, m)
	}
	full = append(full, generic.ObjectMetadata{
		Group: gvk.Group, Version: gvk.Version, Kind: gvk.Kind,
	})
	s.mu.Unlock()

	if err := s.cleanupStaleResources(ctx, full, sourceHub); err != nil {
		return err
	}

	s.mu.Lock()
	delete(s.pendingResyncSessions, gvkKey)
	s.mu.Unlock()
	return nil
}

func (s *HubHAStandbySyncer) appendInventoryEntries(
	session *resyncInventorySession, metadata []generic.ObjectMetadata,
) {
	for _, m := range metadata {
		if m.InventoryBegin || m.Complete || m.Name == "" {
			continue
		}
		session.items[m.Key()] = m
	}
}

// cleanupStaleResources deletes local resources of the metadata GVK that are not listed
// in the active hub's ResyncMetadata set (i.e., stale copies removed from active).
func (s *HubHAStandbySyncer) cleanupStaleResources(
	ctx context.Context, metadata []generic.ObjectMetadata, sourceHub string,
) error {
	activeKeys := make(map[string]bool, len(metadata))
	var gvk schema.GroupVersionKind
	for _, m := range metadata {
		if m.Kind == "" {
			continue
		}
		if gvk.Kind == "" {
			gvk = schema.GroupVersionKind{Group: m.Group, Version: m.Version, Kind: m.Kind}
		}
		if m.InventoryBegin || m.Complete || m.Name == "" {
			continue
		}
		activeKeys[m.Key()] = true
	}

	if gvk.Kind == "" {
		log.Warnw("no valid GVK found in resync metadata", "sourceHub", sourceHub)
		return nil
	}

	list := &unstructured.UnstructuredList{}
	list.SetGroupVersionKind(schema.GroupVersionKind{
		Group: gvk.Group, Version: gvk.Version, Kind: gvk.Kind + "List",
	})
	if err := s.client.List(ctx, list); err != nil {
		if meta.IsNoMatchError(err) {
			return nil
		}
		return fmt.Errorf("failed to list %s for stale cleanup: %w", gvk, err)
	}

	var deleteErrs []error
	for i := range list.Items {
		obj := &list.Items[i]
		if s.resourceFilter != nil && !s.resourceFilter.ShouldSyncResource(obj, gvk) {
			continue
		}
		// Global Hub standby also hosts topology ManagedClusters (imported hubs,
		// local-cluster). Those are not copies of the active regional hub inventory
		// and must not be deleted by ResyncMetadata stale cleanup.
		if gvk.Group == clusterv1.GroupName && gvk.Kind == "ManagedCluster" &&
			shouldSkipHAAnnotation(obj) {
			log.Debugf("skipping Hub HA stale cleanup for global-hub topology ManagedCluster %s",
				obj.GetName())
			continue
		}
		key := generic.ObjectMetadata{
			Namespace: obj.GetNamespace(),
			Name:      obj.GetName(),
			Group:     gvk.Group,
			Version:   gvk.Version,
			Kind:      gvk.Kind,
		}.Key()
		if activeKeys[key] {
			continue
		}
		objMeta := generic.ObjectMetadata{
			Namespace: obj.GetNamespace(),
			Name:      obj.GetName(),
			Group:     gvk.Group,
			Version:   gvk.Version,
			Kind:      gvk.Kind,
		}
		if err := s.deleteResource(ctx, &objMeta, sourceHub); err != nil {
			log.Errorw("failed to delete stale resource",
				"namespace", obj.GetNamespace(), "name", obj.GetName(),
				"kind", gvk.Kind, "error", err)
			deleteErrs = append(deleteErrs, fmt.Errorf("failed to delete stale %s/%s (%s): %w",
				obj.GetNamespace(), obj.GetName(), gvk.Kind, err))
		}
	}
	if len(deleteErrs) > 0 {
		return stderrors.Join(deleteErrs...)
	}
	return nil
}

// setHubAcceptsClient sets ManagedCluster.Spec.HubAcceptsClient field
// For Hub HA failover:
// - false when active hub is healthy (normal state - standby should not accept clients)
// - true when active hub becomes inactive (failover state - standby should accept clients)
func (s *HubHAStandbySyncer) setHubAcceptsClient(obj *unstructured.Unstructured, hubAcceptsClient bool) error {
	// Only process ManagedCluster resources
	gvk := obj.GroupVersionKind()
	if gvk.Group != clusterv1.GroupName || gvk.Kind != "ManagedCluster" {
		return nil
	}

	if err := unstructured.SetNestedField(obj.Object, hubAcceptsClient, "spec", "hubAcceptsClient"); err != nil {
		return fmt.Errorf("failed to set hubAcceptsClient: %w", err)
	}

	log.Debugf("adjusted ManagedCluster %s hubAcceptsClient to %v", obj.GetName(), hubAcceptsClient)
	return nil
}
