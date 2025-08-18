package handlers

import (
	"context"
	"strings"

	corev1 "k8s.io/api/core/v1"
	"sigs.k8s.io/controller-runtime/pkg/client"

	"github.com/stolostron/multicluster-global-hub/agent/pkg/status/filter"
	"github.com/stolostron/multicluster-global-hub/agent/pkg/status/generic"
	"github.com/stolostron/multicluster-global-hub/agent/pkg/status/interfaces"
	"github.com/stolostron/multicluster-global-hub/agent/pkg/status/syncers/configmap"
	policyhandler "github.com/stolostron/multicluster-global-hub/agent/pkg/status/syncers/policies/handlers"
	"github.com/stolostron/multicluster-global-hub/pkg/bundle/event"
	"github.com/stolostron/multicluster-global-hub/pkg/constants"
	"github.com/stolostron/multicluster-global-hub/pkg/enum"
	"github.com/stolostron/multicluster-global-hub/pkg/logger"
	"github.com/stolostron/multicluster-global-hub/pkg/utils"
)

// the current replicated policy event will also emit such message,
// it has contain concrete reason why the state of the compliance change to another.
// I will disable the replicated policy event until it contain some valuable message.
// disable it by setting the emit() return false
//
//	{
//	  "specversion": "1.0",
//	  "id": "9ff85324-a1a3-44c1-9dbf-e965cbee507c",
//	  "source": "kind-hub1",
//	  "type": "io.open-cluster-management.operator.multiclusterglobalhubs.local.replicatedpolicy.update",
//	  "datacontenttype": "application/json",
//	  "time": "2024-02-04T08:02:30.670142334Z",
//	  "data": [
//	    {
//	      "eventName": "local-policy-namespace.policy-limitrange.17b098ec20742ecc",
//	      "eventNamespace": "kind-hub1-cluster1",
//	      "message": "Policy local-policy-namespace.policy-limitrange status was updated in cluster
//
// message": "Policy local-policy-namespace.policy-limitrange status was updatednamespace kind-hub1-cluster1",
//
//	      "reason": "PolicyStatusSync",
//	      "count": 2,
//	      "source": {
//	        "component": "policy-status-sync"
//	      },
//	      "createdAt": "2024-02-04T07:39:58Z",
//	      "policyId": "9ff85324-a1a3-44c1-9dbf-e965cbee507c",
//	      "clusterId": "cef103c3-fe2c-4fbc-a3fb-a96492caa049",
//	      "compliance": "NonCompliant"
//	    }
//	  ],
//	  "kafkapartition": "0",
//	  "kafkatopic": "event",
//	  "kafkamessagekey": "kind-hub1",
//	  "kafkaoffset": "13"
//	}
var log = logger.DefaultZapLogger()

func NewReplicatedPolicyEventEmitter(eventType enum.EventType) interfaces.Emitter {
	name := strings.ReplaceAll(string(eventType), enum.EventTypePrefix, "")
	return generic.NewGenericEmitter(eventType, generic.WithPostSend(
		// After sending the event, update the filter cache and clear the bundle from the handler cache.
		func(data interface{}) {
			events, ok := data.(*event.ReplicatedPolicyEventBundle)
			if !ok {
				return
			}
			// update the time filter: with latest event
			for _, evt := range *events {
				filter.CacheTime(name, evt.CreatedAt.Time)
			}
			// reset the payload
			*events = (*events)[:0]
		}),
	)
}

type localReplicatedPolicyEventHandler struct {
	ctx           context.Context
	name          string
	eventType     string
	runtimeClient client.Client
	payload       *event.ReplicatedPolicyEventBundle
}

func NewLocalReplicatedPolicyEventHandler(ctx context.Context, c client.Client) interfaces.Handler {
	name := strings.ReplaceAll(string(enum.LocalReplicatedPolicyEventType), enum.EventTypePrefix, "")
	filter.RegisterTimeFilter(name)
	return &localReplicatedPolicyEventHandler{
		ctx:           ctx,
		name:          name,
		eventType:     string(enum.LocalReplicatedPolicyEventType),
		runtimeClient: c,
		payload:       &event.ReplicatedPolicyEventBundle{},
	}
}

func (h *localReplicatedPolicyEventHandler) Get() interface{} {
	return h.payload
}

// enable local policy and is replicated policy
func (h *localReplicatedPolicyEventHandler) shouldUpdate(obj client.Object) bool {
	if configmap.GetEnableLocalPolicy() != configmap.EnableLocalPolicyTrue {
		return false
	}
	policy, ok := policyEventPredicate(h.ctx, h.name, obj, h.runtimeClient)

	return ok && !utils.HasAnnotation(policy, constants.OriginOwnerReferenceAnnotation) &&
		utils.HasItemKey(policy.GetLabels(), constants.PolicyEventRootPolicyNameLabelKey)
}

func (h *localReplicatedPolicyEventHandler) Update(obj client.Object) bool {
	if !h.shouldUpdate(obj) {
		return false
	}

	evt, ok := obj.(*corev1.Event)
	if !ok {
		return false
	}

	// get policy
	policy, err := getInvolvePolicy(h.ctx, h.runtimeClient, evt)
	if err != nil {
		log.Errorf("failed to get involved policy event: %s/%s, involved: %s/%s, error: %v", evt.Namespace, evt.Name,
			evt.InvolvedObject.Namespace, evt.InvolvedObject.Name, err)
		return false
	}

	rootPolicy, clusterID, clusterName, err := policyhandler.GetRootPolicyAndClusterInfo(h.ctx, policy, h.runtimeClient)
	if err != nil {
		log.Errorf("failed to get rootPolicy/clusterID/clusterName from the replicatedPolicy: %v", err)
		return false
	}
	// update
	replicatedPolicyEvent := event.ReplicatedPolicyEvent{
		BaseEvent: event.BaseEvent{
			EventName:      evt.Name,
			EventNamespace: evt.Namespace,
			Message:        evt.Message,
			Reason:         evt.Reason,
			Count:          evt.Count,
			Source:         evt.Source,
			CreatedAt:      evt.CreationTimestamp,
		},
		PolicyID:    string(rootPolicy.GetUID()),
		ClusterID:   clusterID,
		ClusterName: clusterName,
		Compliance:  policyCompliance(rootPolicy, evt),
	}
	// cache to events and update version
	*h.payload = append(*h.payload, replicatedPolicyEvent)
	return true
}

func (*localReplicatedPolicyEventHandler) Delete(client.Object) bool {
	// do nothing
	return false
}
