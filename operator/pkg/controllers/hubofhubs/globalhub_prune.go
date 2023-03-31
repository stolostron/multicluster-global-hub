package hubofhubs

import (
	"context"
	"fmt"
	"time"

	"github.com/go-logr/logr"
	admissionregistrationv1 "k8s.io/api/admissionregistration/v1"
	corev1 "k8s.io/api/core/v1"
	rbacv1 "k8s.io/api/rbac/v1"
	"k8s.io/apimachinery/pkg/api/errors"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/types"
	"k8s.io/apimachinery/pkg/util/wait"
	addonv1alpha1 "open-cluster-management.io/api/addon/v1alpha1"
	"sigs.k8s.io/controller-runtime/pkg/client"
	ctrllog "sigs.k8s.io/controller-runtime/pkg/log"

	operatorv1alpha2 "github.com/stolostron/multicluster-global-hub/operator/apis/v1alpha2"
	operatorconstants "github.com/stolostron/multicluster-global-hub/operator/pkg/constants"
	"github.com/stolostron/multicluster-global-hub/operator/pkg/utils"
	"github.com/stolostron/multicluster-global-hub/pkg/constants"
	"github.com/stolostron/multicluster-global-hub/pkg/jobs"
)

func (r *MulticlusterGlobalHubReconciler) pruneGlobalHubResources(ctx context.Context,
	mgh *operatorv1alpha2.MulticlusterGlobalHub,
) error {
	log := ctrllog.FromContext(ctx)
	log.Info("pruning global hub resources")

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

	mgh.SetFinalizers(utils.Remove(mgh.GetFinalizers(), constants.GlobalHubCleanupFinalizer))
	if err := utils.UpdateObject(ctx, r.Client, mgh); err != nil {
		return err
	}
	log.Info("removed finalizer from mgh")

	// clean up namesapced resources, eg. mgh system namespace, etc
	if err := r.pruneNamespacedResources(ctx); err != nil {
		return err
	}
	log.Info("pruned namespaced resources")

	// clean up the cluster resources, eg. clusterrole, clusterrolebinding, etc
	if err := r.pruneGlobalResources(ctx); err != nil {
		return err
	}
	log.Info("pruned global resources")

	// remove finalizer from app, policy and placement.
	if err := jobs.NewPruneFinalizer(ctx, r.Client).Run(); err != nil {
		return err
	}
	log.Info("removed finalizer from app, policy, placement and etc")
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
	existingMghConfigMap := &corev1.ConfigMap{}
	err := r.Client.Get(ctx,
		types.NamespacedName{
			Namespace: constants.GHSystemNamespace,
			Name:      constants.GHConfigCMName,
		}, existingMghConfigMap)
	if err != nil && !errors.IsNotFound(err) {
		return err
	} else if err == nil {
		// clean the finalizers added by multicluster-global-hub-manager
		existingMghConfigMap.SetFinalizers([]string{})
		if err := utils.UpdateObject(ctx, r.Client, existingMghConfigMap); err != nil {
			return err
		}
		if err := r.Client.Delete(ctx, existingMghConfigMap); err != nil && !errors.IsNotFound(err) {
			return err
		}
	}

	mghSystemNamespace := &corev1.Namespace{
		ObjectMeta: metav1.ObjectMeta{
			Name: constants.GHSystemNamespace,
			Labels: map[string]string{
				constants.GlobalHubOwnerLabelKey: constants.GHOperatorOwnerLabelVal,
			},
		},
	}
	if err := r.Client.Delete(ctx, mghSystemNamespace); err != nil && !errors.IsNotFound(err) {
		return err
	}

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
