package hubofhubs

import (
	"context"

	subv1alpha1 "github.com/operator-framework/api/pkg/operators/v1alpha1"
	"github.com/stolostron/multicluster-global-hub/operator/pkg/config"
	"github.com/stolostron/multicluster-global-hub/pkg/constants"
	appsv1 "k8s.io/api/apps/v1"
	"k8s.io/apimachinery/pkg/types"
)

// GenerateSubConfig returns a SubscriptionConfig based on proxy variables and the mch operator configuration
func (r *MulticlusterGlobalHubReconciler) GenerateSubConfig(ctx context.Context) (*subv1alpha1.SubscriptionConfig, error) {
	found := &appsv1.Deployment{}
	err := r.Client.Get(ctx, types.NamespacedName{
		Name:      constants.GHOperatorDeploymentName,
		Namespace: config.GetDefaultNamespace(),
	}, found)
	if err != nil {
		return nil, err
	}

	return &subv1alpha1.SubscriptionConfig{
		NodeSelector: found.Spec.Template.Spec.NodeSelector,
		Tolerations:  found.Spec.Template.Spec.Tolerations,
	}, nil
}
