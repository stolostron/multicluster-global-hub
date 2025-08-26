package webhook

import (
	"context"
	"embed"
	"fmt"
	"reflect"

	admissionregistrationv1 "k8s.io/api/admissionregistration/v1"
	corev1 "k8s.io/api/core/v1"
	"k8s.io/apimachinery/pkg/api/errors"
	"k8s.io/client-go/discovery"
	"k8s.io/client-go/discovery/cached/memory"
	"k8s.io/client-go/restmapper"
	addonv1alpha1 "open-cluster-management.io/api/addon/v1alpha1"
	clusterv1beta1 "open-cluster-management.io/api/cluster/v1beta1"
	clusterv1beta2 "open-cluster-management.io/api/cluster/v1beta2"
	ctrl "sigs.k8s.io/controller-runtime"
	"sigs.k8s.io/controller-runtime/pkg/builder"
	"sigs.k8s.io/controller-runtime/pkg/client"
	"sigs.k8s.io/controller-runtime/pkg/event"
	"sigs.k8s.io/controller-runtime/pkg/handler"
	"sigs.k8s.io/controller-runtime/pkg/predicate"

	"github.com/stolostron/multicluster-global-hub/operator/api/operator/v1alpha4"
	"github.com/stolostron/multicluster-global-hub/operator/pkg/config"
	"github.com/stolostron/multicluster-global-hub/operator/pkg/deployer"
	"github.com/stolostron/multicluster-global-hub/operator/pkg/renderer"
	"github.com/stolostron/multicluster-global-hub/operator/pkg/utils"
	"github.com/stolostron/multicluster-global-hub/pkg/constants"
	"github.com/stolostron/multicluster-global-hub/pkg/logger"
)

// +kubebuilder:rbac:groups=operator.open-cluster-management.io,resources=multiclusterglobalhubs,verbs=get;list;watch;
// +kubebuilder:rbac:groups=addon.open-cluster-management.io,resources=addondeploymentconfigs,verbs=create;delete;get;list;update;watch
// +kubebuilder:rbac:groups=cluster.open-cluster-management.io,resources=managedclustersetbindings,verbs=create;get;list;patch;update;delete
// +kubebuilder:rbac:groups="admissionregistration.k8s.io",resources=mutatingwebhookconfigurations,verbs=get;list;watch;create;update;delete
// +kubebuilder:rbac:groups=cluster.open-cluster-management.io,resources=placements,verbs=create;get;list;patch;update;delete
// +kubebuilder:rbac:groups="",resources=services,verbs=get;list;watch;create;update;delete
// +kubebuilder:rbac:groups=cluster.open-cluster-management.io,resources=managedclustersets/bind,resourceNames=global,verbs=create;delete

//go:embed manifests
var fs embed.FS

var (
	log               = logger.DefaultZapLogger()
	isResourceRemoved = true
	webhookReconciler *WebhookReconciler
)

type WebhookReconciler struct {
	ctrl.Manager
	c client.Client
}

func StartController(opts config.ControllerOption) (config.ControllerInterface, error) {
	if webhookReconciler != nil {
		return webhookReconciler, nil
	}
	log.Info("start webhook controller")

	webhookReconciler = &WebhookReconciler{
		c:       opts.Manager.GetClient(),
		Manager: opts.Manager,
	}
	if err := webhookReconciler.SetupWithManager(opts.Manager); err != nil {
		webhookReconciler = nil
		return nil, err
	}
	log.Infof("inited webhook controller")
	return webhookReconciler, nil
}

func (r *WebhookReconciler) IsResourceRemoved() bool {
	log.Infof("webhookController resource removed: %v", isResourceRemoved)
	return isResourceRemoved
}

// TODO: move the hosted resource creation/rendering under the controllers/agent/addon/manifests/templates/hostedagent
// Create the MutatingWebhookConfiguration, which specified the resources need to be handled by the webhook server
func (r *WebhookReconciler) Reconcile(ctx context.Context, req ctrl.Request) (ctrl.Result, error) {
	log.Debugf("reconcile webhook controller")
	mgh, err := config.GetMulticlusterGlobalHub(ctx, r.c)
	if err != nil {
		return ctrl.Result{}, err
	}
	if mgh == nil || config.IsPaused(mgh) {
		return ctrl.Result{}, nil
	}

	if mgh.DeletionTimestamp != nil {
		log.Debugf("deleting mgh in webhook controller")
		err = utils.HandleMghDelete(ctx, &isResourceRemoved, mgh.Namespace, r.pruneWebhookResources)
		log.Debugf("deleted webhook resources, isResourceRemoved:%v", isResourceRemoved)
		return ctrl.Result{}, err
	}
	isResourceRemoved = false
	// create new HoHRenderer and HoHDeployer
	hohRenderer, hohDeployer := renderer.NewHoHRenderer(fs), deployer.NewHoHDeployer(r.c)

	// create discovery client
	dc, err := discovery.NewDiscoveryClientForConfig(r.GetConfig())
	if err != nil {
		return ctrl.Result{}, err
	}

	// create restmapper for deployer to find GVR
	mapper := restmapper.NewDeferredDiscoveryRESTMapper(memory.NewMemCacheClient(dc))

	webhookObjects, err := hohRenderer.Render("manifests", "", func(profile string) (interface{}, error) {
		return WebhookVariables{
			Namespace: mgh.Namespace,
		}, nil
	})
	if err != nil {
		return ctrl.Result{}, fmt.Errorf("failed to render webhook objects: %v", err)
	}
	if err = utils.ManipulateGlobalHubObjects(webhookObjects, mgh, hohDeployer, mapper, r.GetScheme()); err != nil {
		return ctrl.Result{}, fmt.Errorf("failed to create/update webhook objects: %v", err)
	}
	return ctrl.Result{}, nil
}

type WebhookVariables struct {
	Namespace string
}

// SetupWithManager sets up the controller with the Manager.
func (r *WebhookReconciler) SetupWithManager(mgr ctrl.Manager) error {
	return ctrl.NewControllerManagedBy(mgr).Named("webhook-controller").
		For(&v1alpha4.MulticlusterGlobalHub{},
			builder.WithPredicates(mghPred)).
		Watches(&addonv1alpha1.AddOnDeploymentConfig{},
			&handler.EnqueueRequestForObject{}, builder.WithPredicates(config.GeneralPredicate)).
		Watches(&clusterv1beta2.ManagedClusterSetBinding{},
			&handler.EnqueueRequestForObject{}, builder.WithPredicates(config.GeneralPredicate)).
		Watches(&admissionregistrationv1.MutatingWebhookConfiguration{},
			&handler.EnqueueRequestForObject{}, builder.WithPredicates(webhookPred)).
		Watches(&clusterv1beta1.Placement{},
			&handler.EnqueueRequestForObject{}, builder.WithPredicates(config.GeneralPredicate)).
		Complete(r)
}

var mghPred = predicate.Funcs{
	CreateFunc: func(e event.CreateEvent) bool {
		return true
	},
	UpdateFunc: func(e event.UpdateEvent) bool {
		if e.ObjectOld.GetGeneration() != e.ObjectNew.GetGeneration() {
			return true
		}
		if e.ObjectNew.GetDeletionTimestamp() != nil {
			return true
		}
		return !reflect.DeepEqual(e.ObjectOld.GetAnnotations(), e.ObjectNew.GetAnnotations())
	},
	DeleteFunc: func(e event.DeleteEvent) bool {
		return true
	},
}

var webhookPred = predicate.Funcs{
	CreateFunc: func(e event.CreateEvent) bool {
		return false
	},
	UpdateFunc: func(e event.UpdateEvent) bool {
		if e.ObjectNew.GetLabels()[constants.GlobalHubOwnerLabelKey] ==
			constants.GHOperatorOwnerLabelVal {
			new := e.ObjectNew.(*admissionregistrationv1.MutatingWebhookConfiguration)
			old := e.ObjectOld.(*admissionregistrationv1.MutatingWebhookConfiguration)
			if len(new.Webhooks) != len(old.Webhooks) ||
				new.Webhooks[0].Name != old.Webhooks[0].Name ||
				!reflect.DeepEqual(new.Webhooks[0].AdmissionReviewVersions,
					old.Webhooks[0].AdmissionReviewVersions) ||
				!reflect.DeepEqual(new.Webhooks[0].Rules, old.Webhooks[0].Rules) ||
				!reflect.DeepEqual(new.Webhooks[0].ClientConfig.Service, old.Webhooks[0].ClientConfig.Service) {
				return true
			}
			return false
		}
		return false
	},
	DeleteFunc: func(e event.DeleteEvent) bool {
		return e.Object.GetLabels()[constants.GlobalHubOwnerLabelKey] ==
			constants.GHOperatorOwnerLabelVal
	},
}

func (r *WebhookReconciler) pruneWebhookResources(ctx context.Context, namespaces string) error {
	listOpts := []client.ListOption{
		client.MatchingLabels(map[string]string{
			constants.GlobalHubOwnerLabelKey: constants.GHOperatorOwnerLabelVal,
		}),
	}
	webhookList := &admissionregistrationv1.MutatingWebhookConfigurationList{}
	if err := r.c.List(ctx, webhookList, listOpts...); err != nil {
		return err
	}

	for idx := range webhookList.Items {
		if err := r.c.Delete(ctx, &webhookList.Items[idx]); err != nil && !errors.IsNotFound(err) {
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
	if err := r.c.List(ctx, webhookServiceList, webhookServiceListOpts...); err != nil {
		return err
	}
	for idx := range webhookServiceList.Items {
		if err := r.c.Delete(ctx, &webhookServiceList.Items[idx]); err != nil && !errors.IsNotFound(err) {
			return err
		}
	}
	return nil
}
