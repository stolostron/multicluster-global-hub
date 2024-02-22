package hubcluster

import (
	clustersv1alpha1 "open-cluster-management.io/api/cluster/v1alpha1"
	"sigs.k8s.io/controller-runtime/pkg/client"
	"sigs.k8s.io/controller-runtime/pkg/predicate"

	"github.com/stolostron/multicluster-global-hub/agent/pkg/status/controller/generic"
	"github.com/stolostron/multicluster-global-hub/pkg/bundle/cluster"
)

var _ generic.EventController = &infoClusterClaimController{}

type infoClusterClaimController struct {
	generic.Controller
	evtData cluster.HubClusterInfoData
}

func NewInfoClusterClaimController(eventData cluster.HubClusterInfoData) generic.EventController {
	instance := func() client.Object { return &clustersv1alpha1.ClusterClaim{} }
	clusterClaimPredicate := predicate.NewPredicateFuncs(func(object client.Object) bool {
		return object.GetName() == "id.k8s.io"
	})
	return &infoClusterClaimController{
		Controller: generic.NewGenericController(instance, clusterClaimPredicate),
		evtData:    eventData,
	}
}

func (p *infoClusterClaimController) Update(obj client.Object) bool {

	clusterClaim, ok := obj.(*clustersv1alpha1.ClusterClaim)
	if !ok {
		return false
	}

	oldClusterID := p.evtData.ClusterId

	if clusterClaim.Name == "id.k8s.io" {
		p.evtData.ClusterId = clusterClaim.Spec.Value
	}
	// If no ClusterId, do not send the bundle
	if p.evtData.ClusterId == "" {
		return false
	}

	return oldClusterID != p.evtData.ClusterId
}

func (p *infoClusterClaimController) Delete(obj client.Object) bool {
	// do nothing
	return false
}

func (p *infoClusterClaimController) Data() interface{} {
	return p.evtData
}
