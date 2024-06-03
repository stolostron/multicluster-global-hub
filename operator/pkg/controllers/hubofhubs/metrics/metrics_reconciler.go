package metrics

import (
	"context"

	promv1 "github.com/prometheus-operator/prometheus-operator/pkg/apis/monitoring/v1"
	corev1 "k8s.io/api/core/v1"
	"k8s.io/apimachinery/pkg/api/equality"
	"k8s.io/apimachinery/pkg/api/errors"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"sigs.k8s.io/controller-runtime/pkg/client"

	"github.com/stolostron/multicluster-global-hub/operator/apis/v1alpha4"
	"github.com/stolostron/multicluster-global-hub/operator/pkg/config"
	operatorconstants "github.com/stolostron/multicluster-global-hub/operator/pkg/constants"
	"github.com/stolostron/multicluster-global-hub/pkg/constants"
	"github.com/stolostron/multicluster-global-hub/pkg/utils"
)

type MetricsReconciler struct {
	client.Client
}

func (r *MetricsReconciler) Reconcile(ctx context.Context, mgh *v1alpha4.MulticlusterGlobalHub) error {
	// add label openshift.io/cluster-monitoring: "true" to the ns, so that the prometheus can detect the ServiceMonitor.
	namespace := &corev1.Namespace{
		ObjectMeta: metav1.ObjectMeta{
			Name: utils.GetDefaultNamespace(),
		},
	}
	if err := r.Client.Get(ctx, client.ObjectKeyFromObject(namespace), namespace); err != nil {
		return err
	}
	labels := namespace.GetLabels()
	if labels == nil {
		labels = make(map[string]string)
	}

	val, ok := labels[operatorconstants.ClusterMonitoringLabelKey]
	if !ok || val != operatorconstants.ClusterMonitoringLabelVal {
		labels[operatorconstants.ClusterMonitoringLabelKey] = operatorconstants.ClusterMonitoringLabelVal
	}
	namespace.SetLabels(labels)
	if err := r.Client.Update(ctx, namespace); err != nil {
		return err
	}

	// create ServiceMonitor under global hub namespace
	expectedServiceMonitor := &promv1.ServiceMonitor{
		ObjectMeta: metav1.ObjectMeta{
			Name:      operatorconstants.GHServiceMonitorName,
			Namespace: utils.GetDefaultNamespace(),
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
					Interval: promv1.Duration(config.GetMetricsScrapeInterval(mgh)),
				},
			},
		},
	}

	serviceMonitor := &promv1.ServiceMonitor{}
	err := r.Client.Get(ctx, client.ObjectKeyFromObject(expectedServiceMonitor), serviceMonitor)
	if err != nil && errors.IsNotFound(err) {
		return r.Create(ctx, expectedServiceMonitor)
	} else if err != nil {
		return err
	}

	if !equality.Semantic.DeepDerivative(expectedServiceMonitor.Spec, serviceMonitor.Spec) ||
		!equality.Semantic.DeepDerivative(expectedServiceMonitor.GetLabels(), serviceMonitor.GetLabels()) {
		expectedServiceMonitor.ObjectMeta.ResourceVersion = serviceMonitor.ObjectMeta.ResourceVersion
		return r.Update(ctx, expectedServiceMonitor)
	}

	return nil
}
