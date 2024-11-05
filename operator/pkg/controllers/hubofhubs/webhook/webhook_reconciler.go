package webhook

import (
	"context"
	"embed"
	"fmt"

	admissionregistrationv1 "k8s.io/api/admissionregistration/v1"
	corev1 "k8s.io/api/core/v1"
	"k8s.io/apimachinery/pkg/api/errors"
	"k8s.io/client-go/discovery"
	"k8s.io/client-go/discovery/cached/memory"
	"k8s.io/client-go/restmapper"
	ctrl "sigs.k8s.io/controller-runtime"
	"sigs.k8s.io/controller-runtime/pkg/client"

	"github.com/stolostron/multicluster-global-hub/operator/api/operator/v1alpha4"
	"github.com/stolostron/multicluster-global-hub/operator/pkg/config"
	"github.com/stolostron/multicluster-global-hub/operator/pkg/deployer"
	"github.com/stolostron/multicluster-global-hub/operator/pkg/renderer"
	"github.com/stolostron/multicluster-global-hub/operator/pkg/utils"
	"github.com/stolostron/multicluster-global-hub/pkg/constants"
)

//go:embed manifests
var fs embed.FS

type WebhookReconciler struct {
	ctrl.Manager
}

func NewWebhookReconciler(mgr ctrl.Manager,
) *WebhookReconciler {
	return &WebhookReconciler{
		Manager: mgr,
	}
}

func (r *WebhookReconciler) Reconcile(ctx context.Context,
	mgh *v1alpha4.MulticlusterGlobalHub,
) error {
	if mgh.DeletionTimestamp != nil || !config.GetImportClusterInHosted() {
		return r.pruneWebhookResources(ctx)
	}
	// create new HoHRenderer and HoHDeployer
	hohRenderer, hohDeployer := renderer.NewHoHRenderer(fs), deployer.NewHoHDeployer(r.GetClient())

	// create discovery client
	dc, err := discovery.NewDiscoveryClientForConfig(r.Manager.GetConfig())
	if err != nil {
		return err
	}

	// create restmapper for deployer to find GVR
	mapper := restmapper.NewDeferredDiscoveryRESTMapper(memory.NewMemCacheClient(dc))

	webhookObjects, err := hohRenderer.Render("manifests", "", func(profile string) (interface{}, error) {
		return WebhookVariables{
			ImportClusterInHosted: config.GetImportClusterInHosted(),
			Namespace:             mgh.Namespace,
		}, nil
	})
	if err != nil {
		return fmt.Errorf("failed to render webhook objects: %v", err)
	}
	if err = utils.ManipulateGlobalHubObjects(webhookObjects, mgh, hohDeployer, mapper, r.GetScheme()); err != nil {
		return fmt.Errorf("failed to create/update webhook objects: %v", err)
	}
	return nil
}

type WebhookVariables struct {
	ImportClusterInHosted bool
	Namespace             string
}

func (r *WebhookReconciler) pruneWebhookResources(ctx context.Context) error {
	listOpts := []client.ListOption{
		client.MatchingLabels(map[string]string{
			constants.GlobalHubOwnerLabelKey: constants.GHOperatorOwnerLabelVal,
		}),
	}
	webhookList := &admissionregistrationv1.MutatingWebhookConfigurationList{}
	if err := r.GetClient().List(ctx, webhookList, listOpts...); err != nil {
		return err
	}

	for idx := range webhookList.Items {
		if err := r.GetClient().Delete(ctx, &webhookList.Items[idx]); err != nil && !errors.IsNotFound(err) {
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
	if err := r.GetClient().List(ctx, webhookServiceList, webhookServiceListOpts...); err != nil {
		return err
	}
	for idx := range webhookServiceList.Items {
		if err := r.GetClient().Delete(ctx, &webhookServiceList.Items[idx]); err != nil && !errors.IsNotFound(err) {
			return err
		}
	}
	return nil
}
