package hubofhubs

import (
	"context"
	"fmt"

	mchv1 "github.com/stolostron/multiclusterhub-operator/api/v1"
	"k8s.io/apimachinery/pkg/api/errors"
	"k8s.io/apimachinery/pkg/api/meta"
	"sigs.k8s.io/controller-runtime/pkg/client"

	operatorv1alpha2 "github.com/stolostron/multicluster-global-hub/operator/apis/v1alpha2"
	"github.com/stolostron/multicluster-global-hub/operator/pkg/condition"
)

func (r *MulticlusterGlobalHubReconciler) reconcileMCH(ctx context.Context,
	mgh *operatorv1alpha2.MulticlusterGlobalHub,
) error {
	mch, _ := getMCH(ctx, r.Client)
	// skip reconcile mch if it is not found
	if mch == nil {
		return nil
	}

	mch, _ = disableGRCInMCH(mch)

	// if grcDisabledByMGH {
	// 	// set the annotation to remember grc status
	// 	annotations := mgh.GetAnnotations()
	// 	annotations[GRCDisabledByMGHAnnotation] = strconv.FormatBool(grcDisabledByMGH)
	// 	mgh.SetAnnotations(annotations)
	// 	if err := r.Client.Update(ctx, mgh, &client.UpdateOptions{}); err != nil {
	// 		return fmt.Errorf("failed to annotation(%s) to mgh. err = %v", GRCDisabledByMGHAnnotation, err)
	// 	}
	// }

	if err := r.Client.Update(ctx, mch); err != nil {
		return fmt.Errorf("failed to update MCH instance. err = %v", err)
	}

	if err := condition.SetConditionGRCDisabled(ctx, r.Client, mgh,
		condition.CONDITION_STATUS_TRUE); err != nil {
		return condition.FailToSetConditionError(condition.CONDITION_STATUS_TRUE, err)
	}

	return nil
}

func disableGRCInMCH(mch *mchv1.MultiClusterHub) (*mchv1.MultiClusterHub, bool) {
	grcDisabledByMGH := false
	if mch.Spec.Overrides != nil {
		found := false
		for i, c := range mch.Spec.Overrides.Components {
			if c.Name == "grc" {
				if c.Enabled {
					mch.Spec.Overrides.Components[i].Enabled = false
					grcDisabledByMGH = true
				}
				found = true
				break
			}
		}
		if !found {
			mch.Spec.Overrides.Components = append(mch.Spec.Overrides.Components,
				mchv1.ComponentConfig{
					Name:    "grc",
					Enabled: false,
				})
			grcDisabledByMGH = true
		}
	} else {
		mch.Spec.Overrides = &mchv1.Overrides{
			Components: []mchv1.ComponentConfig{
				{
					Name:    "grc",
					Enabled: false,
				},
			},
		}
		grcDisabledByMGH = true
	}

	return mch, grcDisabledByMGH
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
