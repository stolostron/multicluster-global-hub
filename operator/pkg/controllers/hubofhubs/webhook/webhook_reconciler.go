package webhook

import (
	"context"
	"embed"
	"fmt"

	"k8s.io/client-go/discovery"
	"k8s.io/client-go/discovery/cached/memory"
	"k8s.io/client-go/restmapper"
	ctrl "sigs.k8s.io/controller-runtime"

	"github.com/stolostron/multicluster-global-hub/operator/api/operator/v1alpha4"
	"github.com/stolostron/multicluster-global-hub/operator/pkg/config"
	"github.com/stolostron/multicluster-global-hub/operator/pkg/deployer"
	"github.com/stolostron/multicluster-global-hub/operator/pkg/renderer"
	"github.com/stolostron/multicluster-global-hub/operator/pkg/utils"
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
