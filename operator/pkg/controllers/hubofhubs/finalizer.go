package hubofhubs

import (
	"context"
	"fmt"

	"github.com/go-logr/logr"
	corev1 "k8s.io/api/core/v1"
	rbacv1 "k8s.io/api/rbac/v1"
	"k8s.io/apimachinery/pkg/api/errors"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/types"
	"k8s.io/client-go/restmapper"
	chnv1 "open-cluster-management.io/multicloud-operators-channel/pkg/apis/apps/v1"
	placementrulesv1 "open-cluster-management.io/multicloud-operators-subscription/pkg/apis/apps/placementrule/v1"
	appsubv1 "open-cluster-management.io/multicloud-operators-subscription/pkg/apis/apps/v1"
	"sigs.k8s.io/controller-runtime/pkg/client"
	"sigs.k8s.io/controller-runtime/pkg/controller/controllerutil"

	operatorv1alpha1 "github.com/stolostron/multicluster-global-hub/operator/apis/operator/v1alpha1"
	"github.com/stolostron/multicluster-global-hub/operator/pkg/constants"
	"github.com/stolostron/multicluster-global-hub/operator/pkg/deployer"
	"github.com/stolostron/multicluster-global-hub/operator/pkg/renderer"
	"github.com/stolostron/multicluster-global-hub/operator/pkg/utils"
	commonconstants "github.com/stolostron/multicluster-global-hub/pkg/constants"
)

func (r *MulticlusterGlobalHubReconciler) recocileFinalizer(ctx context.Context,
	mgh *operatorv1alpha1.MulticlusterGlobalHub,
	mghRenderer renderer.Renderer, mghDeployer deployer.Deployer,
	mapper *restmapper.DeferredDiscoveryRESTMapper, log logr.Logger,
) (bool, error) {
	if mgh.GetDeletionTimestamp() != nil && utils.Contains(mgh.GetFinalizers(),
		commonconstants.GlobalHubCleanupFinalizer) {

		// clean up the application finalizer
		if err := r.pruneApplicationFinalizer(ctx, log); err != nil {
			log.Error(err, "failed to remove manager resorces")
			return true, err
		}

		// clean up the cluster resources, eg. clusterrole, clusterrolebinding, etc
		if err := r.pruneGlobalResources(ctx, log); err != nil {
			log.Error(err, "failed to remove global resources")
			return true, err
		}

		// clean up namesapced resources, eg. mgh system namespace, etc
		if err := r.pruneNamespacedResources(ctx, log); err != nil {
			log.Error(err, "failed to remove namespaced resources")
			return true, err
		}

		mgh.SetFinalizers(utils.Remove(mgh.GetFinalizers(),
			commonconstants.GlobalHubCleanupFinalizer))
		if err := r.Client.Update(context.TODO(), mgh); err != nil {
			log.Error(err, "failed to remove finalizer from multiclusterglobalhub resource")
			return true, err
		}
		log.Info("finalizer is removed from multiclusterglobalhub resource")
		return true, nil
	}

	if !utils.Contains(mgh.GetFinalizers(), commonconstants.GlobalHubCleanupFinalizer) {
		mgh.SetFinalizers(append(mgh.GetFinalizers(),
			commonconstants.GlobalHubCleanupFinalizer))
		if err := r.Client.Update(context.TODO(), mgh); err != nil {
			log.Error(err, "failed to add finalizer to multiclusterglobalhub resource")
			return false, err
		}
		log.Info("finalizer is added to multiclusterglobalhub resource")
	}
	return false, nil
}

// pruneGlobalResources deletes the cluster scoped resources created by the multicluster-global-hub-operator
// cluster scoped resources need to be deleted manually because they don't have ownerrefenence set
func (r *MulticlusterGlobalHubReconciler) pruneGlobalResources(ctx context.Context, log logr.Logger) error {
	log.Info("clean up multicluster-global-hub global resources")
	listOpts := []client.ListOption{
		client.MatchingLabels(map[string]string{
			commonconstants.GlobalHubOwnerLabelKey: commonconstants.HoHOperatorOwnerLabelVal,
		}),
	}

	log.Info("clean up the ClusterRole")
	clusterRoleList := &rbacv1.ClusterRoleList{}
	if err := r.Client.List(ctx, clusterRoleList, listOpts...); err != nil && !errors.IsNotFound(err) {
		return err
	}
	for idx := range clusterRoleList.Items {
		if err := r.Client.Delete(ctx, &clusterRoleList.Items[idx]); err != nil && !errors.IsNotFound(err) {
			return err
		}
	}

	log.Info("clean up the ClusterRoleBinding")
	clusterRoleBindingList := &rbacv1.ClusterRoleBindingList{}
	if err := r.Client.List(ctx, clusterRoleBindingList, listOpts...); err != nil && !errors.IsNotFound(err) {
		return err
	}
	for idx := range clusterRoleBindingList.Items {
		if err := r.Client.Delete(ctx, &clusterRoleBindingList.Items[idx]); err != nil && !errors.IsNotFound(err) {
			return err
		}
	}

	log.Info("multicluster-global-hub global resources are cleaned up")
	return nil
}

// pruneNamespacedResources tries to delete mgh resources
func (r *MulticlusterGlobalHubReconciler) pruneNamespacedResources(ctx context.Context, log logr.Logger) error {
	log.Info("clean up multicluster-global-hub namespaced resources")

	// the multicluster-global-hub-config configmap is created by operator and finalized by manager
	log.Info(fmt.Sprintf("clean up the namespace %s configmap %s", constants.HOHSystemNamespace, constants.HOHConfigName))
	existingMghConfigMap := &corev1.ConfigMap{}
	if err := r.Client.Get(ctx,
		types.NamespacedName{
			Namespace: constants.HOHSystemNamespace,
			Name:      constants.HOHConfigName,
		}, existingMghConfigMap); err != nil && !errors.IsNotFound(err) {
		return err
	}
	if err := r.Client.Delete(ctx, existingMghConfigMap); err != nil && !errors.IsNotFound(err) {
		return err
	}

	// clean the finalizers added by multicluster-global-hub-manager
	existingMghConfigMap.SetFinalizers([]string{})
	if err := r.Client.Update(ctx, existingMghConfigMap); err != nil {
		return err
	}
	if err := r.Client.Delete(ctx, existingMghConfigMap); err != nil && !errors.IsNotFound(err) {
		return err
	}

	log.Info(fmt.Sprintf("clean up the namespace %s", constants.HOHSystemNamespace))
	mghSystemNamespace := &corev1.Namespace{
		ObjectMeta: metav1.ObjectMeta{
			Name: constants.HOHSystemNamespace,
			Labels: map[string]string{
				commonconstants.GlobalHubOwnerLabelKey: commonconstants.HoHOperatorOwnerLabelVal,
			},
		},
	}
	if err := r.Client.Delete(ctx, mghSystemNamespace); err != nil && !errors.IsNotFound(err) {
		return err
	}

	log.Info("multicluster-global-hub namespaced resources are cleaned up")
	return nil
}

func (r *MulticlusterGlobalHubReconciler) pruneApplicationFinalizer(ctx context.Context, log logr.Logger) error {
	log.Info("clean up the application subscription finalizer")
	appsubs := &appsubv1.SubscriptionList{}
	if err := r.Client.List(ctx, appsubs, &client.ListOptions{}); err != nil && errors.IsNotFound(err) {
		return err
	}
	for idx := range appsubs.Items {
		if err := r.pruneObjectFinalizer(ctx, &appsubs.Items[idx],
			commonconstants.GlobalHubCleanupFinalizer); err != nil {
			return err
		}
	}

	log.Info("clean up the applicatoin channel finalizer")
	channels := &chnv1.ChannelList{}
	if err := r.Client.List(ctx, channels, &client.ListOptions{}); err != nil && errors.IsNotFound(err) {
		return err
	}
	for idx := range channels.Items {
		if err := r.pruneObjectFinalizer(ctx, &channels.Items[idx],
			commonconstants.GlobalHubCleanupFinalizer); err != nil {
			return err
		}
	}

	log.Info("clean up the application placementrule finalizer")
	palcementrules := &placementrulesv1.PlacementRuleList{}
	if err := r.Client.List(ctx, palcementrules, &client.ListOptions{}); err != nil && errors.IsNotFound(err) {
		return err
	}
	for idx := range palcementrules.Items {
		if err := r.pruneObjectFinalizer(ctx, &palcementrules.Items[idx],
			commonconstants.GlobalHubCleanupFinalizer); err != nil {
			return err
		}
	}

	log.Info("multicluster-global-hub manager resources are cleaned up")
	return nil
}

func (r *MulticlusterGlobalHubReconciler) pruneObjectFinalizer(ctx context.Context, instance client.Object,
	finalizer string,
) error {
	if controllerutil.ContainsFinalizer(instance, finalizer) {
		controllerutil.RemoveFinalizer(instance, finalizer)
		if err := r.Client.Update(ctx, instance); err != nil {
			return err
		}
	}
	return nil
}
