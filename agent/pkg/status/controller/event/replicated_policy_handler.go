package event

import (
	cloudevents "github.com/cloudevents/sdk-go/v2"
	"github.com/stolostron/multicluster-global-hub/agent/pkg/status/controller/generic"
	"github.com/stolostron/multicluster-global-hub/pkg/bundle/metadata"
	corev1 "k8s.io/api/core/v1"
	"sigs.k8s.io/controller-runtime/pkg/client"
)

type replicatedPolicyEventHandler struct {
	currentEvent *corev1.Event
}

func NewPolicyEventHandler() generic.ObjectHandler {
	return &replicatedPolicyEventHandler{}
}

func (*replicatedPolicyEventHandler) Predicate(obj client.Object) bool {
	event, ok := obj.(*corev1.Event)
	if !ok {
		return false
	}

	policyName := event.InvolvedObject.Name
	policyNamespace := event.InvolvedObject.Namespace

	// get policy

	// only sync the policy event
	return event.InvolvedObject.Kind == "Policy"
}

func (*replicatedPolicyEventHandler) Update(obj client.Object) {
	event, ok := obj.(*corev1.Event)
	if !ok {
		return
	}

	policyName := event.InvolvedObject.Name
	policyNamespace := event.InvolvedObject.Namespace

}

func (*replicatedPolicyEventHandler) Delete(client.Object) {

}

func (*replicatedPolicyEventHandler) GetVersion() *metadata.BundleVersion {
	return nil
}

func (*replicatedPolicyEventHandler) ToCloudEvent() *cloudevents.Event {
	return nil
}

func (*replicatedPolicyEventHandler) PostSend() {

}
