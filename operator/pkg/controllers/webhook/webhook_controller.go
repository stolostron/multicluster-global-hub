package webhook

import (
	"context"
	"embed"
	"fmt"
	"reflect"

	"k8s.io/client-go/discovery"
	"k8s.io/client-go/discovery/cached/memory"
	"k8s.io/client-go/restmapper"
	ctrl "sigs.k8s.io/controller-runtime"
	"sigs.k8s.io/controller-runtime/pkg/controller"
	"sigs.k8s.io/controller-runtime/pkg/event"
	"sigs.k8s.io/controller-runtime/pkg/handler"
	"sigs.k8s.io/controller-runtime/pkg/predicate"
	"sigs.k8s.io/controller-runtime/pkg/source"

	globalhubv1alpha4 "github.com/stolostron/multicluster-global-hub/operator/api/operator/v1alpha4"
	"github.com/stolostron/multicluster-global-hub/operator/pkg/config"
	"github.com/stolostron/multicluster-global-hub/operator/pkg/deployer"
	"github.com/stolostron/multicluster-global-hub/operator/pkg/renderer"
	"github.com/stolostron/multicluster-global-hub/operator/pkg/utils"
)

//go:embed manifests
var fs embed.FS

var startedWebhookController = false

type WebhookReconciler struct {
	ctrl.Manager
	mgh *globalhubv1alpha4.MulticlusterGlobalHub
}

func NewWebhookReconciler(mgr ctrl.Manager,
) *WebhookReconciler {
	return &WebhookReconciler{
		Manager: mgr,
	}
}

func StartWebhookController(opts config.ControllerOption) error {
	if startedWebhookController {
		return nil
	}
	r := &WebhookReconciler{
		Manager: opts.Manager,
		mgh:     opts.MulticlusterGlobalHub,
	}
	if err := r.SetupWithManager(opts.Manager); err != nil {
		return err
	}
	startedWebhookController = true
	return nil
}

func (r *WebhookReconciler) Reconcile(ctx context.Context, req ctrl.Request) (ctrl.Result, error) {
	// create new HoHRenderer and HoHDeployer
	hohRenderer, hohDeployer := renderer.NewHoHRenderer(fs), deployer.NewHoHDeployer(r.GetClient())

	// create discovery client
	dc, err := discovery.NewDiscoveryClientForConfig(r.Manager.GetConfig())
	if err != nil {
		return ctrl.Result{}, err
	}

	// create restmapper for deployer to find GVR
	mapper := restmapper.NewDeferredDiscoveryRESTMapper(memory.NewMemCacheClient(dc))

	webhookObjects, err := hohRenderer.Render("manifests", "", func(profile string) (interface{}, error) {
		return WebhookVariables{
			ImportClusterInHosted: config.GetImportClusterInHosted(),
			Namespace:             r.mgh.Namespace,
		}, nil
	})
	if err != nil {
		return ctrl.Result{}, fmt.Errorf("failed to render webhook objects: %v", err)
	}
	if err = utils.ManipulateGlobalHubObjects(webhookObjects, r.mgh, hohDeployer, mapper, r.GetScheme()); err != nil {
		return ctrl.Result{}, fmt.Errorf("failed to create/update webhook objects: %v", err)
	}
	return ctrl.Result{}, nil
}

type WebhookVariables struct {
	ImportClusterInHosted bool
	Namespace             string
}

// SetupWithManager sets up the controller with the Manager.
func (r *WebhookReconciler) SetupWithManager(mgr ctrl.Manager) error {
	c, err := controller.New("webhook-controller", mgr, controller.Options{
		Reconciler: r,
	})
	if err != nil {
		return err
	}
	return c.Watch(source.Kind(mgr.GetCache(), &globalhubv1alpha4.MulticlusterGlobalHub{},
		&handler.TypedEnqueueRequestForObject[*globalhubv1alpha4.MulticlusterGlobalHub]{},
		predicate.TypedFuncs[*globalhubv1alpha4.MulticlusterGlobalHub]{
			CreateFunc: func(e event.TypedCreateEvent[*globalhubv1alpha4.MulticlusterGlobalHub]) bool {
				return true
			},
			UpdateFunc: func(e event.TypedUpdateEvent[*globalhubv1alpha4.MulticlusterGlobalHub]) bool {
				if e.ObjectOld.GetGeneration() != e.ObjectNew.GetGeneration() {
					return true
				}
				return !reflect.DeepEqual(e.ObjectOld.GetAnnotations(), e.ObjectNew.GetAnnotations())
			},
			DeleteFunc: func(e event.TypedDeleteEvent[*globalhubv1alpha4.MulticlusterGlobalHub]) bool {
				return false
			},
		},
	))
}
