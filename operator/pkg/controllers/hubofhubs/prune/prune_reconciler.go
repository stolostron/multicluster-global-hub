package prune

import (
	"context"
	"fmt"
	"reflect"
	"time"

	kafkav1beta2 "github.com/RedHatInsights/strimzi-client-go/apis/kafka.strimzi.io/v1beta2"
	"github.com/go-logr/logr"
	subv1alpha1 "github.com/operator-framework/api/pkg/operators/v1alpha1"
	promv1 "github.com/prometheus-operator/prometheus-operator/pkg/apis/monitoring/v1"
	admissionregistrationv1 "k8s.io/api/admissionregistration/v1"
	corev1 "k8s.io/api/core/v1"
	rbacv1 "k8s.io/api/rbac/v1"
	"k8s.io/apimachinery/pkg/api/equality"
	"k8s.io/apimachinery/pkg/api/errors"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/types"
	"k8s.io/apimachinery/pkg/util/wait"
	"k8s.io/klog"
	"open-cluster-management.io/api/addon/v1alpha1"
	addonv1alpha1 "open-cluster-management.io/api/addon/v1alpha1"
	clusterv1 "open-cluster-management.io/api/cluster/v1"
	ctrl "sigs.k8s.io/controller-runtime"
	"sigs.k8s.io/controller-runtime/pkg/client"
	"sigs.k8s.io/controller-runtime/pkg/controller/controllerutil"

	globalhubv1alpha4 "github.com/stolostron/multicluster-global-hub/operator/api/operator/v1alpha4"
	"github.com/stolostron/multicluster-global-hub/operator/pkg/config"
	operatorconstants "github.com/stolostron/multicluster-global-hub/operator/pkg/constants"
	addonController "github.com/stolostron/multicluster-global-hub/operator/pkg/controllers/addons"
	"github.com/stolostron/multicluster-global-hub/operator/pkg/controllers/transporter/protocol"
	operatorutils "github.com/stolostron/multicluster-global-hub/operator/pkg/utils"
	"github.com/stolostron/multicluster-global-hub/pkg/constants"
	"github.com/stolostron/multicluster-global-hub/pkg/jobs"
	"github.com/stolostron/multicluster-global-hub/pkg/utils"
)

type PruneReconciler struct {
	client.Client
	log logr.Logger
}

func NewPruneReconciler(c client.Client) *PruneReconciler {
	return &PruneReconciler{
		log:    ctrl.Log.WithName("global-hub-prune"),
		Client: c,
	}
}

func (r *PruneReconciler) Reconcile(ctx context.Context,
	mgh *globalhubv1alpha4.MulticlusterGlobalHub,
) (bool, error) {
	// Deleting the multiclusterglobalhub instance
	if mgh.GetDeletionTimestamp() != nil &&
		operatorutils.Contains(mgh.GetFinalizers(), constants.GlobalHubCleanupFinalizer) {
		if config.GetImportClusterInHosted() {
			hasmanagedHub, err := r.hasManagedHub(ctx)
			if err != nil {
				return true, err
			}
			if hasmanagedHub {
				klog.Errorf("You need to detach all the managed hub clusters before uninstalling")
				return true, nil
			}
		}

		if err := r.revertClusterManagementAddon(ctx); err != nil {
			return true, err
		}

		if err := r.pruneWebhookResources(ctx); err != nil {
			return true, err
		}
		needRequeue, err := r.GlobalHubResources(ctx, mgh)
		if err != nil {
			return true, fmt.Errorf("failed to prune Global Hub resources %v", err)
		}
		if needRequeue {
			return true, nil
		}
		if err := r.MetricsResources(ctx); err != nil {
			return true, err
		}
		// clean the kafka connection so that the manager can use the new one after reinstall
		config.SetTransporterConn(nil)
		return false, nil
	}
	// If webhook do not need to enable, should remove the related resources
	if config.IsACMResourceReady() && !config.GetImportClusterInHosted() {
		if err := r.pruneWebhookResources(ctx); err != nil {
			return true, err
		}
		if err := r.pruneHostedResources(ctx); err != nil {
			return true, err
		}
	}

	// reconcile metrics
	if config.IsBYOKafka() && config.IsBYOPostgres() {
		mgh.Spec.EnableMetrics = false
		klog.Info("Kafka and Postgres are provided by customer, disable metrics")
		if err := r.MetricsResources(ctx); err != nil {
			return true, err
		}
	}
	return false, nil
}

func (r *PruneReconciler) hasManagedHub(ctx context.Context) (bool, error) {
	mcaList := &v1alpha1.ManagedClusterAddOnList{}
	err := r.Client.List(ctx, mcaList)
	if err != nil {
		return false, err
	}
	for _, mca := range mcaList.Items {
		if mca.Name == operatorconstants.GHManagedClusterAddonName {
			return true, nil
		}
	}

	return false, nil
}

func (r *PruneReconciler) revertClusterManagementAddon(ctx context.Context) error {
	if !config.IsACMResourceReady() {
		return nil
	}
	cmaList := &v1alpha1.ClusterManagementAddOnList{}
	err := r.Client.List(ctx, cmaList)
	if err != nil {
		return err
	}
	for _, cma := range cmaList.Items {
		if !addonController.AddonList.Has(cma.Name) {
			continue
		}
		err = r.removeGlobalhubConfig(ctx, cma)
		if err != nil {
			return err
		}
	}
	return nil
}

func (r *PruneReconciler) removeGlobalhubConfig(ctx context.Context, cma v1alpha1.ClusterManagementAddOn) error {
	if len(cma.Spec.InstallStrategy.Placements) == 0 {
		return nil
	}
	var desiredPlacementStrategy []addonv1alpha1.PlacementStrategy
	exist := false
	for _, pl := range cma.Spec.InstallStrategy.Placements {
		if reflect.DeepEqual(pl.PlacementRef, addonController.GlobalhubCmaConfig.PlacementRef) {
			exist = true
			continue
		}
		desiredPlacementStrategy = append(desiredPlacementStrategy, pl)
	}
	if !exist {
		return nil
	}
	cma.Spec.InstallStrategy.Placements = desiredPlacementStrategy
	return r.Client.Update(ctx, &cma)
}

func (r *PruneReconciler) pruneHostedResources(ctx context.Context) error {
	addonDeployConfig := &addonv1alpha1.AddOnDeploymentConfig{}
	if err := r.Client.Get(ctx, types.NamespacedName{
		Namespace: utils.GetDefaultNamespace(),
		Name:      "global-hub",
	}, addonDeployConfig); err != nil {
		if errors.IsNotFound(err) {
			return nil
		}
		return err
	}
	return r.Client.Delete(ctx, addonDeployConfig)
}

func (r *PruneReconciler) pruneWebhookResources(ctx context.Context) error {
	listOpts := []client.ListOption{
		client.MatchingLabels(map[string]string{
			constants.GlobalHubOwnerLabelKey: constants.GHOperatorOwnerLabelVal,
		}),
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

	webhookServiceListOpts := []client.ListOption{
		client.MatchingLabels(map[string]string{
			constants.GlobalHubOwnerLabelKey: constants.GHOperatorOwnerLabelVal,
			"service":                        "multicluster-global-hub-webhook",
		}),
	}
	webhookServiceList := &corev1.ServiceList{}
	if err := r.Client.List(ctx, webhookServiceList, webhookServiceListOpts...); err != nil {
		return err
	}
	for idx := range webhookServiceList.Items {
		if err := r.Client.Delete(ctx, &webhookServiceList.Items[idx]); err != nil && !errors.IsNotFound(err) {
			return err
		}
	}
	return nil
}

func (r *PruneReconciler) pruneACMResources(ctx context.Context) error {
	// delete addon.open-cluster-management.io/on-multicluster-hub annotation
	if err := r.pruneManagedHubs(ctx); err != nil {
		return fmt.Errorf("failed to delete annotation from the managed cluster: %w", err)
	}

	// delete ClusterManagementAddon firstly to trigger clean up addons.
	if err := r.deleteClusterManagementAddon(ctx); err != nil {
		return fmt.Errorf("failed to delete ClusterManagementAddon: %w", err)
	}
	r.log.Info("deleted ClusterManagementAddon", "name", operatorconstants.GHClusterManagementAddonName)

	// prune the hub resources until all addons are cleaned up
	if err := r.waitUtilAddonDeleted(ctx, r.log); err != nil {
		return fmt.Errorf("failed to wait until all addons are deleted: %w", err)
	}
	r.log.Info("all addons are deleted")

	return nil
}

func (r *PruneReconciler) GlobalHubResources(ctx context.Context,
	mgh *globalhubv1alpha4.MulticlusterGlobalHub,
) (bool, error) {
	if config.IsACMResourceReady() {
		if err := r.pruneACMResources(ctx); err != nil {
			return true, err
		}
	}

	if !config.IsBYOKafka() {
		needRequeue, err := r.pruneStrimziResources(ctx)
		if err != nil {
			return true, err
		}
		if needRequeue {
			return true, nil
		}
	}

	mgh.SetFinalizers(operatorutils.Remove(mgh.GetFinalizers(), constants.GlobalHubCleanupFinalizer))
	if err := operatorutils.UpdateObject(ctx, r.Client, mgh); err != nil {
		return true, err
	}

	// clean up namesapced resources, eg. mgh system namespace, etc
	if err := r.pruneNamespacedResources(ctx); err != nil {
		return true, err
	}

	// clean up the cluster resources, eg. clusterrole, clusterrolebinding, etc
	if err := r.pruneGlobalResources(ctx); err != nil {
		return true, err
	}

	if config.IsACMResourceReady() {
		// remove finalizer from app, policy and placement.
		// the finalizer is added by the global hub manager. ideally, they should be pruned by manager
		// But currently, we do not have a channel from operator to let manager knows when to start pruning.
		if err := jobs.NewPruneFinalizer(ctx, r.Client).Run(); err != nil {
			return true, err
		}
		r.log.Info("removed finalizer from mgh, app, policy, placement and etc")
	}

	return false, nil
}

func (r *PruneReconciler) MetricsResources(ctx context.Context) error {
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
func (r *PruneReconciler) pruneGlobalResources(ctx context.Context) error {
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
func (r *PruneReconciler) pruneNamespacedResources(ctx context.Context) error {
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

func (r *PruneReconciler) deleteClusterManagementAddon(ctx context.Context) error {
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

func (r *PruneReconciler) waitUtilAddonDeleted(ctx context.Context, log logr.Logger) error {
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

func (r *PruneReconciler) pruneManagedHubs(ctx context.Context) error {
	clusters := &clusterv1.ManagedClusterList{}
	if err := r.List(ctx, clusters, &client.ListOptions{}); err != nil {
		return err
	}

	for idx, managedHub := range clusters.Items {
		if managedHub.Name == constants.LocalClusterName {
			continue
		}
		orgAnnotations := managedHub.GetAnnotations()
		if orgAnnotations == nil {
			continue
		}
		annotations := make(map[string]string, len(orgAnnotations))
		operatorutils.CopyMap(annotations, managedHub.GetAnnotations())

		delete(orgAnnotations, operatorconstants.AnnotationONMulticlusterHub)
		delete(orgAnnotations, operatorconstants.AnnotationPolicyONMulticlusterHub)
		_ = controllerutil.RemoveFinalizer(&clusters.Items[idx], constants.GlobalHubCleanupFinalizer)
		if !equality.Semantic.DeepEqual(annotations, orgAnnotations) {
			if err := r.Update(ctx, &clusters.Items[idx], &client.UpdateOptions{}); err != nil {
				return err
			}
		}
	}

	return nil
}

func (r *PruneReconciler) pruneStrimziResources(ctx context.Context) (bool, error) {
	klog.Infof("Remove strimzi resources")
	listOpts := []client.ListOption{
		client.HasLabels{constants.GlobalHubOwnerLabelKey},
	}
	kafkaUserList := &kafkav1beta2.KafkaUserList{}
	klog.Infof("Delete kafkaUsers")
	if err := r.Client.List(ctx, kafkaUserList, listOpts...); err != nil {
		return true, err
	}
	for idx := range kafkaUserList.Items {
		klog.Infof("Delete kafka user %v", kafkaUserList.Items[idx].Name)
		if err := r.Client.Delete(ctx, &kafkaUserList.Items[idx]); err != nil && !errors.IsNotFound(err) {
			return true, err
		}
	}

	kafkaTopicList := &kafkav1beta2.KafkaTopicList{}
	klog.Infof("Delete kafkaTopics")

	if err := r.Client.List(ctx, kafkaTopicList, listOpts...); err != nil {
		return true, err
	}
	for idx := range kafkaTopicList.Items {
		klog.Infof("Delete kafka topic %v", kafkaTopicList.Items[idx].Name)
		if err := r.Client.Delete(ctx, &kafkaTopicList.Items[idx]); err != nil && !errors.IsNotFound(err) {
			return true, err
		}
	}
	// Wait kafkatopic is removed
	if err := r.Client.List(ctx, kafkaTopicList, listOpts...); err != nil {
		return true, err
	}

	if len(kafkaTopicList.Items) != 0 {
		return true, nil
	}

	kafka := &kafkav1beta2.Kafka{
		ObjectMeta: metav1.ObjectMeta{
			Name:      protocol.KafkaClusterName,
			Namespace: utils.GetDefaultNamespace(),
		},
	}
	klog.Infof("Delete kafka cluster %v", kafka.Name)

	if err := r.Client.Delete(ctx, kafka); err != nil && !errors.IsNotFound(err) {
		return true, err
	}
	klog.Infof("kafka cluster deleted")

	kafkaSub := &subv1alpha1.Subscription{}
	err := r.Client.Get(ctx, types.NamespacedName{
		Namespace: utils.GetDefaultNamespace(),
		Name:      protocol.DefaultKafkaSubName,
	}, kafkaSub)
	if err != nil {
		klog.Errorf("Failed to get strimzi subscription, err:%v", err)
		if errors.IsNotFound(err) {
			return false, nil
		}
		return true, err
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
			return true, err
		}
		klog.Infof("kafka csv deleted")
	}

	if err := r.Client.Delete(ctx, kafkaSub); err != nil && !errors.IsNotFound(err) {
		return true, err
	}
	klog.Infof("kafka subscription deleted")

	return false, nil
}
