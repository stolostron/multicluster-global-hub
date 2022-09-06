/*
Copyright 2022.

Licensed under the Apache License, Version 2.0 (the "License");
you may not use this file except in compliance with the License.
You may obtain a copy of the License at

    http://www.apache.org/licenses/LICENSE-2.0

Unless required by applicable law or agreed to in writing, software
distributed under the License is distributed on an "AS IS" BASIS,
WITHOUT WARRANTIES OR CONDITIONS OF ANY KIND, either express or implied.
See the License for the specific language governing permissions and
limitations under the License.
*/

package hubofhubs

import (
	"context"
	"embed"
	"fmt"
	"strconv"

	"github.com/go-logr/logr"
	appsv1 "k8s.io/api/apps/v1"
	corev1 "k8s.io/api/core/v1"
	networkingv1 "k8s.io/api/networking/v1"
	rbacv1 "k8s.io/api/rbac/v1"
	"k8s.io/apimachinery/pkg/api/equality"
	"k8s.io/apimachinery/pkg/api/errors"
	"k8s.io/apimachinery/pkg/api/meta"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/apis/meta/v1/unstructured"
	"k8s.io/apimachinery/pkg/runtime"
	"k8s.io/apimachinery/pkg/types"
	"k8s.io/client-go/discovery"
	"k8s.io/client-go/discovery/cached/memory"
	"k8s.io/client-go/dynamic"
	"k8s.io/client-go/kubernetes"
	"k8s.io/client-go/restmapper"
	ctrl "sigs.k8s.io/controller-runtime"
	"sigs.k8s.io/controller-runtime/pkg/builder"
	"sigs.k8s.io/controller-runtime/pkg/client"
	"sigs.k8s.io/controller-runtime/pkg/controller/controllerutil"
	"sigs.k8s.io/controller-runtime/pkg/event"
	"sigs.k8s.io/controller-runtime/pkg/handler"
	ctrllog "sigs.k8s.io/controller-runtime/pkg/log"
	"sigs.k8s.io/controller-runtime/pkg/manager"
	"sigs.k8s.io/controller-runtime/pkg/predicate"
	"sigs.k8s.io/controller-runtime/pkg/reconcile"
	"sigs.k8s.io/controller-runtime/pkg/source"

	// pmcontroller "github.com/stolostron/multicluster-global-hub/operator/pkg/controllers/packagemanifest"
	operatorv1alpha2 "github.com/stolostron/multicluster-global-hub/operator/apis/v1alpha2"
	"github.com/stolostron/multicluster-global-hub/operator/pkg/condition"
	"github.com/stolostron/multicluster-global-hub/operator/pkg/config"
	"github.com/stolostron/multicluster-global-hub/operator/pkg/constants"
	leafhubscontroller "github.com/stolostron/multicluster-global-hub/operator/pkg/controllers/leafhub"
	"github.com/stolostron/multicluster-global-hub/operator/pkg/deployer"
	"github.com/stolostron/multicluster-global-hub/operator/pkg/renderer"
	"github.com/stolostron/multicluster-global-hub/operator/pkg/utils"
	commonconstants "github.com/stolostron/multicluster-global-hub/pkg/constants"
)

//go:embed manifests
var fs embed.FS

var isLeafHubControllerRunnning = false

// var isPackageManifestControllerRunnning = false

// MulticlusterGlobalHubReconciler reconciles a MulticlusterGlobalHub object
type MulticlusterGlobalHubReconciler struct {
	manager.Manager
	client.Client
	KubeClient kubernetes.Interface
	Scheme     *runtime.Scheme
}

//+kubebuilder:rbac:groups=operator.open-cluster-management.io,resources=multiclusterglobalhubs,verbs=get;list;watch;create;update;patch;delete
//+kubebuilder:rbac:groups=operator.open-cluster-management.io,resources=multiclusterglobalhubs/status,verbs=get;update;patch
//+kubebuilder:rbac:groups=operator.open-cluster-management.io,resources=multiclusterglobalhubs/finalizers,verbs=update
//+kubebuilder:rbac:groups=cluster.open-cluster-management.io,resources=managedclustersets/join,verbs=create;delete
//+kubebuilder:rbac:groups=cluster.open-cluster-management.io,resources=managedclustersets/bind,verbs=create;delete
//+kubebuilder:rbac:groups=apps.open-cluster-management.io,resources=subscriptions,verbs=get;list;update;patch
//+kubebuilder:rbac:groups=apps.open-cluster-management.io,resources=placementrules,verbs=get;list;update;patch
//+kubebuilder:rbac:groups=apps.open-cluster-management.io,resources=channels,verbs=get;list;update;patch
//+kubebuilder:rbac:groups=policy.open-cluster-management.io,resources=policies,verbs=get;list;patch;update
//+kubebuilder:rbac:groups=policy.open-cluster-management.io,resources=placementbindings,verbs=get;list;patch;update
//+kubebuilder:rbac:groups=app.k8s.io,resources=applications,verbs=get;list;patch;update
//+kubebuilder:rbac:groups=cluster.open-cluster-management.io,resources=placements,verbs=get;list;patch;update
//+kubebuilder:rbac:groups=cluster.open-cluster-management.io,resources=managedclustersets,verbs=get;list;patch;update

//+kubebuilder:rbac:groups="",resources=namespaces,verbs=get;list;watch;create;update;delete
//+kubebuilder:rbac:groups="",resources=services,verbs=get;list;watch;create;update;delete
//+kubebuilder:rbac:groups="",resources=serviceaccounts,verbs=get;list;watch;create;update;delete
//+kubebuilder:rbac:groups="",resources=secrets,verbs=get;list;watch;create;update;delete
//+kubebuilder:rbac:groups="",resources=configmaps,verbs=get;list;watch;create;update;delete
//+kubebuilder:rbac:groups="apps",resources=deployments,verbs=get;list;watch;create;update;delete
//+kubebuilder:rbac:groups="networking.k8s.io",resources=ingresses,verbs=get;list;watch;create;update;delete
//+kubebuilder:rbac:groups="networking.k8s.io",resources=networkpolicies,verbs=get;list;watch;create;update;delete
//+kubebuilder:rbac:groups="rbac.authorization.k8s.io",resources=roles,verbs=get;list;watch;create;update;delete
//+kubebuilder:rbac:groups="rbac.authorization.k8s.io",resources=rolebindings,verbs=get;list;watch;create;update;delete
//+kubebuilder:rbac:groups="rbac.authorization.k8s.io",resources=clusterroles,verbs=get;list;watch;create;update;delete
//+kubebuilder:rbac:groups="rbac.authorization.k8s.io",resources=clusterrolebindings,verbs=get;list;watch;create;update;delete

// Reconcile is part of the main kubernetes reconciliation loop which aims to
// move the current state of the cluster closer to the desired state.
// TODO(user): Modify the Reconcile function to compare the state specified by
// the MulticlusterGlobalHub object against the actual cluster state, and then
// perform operations to make the cluster state reflect the state specified by
// the user.
//
// For more details, check Reconcile and its Result here:
// - https://pkg.go.dev/sigs.k8s.io/controller-runtime@v0.11.0/pkg/reconcile
func (r *MulticlusterGlobalHubReconciler) Reconcile(ctx context.Context, req ctrl.Request) (ctrl.Result, error) {
	log := ctrllog.FromContext(ctx)
	log.Info("Reconciling", "namespacedname", req.NamespacedName)

	// Fetch the multiclusterglobalhub instance
	mgh := &operatorv1alpha2.MulticlusterGlobalHub{}
	if err := r.Get(ctx, req.NamespacedName, mgh); err != nil {
		if errors.IsNotFound(err) {
			// Request object not found, could have been deleted after reconcile request.
			// Owned objects are automatically garbage collected. For additional cleanup logic use finalizers.
			// Return and don't requeue
			log.Info("MulticlusterGlobalHub resource not found. Ignoring since object must be deleted")
			return ctrl.Result{}, nil
		}
		// Error reading the object - requeue the request.
		log.Error(err, "Failed to get MulticlusterGlobalHub")
		return ctrl.Result{}, err
	}

	// create new HoHRenderer and HoHDeployer
	hohRenderer, hohDeployer := renderer.NewHoHRenderer(fs), deployer.NewHoHDeployer(r.Client)

	// create discovery client
	dc, err := discovery.NewDiscoveryClientForConfig(r.Manager.GetConfig())
	if err != nil {
		return ctrl.Result{}, err
	}

	// create restmapper for deployer to find GVR
	mapper := restmapper.NewDeferredDiscoveryRESTMapper(memory.NewMemCacheClient(dc))

	// handle gc
	isTerminating, err := r.recocileFinalizer(ctx, mgh, hohRenderer, hohDeployer, mapper, log)
	if err != nil {
		return ctrl.Result{}, err
	}

	if isTerminating {
		log.Info("multiclusterglobalhub is terminating, skip the reconcile")
		return ctrl.Result{}, nil
	}

	// check for image overrides configmap
	var imageOverridesConfigmap *corev1.ConfigMap
	if imageOverridesConfigmapName := config.GetImageOverridesConfigmap(mgh); imageOverridesConfigmapName != "" {
		var err error
		imageOverridesConfigmap, err = r.KubeClient.CoreV1().ConfigMaps(mgh.GetNamespace()).Get(
			ctx, imageOverridesConfigmapName, metav1.GetOptions{})
		if err != nil {
			log.Error(err, "failed to get image overrides configmap",
				"namespace", mgh.GetNamespace(),
				"name", imageOverridesConfigmapName)
			return ctrl.Result{}, err
		}
	}

	// set imgae overrides
	if err := config.SetImageOverrides(mgh, imageOverridesConfigmap); err != nil {
		return ctrl.Result{}, err
	}

	if config.IsPaused(mgh) {
		log.Info("multiclusterglobalhub reconciliation is paused, nothing more to do")
		return ctrl.Result{}, nil
	}

	if !config.SkipDBInit(mgh) {
		// init DB and transport here
		if err = r.reconcileDatabase(ctx, mgh, types.NamespacedName{
			Name:      mgh.Spec.DataLayer.LargeScale.Postgres.Name,
			Namespace: config.GetDefaultNamespace(),
		}); err != nil {
			return ctrl.Result{}, err
		}
	}

	// reconcile open-cluster-management-global-hub-system namespace and multicluster-global-hub configuration
	if err = r.reconcileHoHResources(ctx, mgh); err != nil {
		return ctrl.Result{}, err
	}

	hohRBACObjects, err := hohRenderer.Render("manifests/rbac", func(component string) (interface{}, error) {
		return struct {
			Image     string
			Namespace string
		}{
			Image:     config.GetImage("multicluster_global_hub_rbac"),
			Namespace: config.GetDefaultNamespace(),
		}, nil
	})
	if err != nil {
		return ctrl.Result{}, err
	}

	if err = r.manipulateObj(ctx, hohDeployer, mapper, hohRBACObjects, mgh,
		condition.SetConditionRBACDeployed, log); err != nil {
		return ctrl.Result{}, err
	}

	// retrieve bootstrapserver and CA of kafka from secret
	kafkaBootstrapServer, kafkaCA, err := utils.GetKafkaConfig(ctx, r.KubeClient, mgh)
	if err != nil {
		if conditionError := condition.SetConditionTransportInit(ctx, r.Client, mgh,
			condition.CONDITION_STATUS_FALSE); conditionError != nil {
			return ctrl.Result{}, fmt.Errorf("failed to set condition(%s): %w",
				condition.CONDITION_STATUS_FALSE, conditionError)
		}
		return ctrl.Result{}, err
	}

	if conditionError := condition.SetConditionTransportInit(ctx, r.Client, mgh,
		condition.CONDITION_STATUS_TRUE); conditionError != nil {
		return ctrl.Result{}, fmt.Errorf("failed to set condition(%s): %w",
			condition.CONDITION_STATUS_TRUE, conditionError)
	}

	managerObjects, err := hohRenderer.Render("manifests/manager", func(component string) (interface{}, error) {
		return struct {
			Image                string
			DBSecret             string
			KafkaCA              string
			KafkaBootstrapServer string
			Namespace            string
		}{
			Image:                config.GetImage("multicluster_global_hub_manager"),
			DBSecret:             mgh.Spec.DataLayer.LargeScale.Postgres.Name,
			KafkaCA:              kafkaCA,
			KafkaBootstrapServer: kafkaBootstrapServer,
			Namespace:            config.GetDefaultNamespace(),
		}, nil
	})
	if err != nil {
		return ctrl.Result{}, err
	}

	if err = r.manipulateObj(ctx, hohDeployer, mapper, managerObjects, mgh,
		condition.SetConditionManagerDeployed, log); err != nil {
		return ctrl.Result{}, err
	}

	// try to start leafhub controller if it is not running
	if !isLeafHubControllerRunnning {
		dynamicClient, err := dynamic.NewForConfig(r.Manager.GetConfig())
		if err != nil {
			log.Error(err, "failed to create dynamic client")
			return ctrl.Result{}, err
		}
		kubeClient, err := kubernetes.NewForConfig(r.Manager.GetConfig())
		if err != nil {
			log.Error(err, "failed to create kube client")
			return ctrl.Result{}, err
		}
		if err := (&leafhubscontroller.LeafHubReconciler{
			DynamicClient: dynamicClient,
			KubeClient:    kubeClient,
			Client:        r.Client,
			Scheme:        r.Scheme,
		}).SetupWithManager(r.Manager); err != nil {
			log.Error(err, "unable to create controller", "controller", "LeafHub")
			return ctrl.Result{}, err
		}
		log.Info("leafhub controller is started")
		isLeafHubControllerRunnning = true
	}

	// // try to start packagemanifest controller if it is not running
	// if !isPackageManifestControllerRunnning {
	// 	if err := (&pmcontroller.PackageManifestReconciler{
	// 		Client: r.Client,
	// 		Scheme: r.Scheme,
	// 	}).SetupWithManager(r.Manager); err != nil {
	// 		log.Error(err, "unable to create controller", "controller", "PackageManifest")
	// 		return ctrl.Result{}, err
	// 	}
	// 	log.Info("packagemanifest controller is started")
	// 	isPackageManifestControllerRunnning = true
	// }

	return ctrl.Result{}, nil
}

func (r *MulticlusterGlobalHubReconciler) manipulateObj(ctx context.Context, hohDeployer deployer.Deployer,
	mapper *restmapper.DeferredDiscoveryRESTMapper, objs []*unstructured.Unstructured,
	mgh *operatorv1alpha2.MulticlusterGlobalHub, setConditionFunc condition.SetConditionFunc,
	log logr.Logger,
) error {
	// manipulate the object
	for _, obj := range objs {
		mapping, err := mapper.RESTMapping(obj.GroupVersionKind().GroupKind(), obj.GroupVersionKind().Version)
		if err != nil {
			log.Error(err, "failed to find mapping for resource", "kind", obj.GetKind(),
				"namespace", obj.GetNamespace(), "name", obj.GetName())
			return err
		}

		if mapping.Scope.Name() == meta.RESTScopeNameNamespace {
			// for namespaced resource, set ownerreference of controller
			if err := controllerutil.SetControllerReference(mgh, obj, r.Scheme); err != nil {
				log.Error(err, "failed to set controller reference", "kind", obj.GetKind(),
					"namespace", obj.GetNamespace(), "name", obj.GetName())
				return err
			}
		}

		// set owner labels
		labels := obj.GetLabels()
		if labels == nil {
			labels = make(map[string]string)
		}
		labels[commonconstants.GlobalHubOwnerLabelKey] =
			commonconstants.HoHOperatorOwnerLabelVal
		obj.SetLabels(labels)

		log.Info("Creating or updating object", "object", obj)
		if err := hohDeployer.Deploy(obj); err != nil {
			if setConditionFunc != nil {
				conditionError := setConditionFunc(ctx, r.Client, mgh, condition.CONDITION_STATUS_FALSE)
				if conditionError != nil {
					return fmt.Errorf("failed to set condition(%s): %w",
						condition.CONDITION_STATUS_FALSE, conditionError)
				}
			}
			return err
		}
	}

	if setConditionFunc != nil {
		if conditionError := setConditionFunc(ctx, r.Client, mgh,
			condition.CONDITION_STATUS_TRUE); conditionError != nil {
			return fmt.Errorf("failed to set condition(%s): %w",
				condition.CONDITION_STATUS_TRUE, conditionError)
		}
	}

	return nil
}

// reconcileHoHResources tries to create hoh resources if they don't exist
func (r *MulticlusterGlobalHubReconciler) reconcileHoHResources(ctx context.Context,
	mgh *operatorv1alpha2.MulticlusterGlobalHub,
) error {
	if err := r.Client.Get(ctx,
		types.NamespacedName{
			Name: constants.HOHSystemNamespace,
		}, &corev1.Namespace{}); err != nil {
		if errors.IsNotFound(err) {
			if err := r.Client.Create(ctx, &corev1.Namespace{
				ObjectMeta: metav1.ObjectMeta{
					Name: constants.HOHSystemNamespace,
					Labels: map[string]string{
						commonconstants.GlobalHubOwnerLabelKey: commonconstants.HoHOperatorOwnerLabelVal,
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
	hohConfigMap := &corev1.ConfigMap{
		ObjectMeta: metav1.ObjectMeta{
			Namespace: constants.HOHSystemNamespace,
			Name:      constants.HOHConfigName,
			Labels: map[string]string{
				commonconstants.GlobalHubOwnerLabelKey: commonconstants.HoHOperatorOwnerLabelVal,
			},
		},
		Data: map[string]string{
			"aggregationLevel":    string(mgh.Spec.AggregationLevel),
			"enableLocalPolicies": strconv.FormatBool(mgh.Spec.EnableLocalPolicies),
		},
	}

	existingHoHConfigMap := &corev1.ConfigMap{}
	if err := r.Client.Get(ctx,
		types.NamespacedName{
			Namespace: constants.HOHSystemNamespace,
			Name:      constants.HOHConfigName,
		}, existingHoHConfigMap); err != nil {
		if errors.IsNotFound(err) {
			if err := r.Client.Create(ctx, hohConfigMap); err != nil {
				return err
			}
			return nil
		} else {
			return err
		}
	}

	if !equality.Semantic.DeepDerivative(hohConfigMap.Data, existingHoHConfigMap.Data) ||
		!equality.Semantic.DeepDerivative(hohConfigMap.GetLabels(), existingHoHConfigMap.GetLabels()) {
		hohConfigMap.ObjectMeta.ResourceVersion = existingHoHConfigMap.ObjectMeta.ResourceVersion
		if err := r.Client.Update(ctx, hohConfigMap); err != nil {
			return err
		}
		return nil
	}

	return nil
}

// SetupWithManager sets up the controller with the Manager.
func (r *MulticlusterGlobalHubReconciler) SetupWithManager(mgr ctrl.Manager) error {
	mghPred := predicate.Funcs{
		CreateFunc: func(e event.CreateEvent) bool {
			// set request name to be used in leafhub controller
			config.SetHoHMGHNamespacedName(types.NamespacedName{
				Namespace: e.Object.GetNamespace(), Name: e.Object.GetName(),
			})
			return true
		},
		UpdateFunc: func(e event.UpdateEvent) bool {
			return e.ObjectOld.GetGeneration() != e.ObjectNew.GetGeneration()
		},
		DeleteFunc: func(e event.DeleteEvent) bool {
			return !e.DeleteStateUnknown
		},
	}

	ownPred := predicate.Funcs{
		CreateFunc: func(e event.CreateEvent) bool {
			return false
		},
		UpdateFunc: func(e event.UpdateEvent) bool {
			return e.ObjectOld.GetGeneration() != e.ObjectNew.GetGeneration() // only requeue when spec change
		},
		DeleteFunc: func(e event.DeleteEvent) bool {
			return true
		},
	}

	resPred := predicate.Funcs{
		CreateFunc: func(e event.CreateEvent) bool {
			return false
		},
		UpdateFunc: func(e event.UpdateEvent) bool {
			if e.ObjectNew.GetLabels()[commonconstants.GlobalHubOwnerLabelKey] ==
				commonconstants.HoHOperatorOwnerLabelVal &&
				e.ObjectNew.GetGeneration() != e.ObjectOld.GetGeneration() {
				return true
			}
			return false
		},
		DeleteFunc: func(e event.DeleteEvent) bool {
			return e.Object.GetLabels()[commonconstants.GlobalHubOwnerLabelKey] ==
				commonconstants.HoHOperatorOwnerLabelVal
		},
	}

	opaDataPred := predicate.Funcs{
		CreateFunc: func(e event.CreateEvent) bool {
			return false
		},
		UpdateFunc: func(e event.UpdateEvent) bool {
			if e.ObjectNew.GetName() == "opa-data" {
				return false
			}
			return e.ObjectOld.GetGeneration() != e.ObjectNew.GetGeneration() // only requeue when spec change
		},
		DeleteFunc: func(e event.DeleteEvent) bool {
			return true
		},
	}

	return ctrl.NewControllerManagedBy(mgr).
		For(&operatorv1alpha2.MulticlusterGlobalHub{}, builder.WithPredicates(mghPred)).
		Owns(&appsv1.Deployment{}, builder.WithPredicates(ownPred)).
		Owns(&corev1.Service{}, builder.WithPredicates(ownPred)).
		Owns(&corev1.ServiceAccount{}, builder.WithPredicates(ownPred)).
		// watch owned secret, opa-data secret is an exception.
		Owns(&corev1.Secret{}, builder.WithPredicates(opaDataPred)).
		Owns(&rbacv1.Role{}, builder.WithPredicates(ownPred)).
		Owns(&rbacv1.RoleBinding{}, builder.WithPredicates(ownPred)).
		Owns(&networkingv1.Ingress{}, builder.WithPredicates(ownPred)).
		Owns(&networkingv1.NetworkPolicy{}, builder.WithPredicates(ownPred)).
		// secondary watch for configmap
		Watches(&source.Kind{Type: &corev1.ConfigMap{}},
			handler.EnqueueRequestsFromMapFunc(func(obj client.Object) []reconcile.Request {
				return []reconcile.Request{
					// trigger MGH instance reconcile
					{NamespacedName: config.GetHoHMGHNamespacedName()},
				}
			}), builder.WithPredicates(resPred)).
		// secondary watch for namespace
		Watches(&source.Kind{Type: &corev1.Namespace{}},
			handler.EnqueueRequestsFromMapFunc(func(obj client.Object) []reconcile.Request {
				return []reconcile.Request{
					// trigger MGH instance reconcile
					{NamespacedName: config.GetHoHMGHNamespacedName()},
				}
			}), builder.WithPredicates(resPred)).
		// secondary watch for clusterrole
		Watches(&source.Kind{Type: &rbacv1.ClusterRole{}},
			handler.EnqueueRequestsFromMapFunc(func(obj client.Object) []reconcile.Request {
				return []reconcile.Request{
					// trigger MGH instance reconcile
					{NamespacedName: config.GetHoHMGHNamespacedName()},
				}
			}), builder.WithPredicates(resPred)).
		// secondary watch for clusterrolebinding
		Watches(&source.Kind{Type: &rbacv1.ClusterRoleBinding{}},
			handler.EnqueueRequestsFromMapFunc(func(obj client.Object) []reconcile.Request {
				return []reconcile.Request{
					// trigger MGH instance reconcile
					{NamespacedName: config.GetHoHMGHNamespacedName()},
				}
			}), builder.WithPredicates(resPred)).
		Complete(r)
}
