package hubofhubs

import (
	"context"
	"fmt"
	"strconv"

	mchv1 "github.com/stolostron/multiclusterhub-operator/api/v1"
	"k8s.io/apimachinery/pkg/api/errors"
	"k8s.io/apimachinery/pkg/api/meta"
	"sigs.k8s.io/controller-runtime/pkg/client"

	operatorv1alpha2 "github.com/stolostron/multicluster-global-hub/operator/apis/v1alpha2"
	"github.com/stolostron/multicluster-global-hub/operator/pkg/condition"
)

const (
	GRCDisabledByMGHAnnotation = "operator.open-cluster-management.io/grc-disabled-by-mgh"
)

func (r *MulticlusterGlobalHubReconciler) reconcileMCH(ctx context.Context,
	mgh *operatorv1alpha2.MulticlusterGlobalHub,
) error {
	mch, err := getMCH(ctx, r.Client)
	if err != nil {
		return fmt.Errorf("failed to get MCH instance. err = %v", err)
	}

	grcDisabledByMGH := false
	if mch.Spec.Overrides != nil {
		found := false
		for _, c := range mch.Spec.Overrides.Components {
			if c.Name == "grc" {
				if c.Enabled {
					c.Enabled = false
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

	if err := condition.SetConditionGRCDisabled(ctx, r.Client, mgh,
		condition.CONDITION_STATUS_TRUE); err != nil {
		return condition.FailToSetConditionError(condition.CONDITION_STATUS_TRUE, err)
	}

	if grcDisabledByMGH {
		// set the annotation to remember grc status
		annotations := mgh.GetAnnotations()
		annotations[GRCDisabledByMGHAnnotation] = strconv.FormatBool(grcDisabledByMGH)
		mgh.SetAnnotations(annotations)
		if err := r.Client.Update(ctx, mgh, &client.UpdateOptions{}); err != nil {
			return fmt.Errorf("failed to annotation(%s) to mgh. err = %v", GRCDisabledByMGHAnnotation, err)
		}
	}

	return r.Client.Update(ctx, mch)
}

func (r *MulticlusterGlobalHubReconciler) recoverMCH(ctx context.Context,
	mgh *operatorv1alpha2.MulticlusterGlobalHub,
) error {
	grcDisabledByMGHAnnotationVal := mgh.GetAnnotations()[GRCDisabledByMGHAnnotation]
	if grcDisabledByMGHAnnotationVal == "" {
		return nil
	}

	grcDisabledByMGH, err := strconv.ParseBool(grcDisabledByMGHAnnotationVal)
	if err != nil {
		return fmt.Errorf("invalid annotation value type for %s from MGH instance. error = %v",
			GRCDisabledByMGHAnnotation, err)
	}

	if !grcDisabledByMGH {
		return nil
	}

	mch, err := getMCH(ctx, r.Client)
	if err != nil {
		return fmt.Errorf("failed to get MCH instance. err = %v", err)
	}

	if mch.Spec.Overrides != nil {
		found := false
		for _, c := range mch.Spec.Overrides.Components {
			if c.Name == "grc" {
				if !c.Enabled {
					c.Enabled = true
				}
				found = true
				break
			}
		}
		if !found {
			mch.Spec.Overrides.Components = append(mch.Spec.Overrides.Components,
				mchv1.ComponentConfig{
					Name:    "grc",
					Enabled: true,
				})
		}
	} else {
		mch.Spec.Overrides = &mchv1.Overrides{
			Components: []mchv1.ComponentConfig{
				{
					Name:    "grc",
					Enabled: true,
				},
			},
		}
	}

	return r.Client.Update(ctx, mch)
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
