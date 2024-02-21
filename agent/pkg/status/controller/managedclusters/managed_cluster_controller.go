package managedclusters

import (
	"time"

	clusterv1 "open-cluster-management.io/api/cluster/v1"
	"sigs.k8s.io/controller-runtime/pkg/client"
	"sigs.k8s.io/controller-runtime/pkg/predicate"

	"github.com/stolostron/multicluster-global-hub/agent/pkg/status/controller/config"
	"github.com/stolostron/multicluster-global-hub/agent/pkg/status/controller/generic"
)

var _ generic.ObjectController = &managedClusterController{}

type managedClusterController struct {
	interval  func() time.Duration
	finalizer bool
}

func NewManagedClusterController() *managedClusterController {
	return &managedClusterController{
		interval:  config.GetManagerClusterDuration,
		finalizer: false,
	}
}

func (s *managedClusterController) Instance() client.Object {
	return &clusterv1.ManagedCluster{}
}

func (s *managedClusterController) Predicate() predicate.Predicate {
	return predicate.NewPredicateFuncs(func(object client.Object) bool {
		return true
	})
}

func (s *managedClusterController) EnableFinalizer() bool {
	return s.finalizer
}
