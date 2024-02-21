package localpolicies

import (
	"github.com/stolostron/multicluster-global-hub/agent/pkg/status/controller/generic"
	"github.com/stolostron/multicluster-global-hub/pkg/bundle/grc"
	"github.com/stolostron/multicluster-global-hub/pkg/bundle/metadata"
	"github.com/stolostron/multicluster-global-hub/pkg/enum"
	"sigs.k8s.io/controller-runtime/pkg/client"
)

func NewCompleteComplianceEmitter(runtimeClient client.Client, topic string,
	dependencyVersion *metadata.BundleVersion) generic.MultiEventEmitter {
	return generic.NewGenericMultiEventEmitter(
		"local-policy-syncer/completeCompliance",
		enum.LocalPolicyCompleteComplianceType,
		runtimeClient,
		complianceEmitterPredicate,
		&grc.CompleteCompliancePayload{},
		generic.WithTopic(topic),
		generic.WithDependencyVersion(dependencyVersion),
	)
}
