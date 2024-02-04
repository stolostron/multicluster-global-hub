package event

import (
	"context"
	"errors"

	cloudevents "github.com/cloudevents/sdk-go/v2"
	"github.com/go-logr/logr"
	corev1 "k8s.io/api/core/v1"
	ctrl "sigs.k8s.io/controller-runtime"
	"sigs.k8s.io/controller-runtime/pkg/client"

	"github.com/stolostron/multicluster-global-hub/agent/pkg/status/controller/generic"
	"github.com/stolostron/multicluster-global-hub/pkg/bundle/event"
	"github.com/stolostron/multicluster-global-hub/pkg/bundle/metadata"
	"github.com/stolostron/multicluster-global-hub/pkg/constants"
	"github.com/stolostron/multicluster-global-hub/pkg/utils"
)

var _ generic.EventEmitter = &replicatedPolicyEventEmitter{}

type replicatedPolicyEventEmitter struct {
	ctx             context.Context
	log             logr.Logger
	eventType       string
	runtimeClient   client.Client
	currentVersion  *metadata.BundleVersion
	lastSentVersion metadata.BundleVersion
	events          map[string]*event.ReplicatedPolicyEvent
}

func NewReplicatedPolicyEventEmitter(ctx context.Context, runtimeClient client.Client) generic.EventEmitter {
	return &replicatedPolicyEventEmitter{
		ctx:             ctx,
		log:             ctrl.Log.WithName("policyevent-syncer/replicatedpolicy"),
		eventType:       LocalReplicatedPolicyEventType,
		runtimeClient:   runtimeClient,
		currentVersion:  metadata.NewBundleVersion(),
		lastSentVersion: *metadata.NewBundleVersion(),
		events:          make(map[string]*event.ReplicatedPolicyEvent),
	}
}

func (h *replicatedPolicyEventEmitter) Emit() bool {
	return h.currentVersion.NewerThan(&h.lastSentVersion)
}

func (h *replicatedPolicyEventEmitter) Update(obj client.Object) {
	evt, ok := obj.(*corev1.Event)
	if !ok {
		return
	}
	// if exist, then return
	evtKey := getEventKey(evt)
	if _, ok = h.events[evtKey]; ok {
		return
	}

	// get policy
	policy, err := getInvolvePolicy(h.ctx, h.runtimeClient, evt)
	if err != nil {
		h.log.Error(err, "failed to get involved policy", "event", evt.Namespace+"/"+evt.Name)
	}

	// global resource || root policy
	if utils.HasAnnotation(policy, constants.OriginOwnerReferenceAnnotation) ||
		!utils.HasLabelKey(policy.GetLabels(), constants.PolicyEventRootPolicyNameLabelKey) {
		return
	}

	// add root policy id
	rootPolicyNamespacedName := policy.GetLabels()[constants.PolicyEventRootPolicyNameLabelKey]
	rootPolicy, err := utils.GetRootPolicy(h.ctx, h.runtimeClient, rootPolicyNamespacedName)
	if err != nil {
		h.log.Error(err, "failed to get root policy", "namespacedName", rootPolicyNamespacedName)
		return
	}

	clusterName, ok := policy.Labels[constants.PolicyEventClusterNameLabelKey]
	if !ok {
		h.log.Error(errors.New("cluster name not found in replicated policy"), "policy", policy.Namespace+"/"+policy.Name)
		return
	}
	clusterId, err := utils.GetClusterId(h.ctx, h.runtimeClient, clusterName)
	if err != nil {
		h.log.Error(err, "failed to get cluster id by cluster", "clusterName", clusterName)
		return
	}

	// update
	replicatedPolicyEvent := &event.ReplicatedPolicyEvent{
		BaseEvent: event.BaseEvent{
			EventName:      evt.Name,
			EventNamespace: evt.Namespace,
			Message:        evt.Message,
			Reason:         evt.Reason,
			Count:          evt.Count,
			Source:         evt.Source,
			CreatedAt:      evt.CreationTimestamp,
		},
		PolicyID:   string(rootPolicy.GetUID()),
		ClusterID:  clusterId,
		Compliance: policyCompliance(rootPolicy, evt),
	}
	// cache to events and update version
	h.events[evtKey] = replicatedPolicyEvent
	h.currentVersion.Incr()
}

func (*replicatedPolicyEventEmitter) Delete(client.Object) {
	// do nothing
}

func (h *replicatedPolicyEventEmitter) ToCloudEvent() *cloudevents.Event {
	if len(h.events) < 1 {
		return nil
	}
	e := cloudevents.NewEvent()
	values := []event.ReplicatedPolicyEvent{}
	for _, val := range h.events {
		values = append(values, *val)
	}
	e.SetID(values[0].PolicyID)
	e.SetType(h.eventType)
	err := e.SetData(cloudevents.ApplicationJSON, values)
	if err != nil {
		h.log.Error(err, "failed to set the payload to cloudvents.Data")
	}
	return &e
}

func (h *replicatedPolicyEventEmitter) PostSend() {
	// update version and clean the cache
	for key := range h.events {
		delete(h.events, key)
	}
	h.currentVersion.Next()
	h.lastSentVersion = *h.currentVersion
}
