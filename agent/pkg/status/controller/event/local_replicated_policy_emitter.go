package event

import (
	"context"
	"fmt"
	"strings"

	cloudevents "github.com/cloudevents/sdk-go/v2"
	"github.com/go-logr/logr"
	corev1 "k8s.io/api/core/v1"
	ctrl "sigs.k8s.io/controller-runtime"
	"sigs.k8s.io/controller-runtime/pkg/client"

	"github.com/stolostron/multicluster-global-hub/agent/pkg/status/controller/config"
	"github.com/stolostron/multicluster-global-hub/agent/pkg/status/controller/filter"
	"github.com/stolostron/multicluster-global-hub/agent/pkg/status/controller/generic"
	"github.com/stolostron/multicluster-global-hub/agent/pkg/status/controller/policies"
	"github.com/stolostron/multicluster-global-hub/pkg/bundle/event"
	"github.com/stolostron/multicluster-global-hub/pkg/bundle/version"
	"github.com/stolostron/multicluster-global-hub/pkg/constants"
	"github.com/stolostron/multicluster-global-hub/pkg/enum"
	"github.com/stolostron/multicluster-global-hub/pkg/utils"
)

var _ generic.ObjectEmitter = &localReplicatedPolicyEmitter{}

// TODO: the current replicated policy event will also emit such message,
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
// message": "Policy local-policy-namespace.policy-limitrange status was updatednamespace kind-hub1-cluster1",
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

type localReplicatedPolicyEmitter struct {
	ctx             context.Context
	name            string
	log             logr.Logger
	eventType       string
	runtimeClient   client.Client
	currentVersion  *version.Version
	lastSentVersion version.Version
	payload         event.ReplicatedPolicyEventBundle
	topic           string
}

func NewLocalReplicatedPolicyEmitter(ctx context.Context, runtimeClient client.Client,
	topic string,
) generic.ObjectEmitter {
	name := strings.Replace(string(enum.LocalReplicatedPolicyEventType), enum.EventTypePrefix, "", -1)
	filter.RegisterTimeFilter(name)
	return &localReplicatedPolicyEmitter{
		ctx:             ctx,
		name:            name,
		log:             ctrl.Log.WithName(name),
		eventType:       string(enum.LocalReplicatedPolicyEventType),
		topic:           topic,
		runtimeClient:   runtimeClient,
		currentVersion:  version.NewVersion(),
		lastSentVersion: *version.NewVersion(),
		payload:         make([]event.ReplicatedPolicyEvent, 0),
	}
}

func (h *localReplicatedPolicyEmitter) PostUpdate() {
	h.currentVersion.Incr()
}

// enable local policy and is replicated policy
func (h *localReplicatedPolicyEmitter) ShouldUpdate(obj client.Object) bool {
	if config.GetEnableLocalPolicy() != config.EnableLocalPolicyTrue {
		return false
	}
	policy, ok := policyEventPredicate(h.ctx, obj, h.runtimeClient, h.log)

	return ok && !utils.HasAnnotation(policy, constants.OriginOwnerReferenceAnnotation) &&
		utils.HasItemKey(policy.GetLabels(), constants.PolicyEventRootPolicyNameLabelKey)
}

func (h *localReplicatedPolicyEmitter) ShouldSend() bool {
	return h.currentVersion.NewerThan(&h.lastSentVersion)
}

func (h *localReplicatedPolicyEmitter) Topic() string {
	return h.topic
}

func (h *localReplicatedPolicyEmitter) Update(obj client.Object) bool {
	evt, ok := obj.(*corev1.Event)
	if !ok {
		return false
	}
	// if it's a older event, then return false
	if !filter.Newer(h.name, evt.LastTimestamp.Time) {
		return false
	}

	// get policy
	policy, err := getInvolvePolicy(h.ctx, h.runtimeClient, evt)
	if err != nil {
		h.log.Error(err, "failed to get involved policy", "event", evt.Namespace+"/"+evt.Name,
			"policy", evt.InvolvedObject.Namespace+"/"+evt.InvolvedObject.Name)
		return false
	}

	rootPolicy, clusterID, err := policies.GetRootPolicyAndClusterID(h.ctx, policy, h.runtimeClient)
	if err != nil {
		h.log.Error(err, "failed to get get rootPolicy/clusterID by replicatedPolicy")
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
			CreatedAt:      evt.LastTimestamp,
		},
		PolicyID:   string(rootPolicy.GetUID()),
		ClusterID:  clusterID,
		Compliance: policyCompliance(rootPolicy, evt),
	}
	// cache to events and update version
	h.payload = append(h.payload, replicatedPolicyEvent)
	return true
}

func (*localReplicatedPolicyEmitter) Delete(client.Object) bool {
	// do nothing
	return false
}

func (h *localReplicatedPolicyEmitter) ToCloudEvent() (*cloudevents.Event, error) {
	if len(h.payload) < 1 {
		return nil, fmt.Errorf("the cloudevent instance shouldn't be nil")
	}
	e := cloudevents.NewEvent()
	e.SetType(h.eventType)
	e.SetSource(config.GetLeafHubName())
	e.SetExtension(version.ExtVersion, h.currentVersion.String())
	err := e.SetData(cloudevents.ApplicationJSON, h.payload)
	return &e, err
}

func (h *localReplicatedPolicyEmitter) PostSend() {
	// update the time filter: with latest event
	for _, evt := range h.payload {
		filter.CacheTime(h.name, evt.CreatedAt.Time)
	}
	// update version and clean the cache
	h.payload = make([]event.ReplicatedPolicyEvent, 0)
	h.currentVersion.Next()
	h.lastSentVersion = *h.currentVersion
}
