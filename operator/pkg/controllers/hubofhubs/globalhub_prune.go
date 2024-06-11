package hubofhubs

import (
	"context"
	"fmt"
	"time"

	kafkav1beta2 "github.com/RedHatInsights/strimzi-client-go/apis/kafka.strimzi.io/v1beta2"
	"github.com/go-logr/logr"
	subv1alpha1 "github.com/operator-framework/api/pkg/operators/v1alpha1"
	promv1 "github.com/prometheus-operator/prometheus-operator/pkg/apis/monitoring/v1"
	admissionregistrationv1 "k8s.io/api/admissionregistration/v1"
	corev1 "k8s.io/api/core/v1"
	rbacv1 "k8s.io/api/rbac/v1"
	"k8s.io/apimachinery/pkg/api/errors"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/types"
	"k8s.io/apimachinery/pkg/util/wait"
	"k8s.io/klog"
	addonv1alpha1 "open-cluster-management.io/api/addon/v1alpha1"
	"sigs.k8s.io/controller-runtime/pkg/client"

	globalhubv1alpha4 "github.com/stolostron/multicluster-global-hub/operator/apis/v1alpha4"
	operatorconstants "github.com/stolostron/multicluster-global-hub/operator/pkg/constants"
	"github.com/stolostron/multicluster-global-hub/operator/pkg/transporter"
	operatorutils "github.com/stolostron/multicluster-global-hub/operator/pkg/utils"
	"github.com/stolostron/multicluster-global-hub/pkg/constants"
	"github.com/stolostron/multicluster-global-hub/pkg/jobs"
	"github.com/stolostron/multicluster-global-hub/pkg/utils"
)

func (r *MulticlusterGlobalHubReconciler) pruneGlobalHubResources(ctx context.Context,
	mgh *globalhubv1alpha4.MulticlusterGlobalHub,
) error {
	log := r.Log.WithName("prune")

	// delete addon.open-cluster-management.io/on-multicluster-hub annotation
	if err := r.pruneManagedHubs(ctx); err != nil {
		return fmt.Errorf("failed to delete annotation from the managed cluster: %w", err)
	}

	// delete ClusterManagementAddon firstly to trigger clean up addons.
	if err := r.deleteClusterManagementAddon(ctx); err != nil {
		return fmt.Errorf("failed to delete ClusterManagementAddon: %w", err)
	}
	log.Info("deleted ClusterManagementAddon", "name", operatorconstants.GHClusterManagementAddonName)

	// prune the hub resources until all addons are cleaned up
	if err := r.waitUtilAddonDeleted(ctx, log); err != nil {
		return fmt.Errorf("failed to wait until all addons are deleted: %w", err)
	}
	log.Info("all addons are deleted")

	mgh.SetFinalizers(operatorutils.Remove(mgh.GetFinalizers(), constants.GlobalHubCleanupFinalizer))
	if err := operatorutils.UpdateObject(ctx, r.Client, mgh); err != nil {
		return err
	}

	// clean up namesapced resources, eg. mgh system namespace, etc
	if err := r.pruneNamespacedResources(ctx); err != nil {
		return err
	}

	// clean up the cluster resources, eg. clusterrole, clusterrolebinding, etc
	if err := r.pruneGlobalResources(ctx); err != nil {
		return err
	}

	if err := r.pruneStrimziResources(ctx); err != nil {
		return err
	}

	// clean up metrics resources
	if err := r.pruneMetricsResources(ctx); err != nil {
		return err
	}

	// remove finalizer from app, policy and placement.
	if err := jobs.NewPruneFinalizer(ctx, r.Client).Run(); err != nil {
		return err
	}
	log.Info("removed finalizer from mgh, app, policy, placement and etc")
	return nil
}

func (r *MulticlusterGlobalHubReconciler) pruneMetricsResources(ctx context.Context) error {
	listOpts := []client.ListOption{
		client.HasLabels{constants.GlobalHubMetricsLabel},
	}
	configmapList := &corev1.ConfigMapList{}
	if err := r.Client.List(ctx, configmapList, listOpts...); err != nil {
		return err
	}
	for idx := range configmapList.Items {
		if err := r.Client.Delete(ctx, &configmapList.Items[idx]); err != nil && !errors.IsNotFound(err) {
			return err
		}
	}

	serviceMonitorList := &promv1.ServiceMonitorList{}
	if err := r.Client.List(ctx, serviceMonitorList, listOpts...); err != nil {
		// Handle the env do not have this kind of resource
		return nil
	}
	for idx := range serviceMonitorList.Items {
		if err := r.Client.Delete(ctx, serviceMonitorList.Items[idx]); err != nil && !errors.IsNotFound(err) {
			return err
		}
	}

	podMonitorList := &promv1.PodMonitorList{}
	if err := r.Client.List(ctx, podMonitorList, listOpts...); err != nil {
		// Handle the env do not have this kind of resource
		return nil
	}
	for idx := range podMonitorList.Items {
		if err := r.Client.Delete(ctx, podMonitorList.Items[idx]); err != nil && !errors.IsNotFound(err) {
			return err
		}
	}

	prometheusRuleList := &promv1.PrometheusRuleList{}
	if err := r.Client.List(ctx, prometheusRuleList, listOpts...); err != nil {
		// Handle the env do not have this kind of resource
		return nil
	}
	for idx := range prometheusRuleList.Items {
		if err := r.Client.Delete(ctx, prometheusRuleList.Items[idx]); err != nil && !errors.IsNotFound(err) {
			return err
		}
	}

	return nil
}

// pruneGlobalResources deletes the cluster scoped resources created by the multicluster-global-hub-operator
// cluster scoped resources need to be deleted manually because they don't have ownerrefenence set
func (r *MulticlusterGlobalHubReconciler) pruneGlobalResources(ctx context.Context) error {
	listOpts := []client.ListOption{
		client.MatchingLabels(map[string]string{
			constants.GlobalHubOwnerLabelKey: constants.GHOperatorOwnerLabelVal,
		}),
	}

	clusterRoleList := &rbacv1.ClusterRoleList{}
	if err := r.Client.List(ctx, clusterRoleList, listOpts...); err != nil {
		return err
	}
	for idx := range clusterRoleList.Items {
		if err := r.Client.Delete(ctx, &clusterRoleList.Items[idx]); err != nil && !errors.IsNotFound(err) {
			return err
		}
	}

	clusterRoleBindingList := &rbacv1.ClusterRoleBindingList{}
	if err := r.Client.List(ctx, clusterRoleBindingList, listOpts...); err != nil {
		return err
	}
	for idx := range clusterRoleBindingList.Items {
		if err := r.Client.Delete(ctx, &clusterRoleBindingList.Items[idx]); err != nil && !errors.IsNotFound(err) {
			return err
		}
	}

	clusterManagementAddOnList := &addonv1alpha1.ClusterManagementAddOnList{}
	if err := r.Client.List(ctx, clusterManagementAddOnList, listOpts...); err != nil {
		return err
	}
	for idx := range clusterManagementAddOnList.Items {
		if err := r.Client.Delete(ctx, &clusterManagementAddOnList.Items[idx]); err != nil && !errors.IsNotFound(err) {
			return err
		}
	}

	webhookList := &admissionregistrationv1.MutatingWebhookConfigurationList{}
	if err := r.Client.List(ctx, webhookList, listOpts...); err != nil {
		return err
	}
	for idx := range webhookList.Items {
		if err := r.Client.Delete(ctx, &webhookList.Items[idx]); err != nil && !errors.IsNotFound(err) {
			return err
		}
	}

	return nil
}

// pruneNamespacedResources tries to delete mgh resources
func (r *MulticlusterGlobalHubReconciler) pruneNamespacedResources(ctx context.Context) error {
	mghServiceMonitor := &promv1.ServiceMonitor{
		ObjectMeta: metav1.ObjectMeta{
			Name:      operatorconstants.GHServiceMonitorName,
			Namespace: utils.GetDefaultNamespace(),
			Labels: map[string]string{
				constants.GlobalHubOwnerLabelKey: constants.GHOperatorOwnerLabelVal,
			},
		},
	}
	if err := r.Client.Delete(ctx, mghServiceMonitor); err != nil && !errors.IsNotFound(err) {
		return err
	}

	return nil
}

func (r *MulticlusterGlobalHubReconciler) pruneStrimziResources(ctx context.Context) error {
	klog.Infof("Remove strimzi resources")

	listOpts := []client.ListOption{
		client.MatchingLabels(map[string]string{
			constants.GlobalHubOwnerLabelKey: constants.GlobalHubOwnerLabelVal,
		}),
	}
	kafkaUserList := &kafkav1beta2.KafkaUserList{}
	klog.Infof("Delete kafkaUsers")

	if err := r.Client.List(ctx, kafkaUserList, listOpts...); err != nil {
		return err
	}
	for idx := range kafkaUserList.Items {
		klog.Infof("Delete kafka user %v", kafkaUserList.Items[idx].Name)
		if err := r.Client.Delete(ctx, &kafkaUserList.Items[idx]); err != nil && !errors.IsNotFound(err) {
			return err
		}
	}
	klog.Infof("kafkaUsers deleted")

	kafkaTopicList := &kafkav1beta2.KafkaTopicList{}
	klog.Infof("Delete kafkaTopics")

	if err := r.Client.List(ctx, kafkaTopicList, listOpts...); err != nil {
		return err
	}
	for idx := range kafkaTopicList.Items {
		klog.Infof("Delete kafka topic %v", kafkaTopicList.Items[idx].Name)

		if err := r.Client.Delete(ctx, &kafkaTopicList.Items[idx]); err != nil && !errors.IsNotFound(err) {
			return err
		}
	}
	klog.Infof("kafkaTopic deleted")

	kafka := &kafkav1beta2.Kafka{
		ObjectMeta: metav1.ObjectMeta{
			Name:      transporter.KafkaClusterName,
			Namespace: utils.GetDefaultNamespace(),
		},
	}
	klog.Infof("Delete kafka cluster %v", kafka.Name)

	if err := r.Client.Delete(ctx, kafka); err != nil && !errors.IsNotFound(err) {
		return err
	}
	klog.Infof("kafka cluster deleted")

	kafkaSub := &subv1alpha1.Subscription{}
	err := r.Client.Get(ctx, types.NamespacedName{
		Namespace: utils.GetDefaultNamespace(),
		Name:      transporter.DefaultKafkaSubName,
	}, kafkaSub)
	if err != nil {
		klog.Errorf("Failed to get strimzi subscription, err:%v", err)
		if errors.IsNotFound(err) {
			return nil
		}
		return err
	}

	if kafkaSub.Status.InstalledCSV != "" {
		kafkaCsv := &subv1alpha1.ClusterServiceVersion{
			ObjectMeta: metav1.ObjectMeta{
				Name:      kafkaSub.Status.InstalledCSV,
				Namespace: utils.GetDefaultNamespace(),
			},
		}
		klog.Infof("Delete kafka csv %v", kafkaCsv.Name)
		if err := r.Client.Delete(ctx, kafkaCsv); err != nil {
			return err
		}
		klog.Infof("kafka csv deleted")
	}

	if err := r.Client.Delete(ctx, kafkaSub); err != nil && !errors.IsNotFound(err) {
		return err
	}
	klog.Infof("kafka subscription deleted")

	return nil
}

func (r *MulticlusterGlobalHubReconciler) deleteClusterManagementAddon(ctx context.Context) error {
	clusterManagementAddOn := &addonv1alpha1.ClusterManagementAddOn{
		ObjectMeta: metav1.ObjectMeta{
			Name: operatorconstants.GHClusterManagementAddonName,
		},
	}
	if err := r.Client.Delete(ctx, clusterManagementAddOn); err != nil {
		if errors.IsNotFound(err) {
			return nil
		}
		return err
	}
	return nil
}

func (r *MulticlusterGlobalHubReconciler) waitUtilAddonDeleted(ctx context.Context, log logr.Logger) error {
	ctx, cancel := context.WithTimeout(ctx, 60*time.Second)
	defer cancel()
	if err := wait.PollUntilWithContext(ctx, 3*time.Second, func(ctx context.Context) (done bool, err error) {
		addonList := &addonv1alpha1.ManagedClusterAddOnList{}
		listOptions := []client.ListOption{
			client.MatchingLabels(map[string]string{
				constants.GlobalHubOwnerLabelKey: constants.GHOperatorOwnerLabelVal,
			}),
		}
		if err := r.List(ctx, addonList, listOptions...); err != nil {
			return false, err
		}
		if len(addonList.Items) == 0 {
			return true, nil
		} else {
			log.Info("waiting for managedclusteraddon to be deleted", "addon size", len(addonList.Items))
			return false, nil
		}
	}); err != nil {
		return err
	}
	return nil
}
