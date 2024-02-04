package event

import (
	"context"
	"errors"

	cloudevents "github.com/cloudevents/sdk-go/v2"
	"github.com/go-logr/logr"
	"github.com/stolostron/multicluster-global-hub/agent/pkg/status/controller/generic"
	"github.com/stolostron/multicluster-global-hub/pkg/bundle/event"
	"github.com/stolostron/multicluster-global-hub/pkg/bundle/metadata"
	"github.com/stolostron/multicluster-global-hub/pkg/constants"
	"github.com/stolostron/multicluster-global-hub/pkg/utils"
	corev1 "k8s.io/api/core/v1"
	ctrl "sigs.k8s.io/controller-runtime"
	"sigs.k8s.io/controller-runtime/pkg/client"
)

type replicatedPolicyEventHandler struct {
	ctx           context.Context
	log           logr.Logger
	runtimeClient client.Client
	version       *metadata.BundleVersion
	events        map[string]*event.ReplicatedPolicyEvent
}

func NewPolicyEventHandler(ctx context.Context, runtimeClient client.Client) generic.ObjectHandler {
	return &replicatedPolicyEventHandler{
		ctx:           ctx,
		log:           ctrl.Log.WithName("replicated-policy-event"),
		runtimeClient: runtimeClient,
		version:       metadata.NewBundleVersion(),
		events:        make(map[string]*event.ReplicatedPolicyEvent),
	}
}

func (h *replicatedPolicyEventHandler) Update(obj client.Object) {
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
		!utils.HasLabelKey(policy.GetLabels(), RootPolicyLabel) {
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
	h.version.Incr()
}

func (*replicatedPolicyEventHandler) Delete(client.Object) {
	// do nothing
}

func (h *replicatedPolicyEventHandler) GetVersion() *metadata.BundleVersion {
	return h.version
}

func (h *replicatedPolicyEventHandler) ToCloudEvent() *cloudevents.Event {
	if len(h.events) < 1 {
		return nil
	}
	e := cloudevents.NewEvent()
	values := []event.ReplicatedPolicyEvent{}
	for _, val := range h.events {
		values = append(values, *val)
	}
	e.SetID(values[0].PolicyID)
	e.SetType(RootEventType)
	e.SetData(cloudevents.ApplicationJSON, values)
	return &e
}

func (h *replicatedPolicyEventHandler) PostSend() {
	// update version and clean the cache
	h.version.Next()
	for key := range h.events {
		delete(h.events, key)
	}
}
