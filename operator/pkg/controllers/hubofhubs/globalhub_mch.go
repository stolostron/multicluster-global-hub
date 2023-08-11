package hubofhubs

import (
	"context"
	"fmt"

	mchv1 "github.com/stolostron/multiclusterhub-operator/api/v1"
	"k8s.io/apimachinery/pkg/api/errors"
	"k8s.io/apimachinery/pkg/api/meta"
	"sigs.k8s.io/controller-runtime/pkg/client"

	globalhubv1alpha4 "github.com/stolostron/multicluster-global-hub/operator/apis/v1alpha4"
	"github.com/stolostron/multicluster-global-hub/operator/pkg/condition"
	"github.com/stolostron/multicluster-global-hub/operator/pkg/utils"
)

func (r *MulticlusterGlobalHubReconciler) reconcileMCH(ctx context.Context,
	mgh *globalhubv1alpha4.MulticlusterGlobalHub,
) error {
	mch, _ := getMCH(ctx, r.Client)
	// skip reconcile mch if it is not found
	if mch == nil {
		return nil
	}

	disableComponentsInMCH(mch, []string{"app-lifecycle", "grc"})

	if err := r.Client.Update(ctx, mch); err != nil {
		return fmt.Errorf("failed to update MCH instance. err = %v", err)
	}

	if err := condition.SetConditionMCHConfigured(ctx, r.Client, mgh,
		condition.CONDITION_STATUS_TRUE); err != nil {
		return condition.FailToSetConditionError(condition.CONDITION_STATUS_TRUE, err)
	}

	return nil
}

func disableComponentsInMCH(mch *mchv1.MultiClusterHub, components []string) {
	if len(components) == 0 {
		return
	}
	if mch.Spec.Overrides != nil {
		foundMap := make(map[string]struct{}, len(components))
		for i, c := range mch.Spec.Overrides.Components {
			if utils.Contains(components, c.Name) {
				if c.Enabled {
					mch.Spec.Overrides.Components[i].Enabled = false
				}
				foundMap[c.Name] = struct{}{}
			}
		}
		for _, c := range components {
			if _, existing := foundMap[c]; !existing {
				mch.Spec.Overrides.Components = append(mch.Spec.Overrides.Components,
					mchv1.ComponentConfig{
						Name:    c,
						Enabled: false,
					})
			}
		}
	} else {
		componentConfigs := make([]mchv1.ComponentConfig, len(components))
		for _, c := range components {
			componentConfigs = append(componentConfigs, mchv1.ComponentConfig{
				Name:    c,
				Enabled: false,
			})
		}
		mch.Spec.Overrides = &mchv1.Overrides{
			Components: componentConfigs,
		}
	}
}

func getMCH(ctx context.Context, k8sClient client.Client) (*mchv1.MultiClusterHub, error) {
	mch := &mchv1.MultiClusterHubList{}
	err := k8sClient.List(ctx, mch)
	if errors.IsNotFound(err) {
		return nil, nil
	}
	if meta.IsNoMatchError(err) {
		return nil, nil
	}
	if err != nil {
		return nil, err
	}

	if len(mch.Items) == 0 {
		return nil, err
	}

	return &mch.Items[0], nil
}
