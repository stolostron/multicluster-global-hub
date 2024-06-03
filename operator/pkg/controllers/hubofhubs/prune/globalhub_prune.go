package prune

import (
	"context"
	"fmt"
	"time"

	"github.com/go-logr/logr"
	promv1 "github.com/prometheus-operator/prometheus-operator/pkg/apis/monitoring/v1"
	admissionregistrationv1 "k8s.io/api/admissionregistration/v1"
	corev1 "k8s.io/api/core/v1"
	rbacv1 "k8s.io/api/rbac/v1"
	"k8s.io/apimachinery/pkg/api/equality"
	"k8s.io/apimachinery/pkg/api/errors"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/util/wait"
	"k8s.io/klog"
	addonv1alpha1 "open-cluster-management.io/api/addon/v1alpha1"
	clusterv1 "open-cluster-management.io/api/cluster/v1"
	"sigs.k8s.io/controller-runtime/pkg/client"
	"sigs.k8s.io/controller-runtime/pkg/controller/controllerutil"

	globalhubv1alpha4 "github.com/stolostron/multicluster-global-hub/operator/apis/v1alpha4"
	"github.com/stolostron/multicluster-global-hub/operator/pkg/config"
	operatorconstants "github.com/stolostron/multicluster-global-hub/operator/pkg/constants"
	operatorutils "github.com/stolostron/multicluster-global-hub/operator/pkg/utils"
	"github.com/stolostron/multicluster-global-hub/pkg/constants"
	"github.com/stolostron/multicluster-global-hub/pkg/jobs"
	"github.com/stolostron/multicluster-global-hub/pkg/utils"
)

type PureReconciler struct {
	client.Client
	log logr.Logger
}

func (r *PureReconciler) Reconcile(ctx context.Context,
	mgh *globalhubv1alpha4.MulticlusterGlobalHub,
) error {
	// Deleting the multiclusterglobalhub instance
	if mgh.GetDeletionTimestamp() != nil &&
		operatorutils.Contains(mgh.GetFinalizers(), constants.GlobalHubCleanupFinalizer) {
		if err := r.GlobalHubResources(ctx, mgh); err != nil {
			return fmt.Errorf("failed to prune Global Hub resources %v", err)
		}
		return nil
	}

	// reconcile metrics
	if config.IsBYOKafka() && config.IsBYOPostgres() {
		mgh.Spec.EnableMetrics = false
		klog.Info("Kafka and Postgres are provided by customer, disable metrics")
		if err := r.MetricsResources(ctx); err != nil {
			return err
		}
	}
	return nil
}

func (r *PureReconciler) GlobalHubResources(ctx context.Context,
	mgh *globalhubv1alpha4.MulticlusterGlobalHub,
) error {
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

	// remove finalizer from app, policy and placement.
	if err := jobs.NewPruneFinalizer(ctx, r.Client).Run(); err != nil {
		return err
	}
	r.log.Info("removed finalizer from mgh, app, policy, placement and etc")
	return nil
}

func (r *PureReconciler) MetricsResources(ctx context.Context) error {
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
func (r *PureReconciler) pruneGlobalResources(ctx context.Context) error {
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
func (r *PureReconciler) pruneNamespacedResources(ctx context.Context) error {
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

func (r *PureReconciler) deleteClusterManagementAddon(ctx context.Context) error {
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

func (r *PureReconciler) waitUtilAddonDeleted(ctx context.Context, log logr.Logger) error {
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

func (r *PureReconciler) pruneManagedHubs(ctx context.Context) error {
	clusters := &clusterv1.ManagedClusterList{}
	if err := r.List(ctx, clusters, &client.ListOptions{}); err != nil {
		return err
	}

	for idx, managedHub := range clusters.Items {
		if managedHub.Name == operatorconstants.LocalClusterName {
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
