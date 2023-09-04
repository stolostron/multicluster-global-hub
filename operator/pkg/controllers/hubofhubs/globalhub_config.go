package hubofhubs

import (
	"context"

	globalhubv1alpha4 "github.com/stolostron/multicluster-global-hub/operator/apis/v1alpha4"
	"github.com/stolostron/multicluster-global-hub/operator/pkg/config"
)

// reconcileSystemConfig tries to create hoh resources if they don't exist
func (r *MulticlusterGlobalHubReconciler) reconcileSystemConfig(ctx context.Context,
	mgh *globalhubv1alpha4.MulticlusterGlobalHub,
) error {
	log := r.Log.WithName("config")
	log.Info("set operand images; service monitor interval; set global hub agent config")
	// set image overrides
	if err := config.SetImageOverrides(mgh); err != nil {
		return err
	}

	// set statistic log interval
	if err := config.SetStatisticLogInterval(mgh); err != nil {
		return err
	}
	return nil
}
