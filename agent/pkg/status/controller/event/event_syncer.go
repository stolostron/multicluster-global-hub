package event

import (
	"context"
	"errors"
	"fmt"
	"sync"
	"time"

	corev1 "k8s.io/api/core/v1"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	ctrl "sigs.k8s.io/controller-runtime"

	"github.com/go-logr/logr"
	"github.com/stolostron/multicluster-global-hub/agent/pkg/status/controller/config"
	"github.com/stolostron/multicluster-global-hub/agent/pkg/status/controller/generic"
	"github.com/stolostron/multicluster-global-hub/pkg/bundle"
	"github.com/stolostron/multicluster-global-hub/pkg/bundle/grc"
	"github.com/stolostron/multicluster-global-hub/pkg/bundle/metadata"
	"github.com/stolostron/multicluster-global-hub/pkg/constants"
	"github.com/stolostron/multicluster-global-hub/pkg/transport"
	"github.com/stolostron/multicluster-global-hub/pkg/utils"
	"sigs.k8s.io/controller-runtime/pkg/client"
	"sigs.k8s.io/controller-runtime/pkg/predicate"
)

type BaseEvent struct {
	EventName      string             `json:"eventName"`
	EventNamespace string             `json:"eventNamespace"`
	Message        string             `json:"message,omitempty"`
	Reason         string             `json:"reason,omitempty"`
	Count          int                `json:"count,omitempty"`
	Source         corev1.EventSource `json:"source,omitempty"`
	CreatedAt      metav1.Time        `json:"createdAt,omitempty"`
}

type RootPolicyEvent struct {
	BaseEvent
	PolicyID   string `json:"policyId"`
	Compliance string `json:"compliance"`
}

type ReplicatedPolicyEvent struct {
	BaseEvent
	PolicyID   string `json:"policyId"`
	ClusterID  string `json:"clusterId"`
	Compliance string `json:"compliance"`
}

type EventEntry struct {
	eventType       string
	events          []*corev1.Event
	eventPredicate  func() bool
	lastSentVersion metadata.BundleVersion // not pointer so it does not point to the bundle's internal version
}

type eventSyncer struct {
	log                     logr.Logger
	client                  client.Client
	producer                transport.Producer
	topic                   string
	orderedBundleCollection []*generic.BundleEntry
	intervalFunc            func() time.Duration
	startOnce               sync.Once
	lock                    sync.Mutex
}

// AddPolicyStatusSyncer adds policies status controller to the manager.
func AddEventSyncer(mgr ctrl.Manager, producer transport.Producer) (*generic.HybridSyncManager, error) {

	pred := predicate.NewPredicateFuncs(func(object client.Object) bool {
		return true
	})
	syncer := &eventSyncer{
		log:      ctrl.Log.WithName("event-syncer"),
		client:   mgr.GetClient(),
		producer: producer,
		topic:    "event",
	}

	syncer.startOnce.Do(func() {
		// go c.periodicSync()
	})
	err := ctrl.NewControllerManagedBy(mgr).For(&corev1.Event{}).WithEventFilter(pred).Complete(syncer)
	if err != nil {

	}

	leafHubName := config.GetLeafHubName()
	bundleCollection, err := createBundleCollection(producer, leafHubName)
	if err != nil {
		return nil, fmt.Errorf("failed to add policies controller to the manager - %w", err)
	}

	rootPolicyPredicate := predicate.NewPredicateFuncs(func(object client.Object) bool {
		return !utils.HasLabelKey(object.GetLabels(), rootPolicyLabel)
	})

	ownerRefAnnotationPredicate := predicate.NewPredicateFuncs(func(object client.Object) bool {
		return utils.HasAnnotation(object, constants.OriginOwnerReferenceAnnotation)
	})

	createObjFunction := func() bundle.Object { return &corev1.Event{} }

	if err := generic.NewGenericStatusSyncer(mgr, policiesStatusSyncLog, producer, bundleCollection, createObjFunction,
		predicate.And(rootPolicyPredicate, ownerRefAnnotationPredicate), config.GetPolicyDuration); err != nil {
		return hybridSyncManager, fmt.Errorf("failed to add policies controller to the manager - %w", err)
	}
	return hybridSyncManager, nil
}

func (c *eventSyncer) Reconcile(ctx context.Context, request ctrl.Request) (ctrl.Result, error) {
	reqLogger := c.log.WithValues("Namespace", request.Namespace, "Name", request.Name)

	object := c.createBundleObjFunc()
	if err := c.client.Get(ctx, request.NamespacedName, object); apierrors.IsNotFound(err) {
		// the instance was deleted and it had no finalizer on it.
		// for the local resources, there is no finalizer so we need to delete the object from the bundle
		object.SetNamespace(request.Namespace)
		object.SetName(request.Name)
		if e := c.deleteObjectAndFinalizer(ctx, object, reqLogger); e != nil {
			return ctrl.Result{Requeue: true, RequeueAfter: REQUEUE_PERIOD}, e
		}
		return ctrl.Result{}, nil
	} else if err != nil {
		return ctrl.Result{Requeue: true, RequeueAfter: REQUEUE_PERIOD},
			fmt.Errorf("reconciliation failed: %w", err)
	}

	if c.isObjectBeingDeleted(object) {
		if err := c.deleteObjectAndFinalizer(ctx, object, reqLogger); err != nil {
			return ctrl.Result{Requeue: true, RequeueAfter: REQUEUE_PERIOD}, err
		}
	} else { // otherwise, the object was not deleted and no error occurred
		if err := c.updateObjectAndFinalizer(ctx, object, reqLogger); err != nil {
			return ctrl.Result{Requeue: true, RequeueAfter: REQUEUE_PERIOD}, err
		}
	}

	reqLogger.V(2).Info("Reconciliation complete.")

	return ctrl.Result{}, nil
}

func createBundleCollection(pro transport.Producer, leafHubName string) (
	[]*generic.BundleEntry, *generic.HybridSyncManager, error,
) {
	// clusters per policy (base bundle)
	complianceTransportKey := fmt.Sprintf("%s.%s", leafHubName, constants.ComplianceMsgKey)
	complianceBundle := grc.NewAgentComplianceBundle(leafHubName, extractPolicyID)

	// minimal compliance status bundle
	minimalComplianceTransportKey := fmt.Sprintf("%s.%s", leafHubName,
		constants.MinimalComplianceMsgKey)
	minimalComplianceBundle := grc.NewAgentMinimalComplianceBundle(leafHubName)

	fullStatusPredicate := func() bool { return config.GetAggregationLevel() == config.AggregationFull }
	minimalStatusPredicate := func() bool {
		return config.GetAggregationLevel() == config.AggregationMinimal
	}

	// apply a hybrid sync manager on the (full aggregation) compliance bundles
	// completeComplianceStatusBundleCollectionEntry, deltaComplianceStatusBundleCollectionEntry,
	hybridSyncManager, err := getHybridSyncManager(pro, leafHubName, fullStatusPredicate,
		complianceBundle)
	if err != nil {
		return nil, nil, fmt.Errorf("failed to initialize hybrid sync manager - %w", err)
	}

	// no need to send in the same cycle both clusters per policy and compliance. if CpP was sent, don't send compliance
	return []*generic.BundleEntry{ // multiple bundles for policy status
		generic.NewBundleEntry(complianceTransportKey, complianceBundle, fullStatusPredicate),
		hybridSyncManager.GetBundleCollectionEntry(metadata.CompleteStateMode),
		// hybridSyncManager.GetBundleCollectionEntry(genericbundle.DeltaStateMode),
		generic.NewBundleEntry(minimalComplianceTransportKey, minimalComplianceBundle, minimalStatusPredicate),
	}, hybridSyncManager, nil
}

// getHybridComplianceBundleCollectionEntries creates a complete/delta compliance bundle collection entries and has
// them managed by a genericHybridSyncManager.
// The collection entries are returned (or nils with an error if any occurred).
func getHybridSyncManager(producer transport.Producer, leafHubName string,
	fullStatusPredicate func() bool, complianceBundle bundle.AgentBundle,
) (*generic.HybridSyncManager, error) {
	// complete compliance status bundle
	completeComplianceStatusTransportKey := fmt.Sprintf("%s.%s", leafHubName,
		constants.CompleteComplianceMsgKey)
	completeComplianceStatusBundle := grc.NewAgentCompleteComplianceBundle(leafHubName, complianceBundle,
		extractPolicyID)

	// delta compliance status bundle
	deltaComplianceStatusTransportKey := fmt.Sprintf("%s.%s", leafHubName, constants.DeltaComplianceMsgKey)
	deltaComplianceStatusBundle := grc.NewAgentDeltaComplianceBundle(leafHubName, completeComplianceStatusBundle,
		complianceBundle.(*grc.ComplianceBundle), extractPolicyID)

	completeComplianceBundleCollectionEntry := generic.NewBundleEntry(completeComplianceStatusTransportKey,
		completeComplianceStatusBundle, fullStatusPredicate)
	deltaComplianceBundleCollectionEntry := generic.NewBundleEntry(deltaComplianceStatusTransportKey,
		deltaComplianceStatusBundle, fullStatusPredicate)

	hybridSyncManager, err := generic.NewHybridSyncManager(
		ctrl.Log.WithName("compliance-status-hybrid-sync-manager"),
		completeComplianceBundleCollectionEntry,
		deltaComplianceBundleCollectionEntry)
	if err != nil {
		return nil, fmt.Errorf("%w: %v", err, errors.New(
			"failed to create hybrid sync manager"))
	}
	return hybridSyncManager, nil
}

func extractPolicyID(obj bundle.Object) (string, bool) {
	val, ok := obj.GetAnnotations()[constants.OriginOwnerReferenceAnnotation]
	return val, ok
}
