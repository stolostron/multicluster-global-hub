// Copyright (c) 2025 Red Hat, Inc.
// Copyright Contributors to the Open Cluster Management project

package hubha

import (
	"context"
	"fmt"

	cloudevents "github.com/cloudevents/sdk-go/v2"
	"k8s.io/apimachinery/pkg/api/errors"
	"k8s.io/apimachinery/pkg/apis/meta/v1/unstructured"
	"k8s.io/apimachinery/pkg/runtime/schema"
	"k8s.io/client-go/util/retry"
	"sigs.k8s.io/controller-runtime/pkg/client"
	"sigs.k8s.io/controller-runtime/pkg/controller/controllerutil"

	"github.com/stolostron/multicluster-global-hub/pkg/bundle/generic"
	"github.com/stolostron/multicluster-global-hub/pkg/constants"
)

// HubHAStandbySyncer receives ACM resources from active hub and applies them to standby hub
type HubHAStandbySyncer struct {
	client client.Client
}

func NewHubHAStandbySyncer(c client.Client) *HubHAStandbySyncer {
	return &HubHAStandbySyncer{
		client: c,
	}
}

// Sync processes CloudEvents containing Hub HA resources from active hub
func (s *HubHAStandbySyncer) Sync(ctx context.Context, evt *cloudevents.Event) error {
	// Only process Hub HA resource events
	if evt.Type() != constants.HubHAResourcesMsgKey {
		return nil
	}

	log.Infof("standby hub received Hub HA resources from active hub: %s", evt.Source())

	// Unmarshal the bundle
	bundle := generic.NewGenericBundle[*unstructured.Unstructured]()
	if err := evt.DataAs(bundle); err != nil {
		return fmt.Errorf("failed to unmarshal Hub HA resource bundle: %w", err)
	}

	sourceHub := evt.Source()

	// Apply created resources
	for _, obj := range bundle.Create {
		if err := s.createResource(ctx, obj, sourceHub); err != nil {
			log.Errorf("failed to create resource %s/%s from active hub %s: %v",
				obj.GetNamespace(), obj.GetName(), sourceHub, err)
			// Continue with other resources instead of failing entirely
		}
	}

	// Apply updated resources
	for _, obj := range bundle.Update {
		if err := s.updateResource(ctx, obj, sourceHub); err != nil {
			log.Errorf("failed to update resource %s/%s from active hub %s: %v",
				obj.GetNamespace(), obj.GetName(), sourceHub, err)
		}
	}

	// Handle resync (same as create/update)
	for _, obj := range bundle.Resync {
		if err := s.updateResource(ctx, obj, sourceHub); err != nil {
			log.Errorf("failed to resync resource %s/%s from active hub %s: %v",
				obj.GetNamespace(), obj.GetName(), sourceHub, err)
		}
	}

	// Handle deleted resources
	for _, meta := range bundle.Delete {
		if err := s.deleteResource(ctx, &meta, sourceHub); err != nil {
			log.Errorf("failed to delete resource %s/%s from active hub %s: %v",
				meta.Namespace, meta.Name, sourceHub, err)
		}
	}

	log.Infof("standby hub processed Hub HA bundle from %s: created=%d, updated=%d, resynced=%d, deleted=%d",
		sourceHub, len(bundle.Create), len(bundle.Update), len(bundle.Resync), len(bundle.Delete))

	return nil
}

func (s *HubHAStandbySyncer) createResource(ctx context.Context, obj *unstructured.Unstructured, sourceHub string) error {
	log.Infof("creating resource from active hub %s: %s/%s (%s)",
		sourceHub, obj.GetNamespace(), obj.GetName(), obj.GetKind())

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

func (s *HubHAStandbySyncer) updateResource(ctx context.Context, obj *unstructured.Unstructured, sourceHub string) error {
	log.Debugf("updating resource from active hub %s: %s/%s (%s)",
		sourceHub, obj.GetNamespace(), obj.GetName(), obj.GetKind())

	err := retry.RetryOnConflict(retry.DefaultRetry, func() error {
		existing := &unstructured.Unstructured{}
		existing.SetGroupVersionKind(obj.GroupVersionKind())

		if err := s.client.Get(ctx, client.ObjectKeyFromObject(obj), existing); err != nil {
			// Resource doesn't exist, create it
			return s.client.Create(ctx, obj)
		}

		// Preserve some metadata from existing resource
		obj.SetResourceVersion(existing.GetResourceVersion())
		obj.SetUID(existing.GetUID())

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
