package event

import (
	"context"

	cloudevents "github.com/cloudevents/sdk-go/v2"
	"github.com/go-logr/logr"
	lru "github.com/hashicorp/golang-lru"
	corev1 "k8s.io/api/core/v1"
	ctrl "sigs.k8s.io/controller-runtime"
	"sigs.k8s.io/controller-runtime/pkg/client"

	"github.com/stolostron/multicluster-global-hub/agent/pkg/status/controller/config"
	"github.com/stolostron/multicluster-global-hub/agent/pkg/status/controller/generic"
	localpolicies "github.com/stolostron/multicluster-global-hub/agent/pkg/status/controller/local_policies"
	"github.com/stolostron/multicluster-global-hub/pkg/bundle/event"
	"github.com/stolostron/multicluster-global-hub/pkg/bundle/metadata"
	"github.com/stolostron/multicluster-global-hub/pkg/constants"
	"github.com/stolostron/multicluster-global-hub/pkg/enum"
	"github.com/stolostron/multicluster-global-hub/pkg/transport"
	"github.com/stolostron/multicluster-global-hub/pkg/utils"
)

var _ generic.EventEmitter = &localReplicatedPolicyEmitter{}

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
	log             logr.Logger
	eventType       string
	runtimeClient   client.Client
	currentVersion  *metadata.BundleVersion
	lastSentVersion metadata.BundleVersion
	events          []event.ReplicatedPolicyEvent
	cache           *lru.Cache
	topic           string
}

func NewLocalReplicatedPolicyEmitter(ctx context.Context, runtimeClient client.Client) generic.EventEmitter {
	cache, _ := lru.New(20)
	return &localReplicatedPolicyEmitter{
		ctx:             ctx,
		log:             ctrl.Log.WithName("policy-event-syncer/replicated-policy"),
		eventType:       string(enum.LocalReplicatedPolicyEventType),
		topic:           transport.GenericEventTopic,
		runtimeClient:   runtimeClient,
		currentVersion:  metadata.NewBundleVersion(),
		lastSentVersion: *metadata.NewBundleVersion(),
		cache:           cache,
		events:          make([]event.ReplicatedPolicyEvent, 0),
	}
}

func (h *localReplicatedPolicyEmitter) Emit() bool {
	return config.GetEnableLocalPolicy() == config.EnableLocalPolicyTrue &&
		h.currentVersion.NewerThan(&h.lastSentVersion)
}

func (h *localReplicatedPolicyEmitter) Topic() string {
	return h.topic
}

func (h *localReplicatedPolicyEmitter) Update(obj client.Object) {
	evt, ok := obj.(*corev1.Event)
	if !ok {
		return
	}
	// if exist, then return
	evtKey := getEventKey(evt)
	if h.cache.Contains(evtKey) {
		return
	}

	// get policy
	policy, err := getInvolvePolicy(h.ctx, h.runtimeClient, evt)
	if err != nil {
		h.log.Error(err, "failed to get involved policy", "event", evt.Namespace+"/"+evt.Name,
			"policy", evt.InvolvedObject.Namespace+"/"+evt.InvolvedObject.Name)
		return
	}

	// global resource || root policy
	if utils.HasAnnotation(policy, constants.OriginOwnerReferenceAnnotation) ||
		!utils.HasItemKey(policy.GetLabels(), constants.PolicyEventRootPolicyNameLabelKey) {
		return
	}

	rootPolicy, clusterID, err := localpolicies.GetRootPolicyAndClusterID(h.ctx, policy, h.runtimeClient)
	if err != nil {
		h.log.Error(err, "failed to get get rootPolicy/clusterID by replicatedPolicy")
		return
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
		PolicyID:   string(rootPolicy.GetUID()),
		ClusterID:  clusterID,
		Compliance: policyCompliance(rootPolicy, evt),
	}

	if replicatedPolicyEvent.PolicyID == "" {
		h.log.Error(err, "the policyID of the replicated policy event is not retrieved", "event",
			evt.Namespace+"/"+evt.Name)
		return
	}

	// cache to events and update version
	h.events = append(h.events, replicatedPolicyEvent)
	h.cache.Add(evtKey, nil)
	h.currentVersion.Incr()
}

func (*localReplicatedPolicyEmitter) Delete(client.Object) {
	// do nothing
}

func (h *localReplicatedPolicyEmitter) ToCloudEvent() *cloudevents.Event {
	if len(h.events) < 1 {
		return nil
	}
	e := cloudevents.NewEvent()
	e.SetType(h.eventType)
	e.SetExtension(metadata.ExtVersion, h.currentVersion.String())
	err := e.SetData(cloudevents.ApplicationJSON, h.events)
	if err != nil {
		h.log.Error(err, "failed to set the payload to cloudvents.Data")
	}
	return &e
}

func (h *localReplicatedPolicyEmitter) PostSend() {
	// update version and clean the cache
	h.events = make([]event.ReplicatedPolicyEvent, 0)
	h.currentVersion.Next()
	h.lastSentVersion = *h.currentVersion
}
