// Copyright (c) 2025 Red Hat, Inc.
// Copyright Contributors to the Open Cluster Management project

package hubha

import (
	"context"
	"encoding/json"
	"fmt"
	"sync"

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

type HubHAEmitter struct {
	producer        transport.Producer
	transportConfig *transport.TransportInternalConfig
	activeHubName   string
	standbyHubName  string
	client          client.Client
	activeResources []schema.GroupVersionKind
	resourceFilter  *utils.HubHAResourceFilter
	bundle          *generic.GenericBundle[*unstructured.Unstructured]
	mu              sync.Mutex
}

func NewHubHAEmitter(
	producer transport.Producer,
	transportConfig *transport.TransportInternalConfig,
	activeHubName string,
	standbyHubName string,
	c client.Client,
) *HubHAEmitter {
	return &HubHAEmitter{
		producer:        producer,
		transportConfig: transportConfig,
		activeHubName:   activeHubName,
		standbyHubName:  standbyHubName,
		client:          c,
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

func (e *HubHAEmitter) SetActiveResources(resources []schema.GroupVersionKind) {
	e.mu.Lock()
	defer e.mu.Unlock()
	e.activeResources = resources
}

func (e *HubHAEmitter) SetStandbyHub(standbyHub string) {
	e.mu.Lock()
	defer e.mu.Unlock()
	e.standbyHubName = standbyHub
}

func (e *HubHAEmitter) EventType() string {
	return constants.HubHAResourcesMsgKey
}

func (e *HubHAEmitter) Predicate() predicate.Predicate {
	return predicate.NewPredicateFuncs(func(obj client.Object) bool {
		uObj, err := toUnstructured(obj)
		if err != nil {
			log.Errorf("failed to convert object to unstructured in predicate: %v", err)
			return false
		}
		gvk := uObj.GroupVersionKind()
		return e.resourceFilter.ShouldSyncResource(obj, gvk)
	})
}

func (e *HubHAEmitter) Update(obj client.Object) error {
	e.mu.Lock()
	defer e.mu.Unlock()

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
		if err := e.sendBundleUnlocked(); err != nil {
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

	log.Debugf("bundled update for %s/%s (%s)", uObj.GetNamespace(), uObj.GetName(), gvk.Kind)
	return nil
}

func (e *HubHAEmitter) Delete(obj client.Object) error {
	e.mu.Lock()
	defer e.mu.Unlock()

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
		if err := e.sendBundleUnlocked(); err != nil {
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

	log.Debugf("bundled delete for %s/%s (%s)", obj.GetNamespace(), obj.GetName(), gvk.Kind)
	return nil
}

func (e *HubHAEmitter) Resync(_ []client.Object) error {
	e.mu.Lock()
	defer e.mu.Unlock()

	e.bundle.Clean()
	totalObjects := 0

	for _, gvk := range e.activeResources {
		list := &unstructured.UnstructuredList{}
		list.SetGroupVersionKind(schema.GroupVersionKind{
			Group:   gvk.Group,
			Version: gvk.Version,
			Kind:    gvk.Kind + "List",
		})

		if err := e.client.List(context.TODO(), list); err != nil {
			if meta.IsNoMatchError(err) {
				log.Debugf("CRD not installed for %s, skipping resync", gvk.String())
				continue
			}
			log.Warnf("failed to list resources for %s during resync: %v", gvk.String(), err)
			continue
		}

		var metadataList []generic.ObjectMetadata
		for i := range list.Items {
			obj := &list.Items[i]
			if !e.resourceFilter.ShouldSyncResource(obj, gvk) {
				continue
			}
			cleanUnstructuredMetadata(obj)

			added, err := e.bundle.AddResync(obj)
			if err != nil {
				return fmt.Errorf("failed to add resync object %s/%s (%s): %w",
					obj.GetNamespace(), obj.GetName(), gvk.Kind, err)
			}
			if !added {
				if err := e.sendBundleUnlocked(); err != nil {
					return err
				}
				added, err = e.bundle.AddResync(obj)
				if err != nil {
					return fmt.Errorf("failed to add resync object %s/%s (%s) after flush: %w",
						obj.GetNamespace(), obj.GetName(), gvk.Kind, err)
				}
				if !added {
					return fmt.Errorf("resync object too large: %s/%s (%s)",
						obj.GetNamespace(), obj.GetName(), gvk.Kind)
				}
			}

			metadataList = append(metadataList, generic.ObjectMetadata{
				Namespace: obj.GetNamespace(),
				Name:      obj.GetName(),
				Group:     gvk.Group,
				Version:   gvk.Version,
				Kind:      gvk.Kind,
			})
			totalObjects++
		}

		if len(metadataList) == 0 {
			continue
		}

		if err := e.sendBundleUnlocked(); err != nil {
			return err
		}
		if err := e.bundle.AddResyncMetadata(metadataList); err != nil {
			return fmt.Errorf("resync metadata too large for %s: %w", gvk.Kind, err)
		}
		if err := e.sendBundleUnlocked(); err != nil {
			return err
		}
	}

	log.Infof("resynced %d Hub HA objects", totalObjects)
	return nil
}

func (e *HubHAEmitter) Send() error {
	e.mu.Lock()
	defer e.mu.Unlock()
	return e.sendBundleUnlocked()
}

func (e *HubHAEmitter) sendBundleUnlocked() error {
	if e.bundle.IsEmpty() {
		return nil
	}

	evt := utils.ToCloudEvent(
		constants.HubHAResourcesMsgKey,
		e.activeHubName,
		e.standbyHubName,
		e.bundle,
	)

	ctx := context.TODO()
	topicCtx := cecontext.WithTopic(ctx, e.transportConfig.KafkaCredential.SpecTopic)
	if err := e.producer.SendEvent(topicCtx, evt); err != nil {
		return fmt.Errorf("failed to send Hub HA bundle from %s to %s: %w",
			e.activeHubName, e.standbyHubName, err)
	}

	log.Infof("sent Hub HA bundle: updates=%d, deletes=%d, resync=%d",
		len(e.bundle.Update), len(e.bundle.Delete), len(e.bundle.Resync))

	e.bundle.Clean()
	return nil
}

func toUnstructured(obj client.Object) (*unstructured.Unstructured, error) {
	if uObj, ok := obj.(*unstructured.Unstructured); ok {
		return uObj.DeepCopy(), nil
	}

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

func cleanUnstructuredMetadata(obj *unstructured.Unstructured) {
	obj.SetManagedFields(nil)
	obj.SetResourceVersion("")
	obj.SetGeneration(0)
	obj.SetUID("")
}
