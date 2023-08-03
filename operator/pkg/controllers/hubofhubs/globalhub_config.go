package hubofhubs

import (
	"context"
	"strconv"

	corev1 "k8s.io/api/core/v1"
	"k8s.io/apimachinery/pkg/api/equality"
	"k8s.io/apimachinery/pkg/api/errors"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/types"

	operatorv1alpha3 "github.com/stolostron/multicluster-global-hub/operator/apis/v1alpha3"
	"github.com/stolostron/multicluster-global-hub/operator/pkg/config"
	operatorconstants "github.com/stolostron/multicluster-global-hub/operator/pkg/constants"
	"github.com/stolostron/multicluster-global-hub/operator/pkg/utils"
	"github.com/stolostron/multicluster-global-hub/pkg/constants"
)

var monitorFlag = false

// reconcileSystemConfig tries to create hoh resources if they don't exist
func (r *MulticlusterGlobalHubReconciler) reconcileSystemConfig(ctx context.Context,
	mgh *operatorv1alpha3.MulticlusterGlobalHub,
) error {
	log := r.Log.WithName("config")
	log.Info("set operand images; monitor the global hub namespace; set global hub agent config")
	// set image overrides
	if err := config.SetImageOverrides(mgh); err != nil {
		return err
	}

	// set statistic log interval
	if err := config.SetStatisticLogInterval(mgh); err != nil {
		return err
	}

	// add label openshift.io/cluster-monitoring: "true" to the ns, so that the prometheus can detect the ServiceMonitor.
	if !monitorFlag {
		r.KubeClient.CoreV1().Namespaces()
		ns, err := r.KubeClient.CoreV1().Namespaces().Get(ctx, config.GetDefaultNamespace(), metav1.GetOptions{})
		if err != nil {
			return err
		}
		labels := ns.GetLabels()
		if labels == nil {
			labels = make(map[string]string)
		}
		labels[operatorconstants.ClusterMonitoringLabelKey] = operatorconstants.ClusterMonitoringLabelVal
		ns.SetLabels(labels)
		if _, err = r.KubeClient.CoreV1().Namespaces().Update(ctx, ns, metav1.UpdateOptions{}); err != nil {
			return err
		}
		monitorFlag = true
	}

	// reconcile global hub global hub config
	if err := r.Client.Get(ctx,
		types.NamespacedName{
			Name: constants.GHSystemNamespace,
		}, &corev1.Namespace{}); err != nil {
		if errors.IsNotFound(err) {
			log.Info("creating global hub system namespace for config", "namespace", constants.GHSystemNamespace)
			if err := r.Client.Create(ctx, &corev1.Namespace{
				ObjectMeta: metav1.ObjectMeta{
					Name: constants.GHSystemNamespace,
					Labels: map[string]string{
						constants.GlobalHubOwnerLabelKey: constants.GHOperatorOwnerLabelVal,
					},
				},
			}); err != nil {
				return err
			}
		} else {
			return err
		}
	}

	// hoh configmap
	expectedHoHConfigMap := &corev1.ConfigMap{
		ObjectMeta: metav1.ObjectMeta{
			Namespace: constants.GHSystemNamespace,
			Name:      constants.GHAgentConfigCMName,
			Labels: map[string]string{
				constants.GlobalHubOwnerLabelKey: constants.GHOperatorOwnerLabelVal,
			},
		},
		Data: map[string]string{
			"aggregationLevel":    string(operatorconstants.FullAggregation),
			"enableLocalPolicies": strconv.FormatBool(true),
		},
	}

	existingHoHConfigMap := &corev1.ConfigMap{}
	if err := r.Client.Get(ctx,
		types.NamespacedName{
			Namespace: constants.GHSystemNamespace,
			Name:      constants.GHAgentConfigCMName,
		}, existingHoHConfigMap); err != nil {
		if errors.IsNotFound(err) {
			log.Info("creating global hub configmap", "namespace", constants.GHSystemNamespace,
				"name", constants.GHAgentConfigCMName)
			if err := r.Client.Create(ctx, expectedHoHConfigMap); err != nil {
				return err
			}
		} else {
			return err
		}
	}

	if !equality.Semantic.DeepDerivative(expectedHoHConfigMap.Data, existingHoHConfigMap.Data) ||
		!equality.Semantic.DeepDerivative(expectedHoHConfigMap.GetLabels(), existingHoHConfigMap.GetLabels()) {
		expectedHoHConfigMap.ObjectMeta.ResourceVersion = existingHoHConfigMap.ObjectMeta.ResourceVersion
		log.Info("updating global hub configmap", "namespace", constants.GHSystemNamespace,
			"name", constants.GHAgentConfigCMName)
		if err := utils.UpdateObject(ctx, r.Client, expectedHoHConfigMap); err != nil {
			return err
		}
	}
	config.SetGlobalHubAgentConfig(expectedHoHConfigMap)
	return nil
}
