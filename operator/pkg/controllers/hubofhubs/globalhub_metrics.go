package hubofhubs

import (
	"context"

	promv1 "github.com/prometheus-operator/prometheus-operator/pkg/apis/monitoring/v1"
	"k8s.io/apimachinery/pkg/api/equality"
	"k8s.io/apimachinery/pkg/api/errors"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"sigs.k8s.io/controller-runtime/pkg/client"

	operatorv1alpha3 "github.com/stolostron/multicluster-global-hub/operator/apis/v1alpha3"
	"github.com/stolostron/multicluster-global-hub/operator/pkg/config"
	operatorconstants "github.com/stolostron/multicluster-global-hub/operator/pkg/constants"
	"github.com/stolostron/multicluster-global-hub/pkg/constants"
)

func (r *MulticlusterGlobalHubReconciler) reconcileMetrics(ctx context.Context,
	mgh *operatorv1alpha3.MulticlusterGlobalHub,
) error {
	log := r.Log.WithName("metrics")

	expectedServiceMonitor := &promv1.ServiceMonitor{
		ObjectMeta: metav1.ObjectMeta{
			Name:      operatorconstants.GHServiceMonitorName,
			Namespace: operatorconstants.GHServiceMonitorNamespace,
			Labels: map[string]string{
				constants.GlobalHubOwnerLabelKey: constants.GHOperatorOwnerLabelVal,
			},
		},
		Spec: promv1.ServiceMonitorSpec{
			Selector: metav1.LabelSelector{
				MatchLabels: map[string]string{
					"name": "multicluster-global-hub-manager",
				},
			},
			NamespaceSelector: promv1.NamespaceSelector{
				MatchNames: []string{
					mgh.Namespace,
				},
			},
			Endpoints: []promv1.Endpoint{
				{
					Port:     "metrics",
					Path:     "/metrics",
					Interval: promv1.Duration(config.GetStatisticLogInterval().String()),
				},
			},
		},
	}

	serviceMonitor := &promv1.ServiceMonitor{}
	err := r.Client.Get(ctx, client.ObjectKeyFromObject(expectedServiceMonitor), serviceMonitor)
	if err != nil && errors.IsNotFound(err) {
		log.Info("creating ServiceMonitor", "namespace", serviceMonitor.Namespace, "name", serviceMonitor.Name)
		return r.Create(ctx, expectedServiceMonitor)
	} else if err != nil {
		return err
	}

	if !equality.Semantic.DeepDerivative(expectedServiceMonitor.Spec, serviceMonitor.Spec) ||
		!equality.Semantic.DeepDerivative(expectedServiceMonitor.GetLabels(), serviceMonitor.GetLabels()) {
		expectedServiceMonitor.ObjectMeta.ResourceVersion =
			serviceMonitor.ObjectMeta.ResourceVersion
		log.Info("updating ServiceMonitor", "namespace", serviceMonitor.Namespace, "name", serviceMonitor.Name)
		return r.Update(ctx, expectedServiceMonitor)
	}

	return nil
}
