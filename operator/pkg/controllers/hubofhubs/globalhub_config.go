package hubofhubs

import (
	"context"

	globalhubv1alpha4 "github.com/stolostron/multicluster-global-hub/operator/apis/v1alpha4"
	"github.com/stolostron/multicluster-global-hub/operator/pkg/config"
	"k8s.io/apimachinery/pkg/types"
)

// reconcileSystemConfig tries to create hoh resources if they don't exist
func (r *MulticlusterGlobalHubReconciler) reconcileSystemConfig(ctx context.Context,
	mgh *globalhubv1alpha4.MulticlusterGlobalHub,
) error {
	log := r.Log.WithName("config")
	log.Info("set operand images; service monitor interval; set global hub agent config")

	// set request name to be used in leafhub controller
	config.SetMGHNamespacedName(types.NamespacedName{
		Namespace: mgh.GetNamespace(), Name: mgh.GetName(),
	})

	// set image overrides
	if err := config.SetImageOverrides(mgh); err != nil {
		return err
	}

	config.SetImagePullSecretName(mgh)

	// set statistic log interval
	if err := config.SetStatisticLogInterval(mgh); err != nil {
		return err
	}
	return nil
}
