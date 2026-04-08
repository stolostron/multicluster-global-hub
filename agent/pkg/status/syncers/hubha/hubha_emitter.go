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
		bundle:          generic.NewGenericBundle[*unstructured.Unstructured](),
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

	e.bundle.Update = append(e.bundle.Update, uObj)
	log.Debugf("syncing update for %s/%s (%s)", uObj.GetNamespace(), uObj.GetName(), gvk.Kind)

	return e.sendBundleUnlocked()
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

	objMeta := generic.ObjectMetadata{
		Namespace: obj.GetNamespace(),
		Name:      obj.GetName(),
		Group:     gvk.Group,
		Version:   gvk.Version,
		Kind:      gvk.Kind,
	}

	e.bundle.Delete = append(e.bundle.Delete, objMeta)
	log.Debugf("syncing delete for %s/%s (%s)", obj.GetNamespace(), obj.GetName(), gvk.Kind)

	return e.sendBundleUnlocked()
}

func (e *HubHAEmitter) Resync(_ []client.Object) error {
	e.mu.Lock()
	defer e.mu.Unlock()

	e.bundle.Clean()

	ctx := context.TODO()
	for _, gvk := range e.activeResources {
		list := &unstructured.UnstructuredList{}
		list.SetGroupVersionKind(schema.GroupVersionKind{
			Group:   gvk.Group,
			Version: gvk.Version,
			Kind:    gvk.Kind + "List",
		})

		if err := e.client.List(ctx, list); err != nil {
			if meta.IsNoMatchError(err) {
				log.Debugf("CRD not installed for %s, skipping resync", gvk.String())
				continue
			}
			log.Warnf("failed to list resources for %s during resync: %v", gvk.String(), err)
			continue
		}

		for i := range list.Items {
			obj := &list.Items[i]
			objGVK := obj.GroupVersionKind()
			if !e.resourceFilter.ShouldSyncResource(obj, objGVK) {
				continue
			}
			cleanUnstructuredMetadata(obj)
			e.bundle.Resync = append(e.bundle.Resync, obj)
		}
	}

	log.Infof("resynced %d Hub HA objects", len(e.bundle.Resync))
	return nil
}

func (e *HubHAEmitter) Send() error {
	e.mu.Lock()
	defer e.mu.Unlock()
	return e.sendBundleUnlocked()
}

func (e *HubHAEmitter) sendBundleUnlocked() error {
	if len(e.bundle.Update) == 0 && len(e.bundle.Delete) == 0 && len(e.bundle.Resync) == 0 {
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
