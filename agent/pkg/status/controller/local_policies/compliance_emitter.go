package localpolicies

import (
	"sigs.k8s.io/controller-runtime/pkg/client"

	"github.com/stolostron/multicluster-global-hub/agent/pkg/status/controller/config"
	"github.com/stolostron/multicluster-global-hub/agent/pkg/status/controller/generic"
	"github.com/stolostron/multicluster-global-hub/pkg/bundle/grc"
	"github.com/stolostron/multicluster-global-hub/pkg/bundle/metadata"
	"github.com/stolostron/multicluster-global-hub/pkg/constants"
	"github.com/stolostron/multicluster-global-hub/pkg/enum"
	"github.com/stolostron/multicluster-global-hub/pkg/utils"
)

func NewComplianceEmitter(runtimeClient client.Client, topic string,
	version *metadata.BundleVersion) generic.MultiEventEmitter {
	return generic.NewGenericMultiEventEmitter(
		"local-policy-syncer/compliance",
		enum.LocalPolicyComplianceType,
		runtimeClient,
		complianceEmitterPredicate,
		&grc.CompliancePayload{},
		generic.WithTopic(topic),
		generic.WithVersion(version),
	)
}

func complianceEmitterPredicate(obj client.Object) bool {
	return !utils.HasItemKey(obj.GetLabels(), constants.PolicyEventRootPolicyNameLabelKey) &&
		config.GetAggregationLevel() == config.AggregationFull
}
