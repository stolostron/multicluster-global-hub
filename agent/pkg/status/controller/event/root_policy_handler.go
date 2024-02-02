package event

import (
	"context"

	cloudevents "github.com/cloudevents/sdk-go/v2"
	"github.com/go-logr/logr"
	"github.com/stolostron/multicluster-global-hub/agent/pkg/status/controller/generic"
	"github.com/stolostron/multicluster-global-hub/pkg/bundle/metadata"
	corev1 "k8s.io/api/core/v1"
	"k8s.io/apimachinery/pkg/api/errors"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	policiesv1 "open-cluster-management.io/governance-policy-propagator/api/v1"
	"sigs.k8s.io/controller-runtime/pkg/client"
)

type rootPolicyEventHandler struct {
	ctx           context.Context
	log           logr.Logger
	runtimeClient client.Client
	version       *metadata.BundleVersion
	currentEvent  *corev1.Event
}

func NewRootPolicyEventHandler() generic.ObjectHandler {
	return &rootPolicyEventHandler{}
}

func (h *rootPolicyEventHandler) Update(obj client.Object) {
	event, ok := obj.(*corev1.Event)
	if !ok {
		return
	}

	// get policy
	policy := &policiesv1.Policy{
		ObjectMeta: metav1.ObjectMeta{
			Name:      event.InvolvedObject.Name,
			Namespace: event.InvolvedObject.Namespace,
		},
	}
	err := h.runtimeClient.Get(h.ctx, client.ObjectKeyFromObject(policy), policy)
	if errors.IsNotFound(err) {
		h.log.Error(err, "failed to get involved object", "event", event.Namespace+"/"+event.Name,
			"policy", policy.Namespace+"/"+policy.Name)
		return
	}

}

func (*rootPolicyEventHandler) Delete(client.Object) {
	// do nothing
}

func (h *rootPolicyEventHandler) GetVersion() *metadata.BundleVersion {
	return h.version
}

func (*rootPolicyEventHandler) ToCloudEvent() *cloudevents.Event {
	return nil
}

func (h *rootPolicyEventHandler) PostSend() {
	// clean the send payload
}
