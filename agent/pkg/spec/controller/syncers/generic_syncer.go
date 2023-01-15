package syncers

import (
	"context"
	"encoding/json"
	"sync"

	"github.com/go-logr/logr"
	"k8s.io/apimachinery/pkg/apis/meta/v1/unstructured"
	ctrl "sigs.k8s.io/controller-runtime"
	"sigs.k8s.io/controller-runtime/pkg/client"

	"github.com/stolostron/multicluster-global-hub/agent/pkg/helper"
	"github.com/stolostron/multicluster-global-hub/agent/pkg/spec/bundle"
	"github.com/stolostron/multicluster-global-hub/agent/pkg/spec/controller/rbac"
	"github.com/stolostron/multicluster-global-hub/agent/pkg/spec/controller/workers"
	"github.com/stolostron/multicluster-global-hub/pkg/transport"
)

// genericBundleSyncer syncs objects spec from received bundles.
type genericBundleSyncer struct {
	log                          logr.Logger
	consumer                     transport.Consumer
	workerPool                   *workers.WorkerPool
	bundleProcessingWaitingGroup sync.WaitGroup
	enforceHohRbac               bool
	payloadChannel               chan []byte
}

func NewGenericSyncer(consumer transport.Consumer, workerPool *workers.WorkerPool, enforceRbac bool) *genericBundleSyncer {
	return &genericBundleSyncer{
		log:                          ctrl.Log.WithName("generic-bundle-syncer"),
		consumer:                     consumer,
		workerPool:                   workerPool,
		bundleProcessingWaitingGroup: sync.WaitGroup{},
		enforceHohRbac:               enforceRbac,
		payloadChannel:               make(chan []byte),
	}
}

// Start function starts bundles spec syncer.
func (syncer *genericBundleSyncer) Start(ctx context.Context) error {
	syncer.log.Info("started bundles syncer...")

	go syncer.sync(ctx)

	<-ctx.Done() // blocking wait for stop event

	close(syncer.payloadChannel)
	syncer.log.Info("stopped bundles syncer")

	return nil
}

func (syncer *genericBundleSyncer) Channel() chan []byte {
	return syncer.payloadChannel
}

func (syncer *genericBundleSyncer) sync(ctx context.Context) {
	for {
		select {
		case <-ctx.Done(): // we have received a signal to stop
			return

		case messagePayload := <-syncer.payloadChannel:
			genericBundle := bundle.NewGenericBundle()
			if err := json.Unmarshal(messagePayload, genericBundle); err != nil {
				syncer.log.Error(err, "parse generic bundle error")
				continue
			}
			syncer.bundleProcessingWaitingGroup.Add(len(genericBundle.Objects) + len(genericBundle.DeletedObjects))
			syncer.syncObjects(genericBundle.Objects)
			syncer.syncDeletedObjects(genericBundle.DeletedObjects)
			syncer.bundleProcessingWaitingGroup.Wait()
		}
	}
}

func (syncer *genericBundleSyncer) syncObjects(bundleObjects []*unstructured.Unstructured) {
	for _, bundleObject := range bundleObjects {
		if !syncer.enforceHohRbac { // if rbac not enforced, use controller's identity.
			bundleObject = syncer.anonymize(bundleObject) // anonymize removes the user identity from the obj if exists
		}

		syncer.workerPool.Submit(workers.NewJob(bundleObject, func(ctx context.Context,
			k8sClient client.Client, obj interface{},
		) {
			defer syncer.bundleProcessingWaitingGroup.Done()

			unstructuredObject, _ := obj.(*unstructured.Unstructured)

			if !syncer.enforceHohRbac { // if rbac not enforced, create missing namespaces.
				if err := helper.CreateNamespaceIfNotExist(ctx, k8sClient,
					unstructuredObject.GetNamespace()); err != nil {
					syncer.log.Error(err, "failed to create namespace",
						"namespace", unstructuredObject.GetNamespace())
					return
				}
			}

			err := helper.UpdateObject(ctx, k8sClient, unstructuredObject)
			if err != nil {
				syncer.log.Error(err, "failed to update object", "name", unstructuredObject.GetName(),
					"namespace", unstructuredObject.GetNamespace(), "kind", unstructuredObject.GetKind())
				return
			}
			syncer.log.Info("object updated", "name", unstructuredObject.GetName(), "namespace",
				unstructuredObject.GetNamespace(), "kind", unstructuredObject.GetKind())
		}))
	}
}

func (syncer *genericBundleSyncer) syncDeletedObjects(deletedObjects []*unstructured.Unstructured) {
	for _, deletedBundleObj := range deletedObjects {
		if !syncer.enforceHohRbac { // if rbac not enforced, use controller's identity.
			deletedBundleObj = syncer.anonymize(deletedBundleObj) // anonymize removes the user identity from the obj if exists
		}

		syncer.workerPool.Submit(workers.NewJob(deletedBundleObj, func(ctx context.Context,
			k8sClient client.Client, obj interface{},
		) {
			defer syncer.bundleProcessingWaitingGroup.Done()

			unstructuredObject, _ := obj.(*unstructured.Unstructured)

			// syncer.deleteObject(ctx, k8sClient, obj.(*unstructured.Unstructured))
			if deleted, err := helper.DeleteObject(ctx, k8sClient, unstructuredObject); err != nil {
				syncer.log.Error(err, "failed to delete object", "name",
					unstructuredObject.GetName(), "namespace",
					unstructuredObject.GetNamespace(), "kind", unstructuredObject.GetKind())
			} else if deleted {
				syncer.log.Info("object deleted", "name", unstructuredObject.GetName(),
					"namespace", unstructuredObject.GetNamespace(), "kind", unstructuredObject.GetKind())
			}
		}))
	}
}

func (syncer *genericBundleSyncer) anonymize(obj *unstructured.Unstructured) *unstructured.Unstructured {
	annotations := obj.GetAnnotations()
	delete(annotations, rbac.UserIdentityAnnotation)
	delete(annotations, rbac.UserGroupsAnnotation)
	obj.SetAnnotations(annotations)
	return obj
}
