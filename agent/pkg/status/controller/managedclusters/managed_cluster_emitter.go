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

func NewManagedClusterEmitter(topic string) generic.ObjectEmitter {
	eventData := genericpayload.GenericObjectData{}
	return generic.NewGenericObjectEmitter(
		enum.LocalPolicySpecType,
		eventData,
		generic.NewGenericObjectHandler(eventData),
		generic.WithTweakFunc(managedClusterTweakFunc),
		generic.WithTopic(topic),
		// default predicate return true
	)
}

func managedClusterTweakFunc(object client.Object) {
	utils.MergeAnnotations(object, map[string]string{
		constants.ManagedClusterManagedByAnnotation: config.GetLeafHubName(),
	})
}
