package hubcluster

import (
	"github.com/stolostron/multicluster-global-hub/pkg/bundle"
	"github.com/stolostron/multicluster-global-hub/pkg/bundle/base"
	"github.com/stolostron/multicluster-global-hub/pkg/bundle/cluster"
	clustersv1alpha1 "open-cluster-management.io/api/cluster/v1alpha1"
	"sigs.k8s.io/controller-runtime/pkg/client"
	"sigs.k8s.io/controller-runtime/pkg/predicate"
)

var _ bundle.ObjectHandler = (*hubClusterClaimHandler)(nil)

type hubClusterClaimHandler struct{}

func NewHubClusterInfoClaimHandler() *hubClusterClaimHandler {
	return &hubClusterClaimHandler{}
}

func (h *hubClusterClaimHandler) Predicate() predicate.Predicate {
	return predicate.NewPredicateFuncs(func(object client.Object) bool {
		return object.GetName() == "id.k8s.io"
	})
}

func (h *hubClusterClaimHandler) CreateObject() bundle.Object {
	return &clustersv1alpha1.ClusterClaim{}
}

func (h *hubClusterClaimHandler) BundleUpdate(obj bundle.Object, b bundle.BaseAgentBundle) {
	hubClusterBundle, ok1 := ensureBundle(b)
	clusterClaim, ok2 := obj.(*clustersv1alpha1.ClusterClaim)
	if !ok1 || !ok2 {
		return
	}

	oldClusterID := hubClusterBundle.Objects[0].ClusterId
	if clusterClaim.Name == "id.k8s.io" {
		hubClusterBundle.Objects[0].ClusterId = clusterClaim.Spec.Value
	}
	// If no ClusterId, do not send the bundle
	if hubClusterBundle.Objects[0].ClusterId == "" {
		return
	}
	if oldClusterID != hubClusterBundle.Objects[0].ClusterId {
		hubClusterBundle.GetVersion().Incr()
	}
}

func (h *hubClusterClaimHandler) BundleDelete(obj bundle.Object, b bundle.BaseAgentBundle) {
	// do noting
}

func ensureBundle(b bundle.BaseAgentBundle) (*cluster.HubClusterInfoBundle, bool) {
	hubClusterBundle, ok := b.(*cluster.HubClusterInfoBundle)
	if !ok {
		return nil, false
	}

	if len(hubClusterBundle.Objects) == 0 {
		hubClusterBundle.Objects = []*base.HubClusterInfo{
			{
				ConsoleURL: "",
				GrafanaURL: "",
				ClusterId:  "",
			},
		}
	}
	return hubClusterBundle, true
}
