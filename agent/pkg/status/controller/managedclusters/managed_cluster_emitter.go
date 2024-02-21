package managedclusters

import (
	"sigs.k8s.io/controller-runtime/pkg/client"

	"github.com/stolostron/multicluster-global-hub/agent/pkg/status/controller/config"
	"github.com/stolostron/multicluster-global-hub/agent/pkg/status/controller/generic"
	genericpayload "github.com/stolostron/multicluster-global-hub/pkg/bundle/generic"
	"github.com/stolostron/multicluster-global-hub/pkg/constants"
	"github.com/stolostron/multicluster-global-hub/pkg/enum"
	"github.com/stolostron/multicluster-global-hub/pkg/utils"
)

func NewManagedClusterEmitter(runtimeClient client.Client, topic string) generic.MultiEventEmitter {
	predicate := func(obj client.Object) bool {
		return true
	}

	return generic.NewGenericMultiEventEmitter(
		"managedCluster-syncer/emitter",
		enum.ManagedClusterType,
		runtimeClient,
		predicate,
		&genericpayload.GenericPayload{},
		generic.WithTopic(topic),
		generic.WithTweakFunc(managedClusterTweakFunc),
	)
}

func managedClusterTweakFunc(object client.Object) {
	utils.MergeAnnotations(object, map[string]string{
		constants.ManagedClusterManagedByAnnotation: config.GetLeafHubName(),
	})
}
