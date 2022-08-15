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
	"encoding/base64"
	"fmt"
	"strconv"
	"time"

	"github.com/go-logr/logr"
	appsv1 "k8s.io/api/apps/v1"
	batchv1 "k8s.io/api/batch/v1"
	corev1 "k8s.io/api/core/v1"
	networkingv1 "k8s.io/api/networking/v1"
	rbacv1 "k8s.io/api/rbac/v1"
	"k8s.io/apimachinery/pkg/api/equality"
	"k8s.io/apimachinery/pkg/api/errors"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/apis/meta/v1/unstructured"
	"k8s.io/apimachinery/pkg/runtime"
	"k8s.io/apimachinery/pkg/types"
	"k8s.io/apimachinery/pkg/util/wait"
	"k8s.io/client-go/dynamic"
	clusterv1beta1 "open-cluster-management.io/api/cluster/v1beta1"
	ctrl "sigs.k8s.io/controller-runtime"
	"sigs.k8s.io/controller-runtime/pkg/builder"
	"sigs.k8s.io/controller-runtime/pkg/client"
	"sigs.k8s.io/controller-runtime/pkg/controller/controllerutil"
	"sigs.k8s.io/controller-runtime/pkg/event"
	ctrllog "sigs.k8s.io/controller-runtime/pkg/log"
	"sigs.k8s.io/controller-runtime/pkg/manager"
	"sigs.k8s.io/controller-runtime/pkg/predicate"

	operatorv1alpha1 "github.com/stolostron/multicluster-globalhub/operator/apis/operator/v1alpha1"
	// pmcontroller "github.com/stolostron/multicluster-globalhub/operator/pkg/controllers/packagemanifest"
	"github.com/stolostron/multicluster-globalhub/operator/pkg/condition"
	"github.com/stolostron/multicluster-globalhub/operator/pkg/config"
	"github.com/stolostron/multicluster-globalhub/operator/pkg/constants"
	leafhubscontroller "github.com/stolostron/multicluster-globalhub/operator/pkg/controllers/leafhub"
	"github.com/stolostron/multicluster-globalhub/operator/pkg/deployer"
	"github.com/stolostron/multicluster-globalhub/operator/pkg/renderer"
	"github.com/stolostron/multicluster-globalhub/operator/pkg/utils"
)

//go:embed manifests
var fs embed.FS

var isLeafHubControllerRunnning = false

// var isPackageManifestControllerRunnning = false

// MultiClusterGlobalHubReconciler reconciles a MultiClusterGlobalHub object
type MultiClusterGlobalHubReconciler struct {
	manager.Manager
	client.Client
	Scheme *runtime.Scheme
}

//+kubebuilder:rbac:groups=operator.open-cluster-management.io,resources=multiclusterglobalhubs,verbs=get;list;watch;create;update;patch;delete
//+kubebuilder:rbac:groups=operator.open-cluster-management.io,resources=multiclusterglobalhubs/status,verbs=get;update;patch
//+kubebuilder:rbac:groups=operator.open-cluster-management.io,resources=multiclusterglobalhubs/finalizers,verbs=update
//+kubebuilder:rbac:groups=cluster.open-cluster-management.io,resources=managedclustersetbindings,verbs=get;list;watch;create;update;patch;delete
//+kubebuilder:rbac:groups=cluster.open-cluster-management.io,resources=managedclustersets/join,verbs=create;delete
//+kubebuilder:rbac:groups=cluster.open-cluster-management.io,resources=managedclustersets/bind,verbs=create;delete

//+kubebuilder:rbac:groups="",resources=namespaces,verbs=get;list;watch;create;update;delete
//+kubebuilder:rbac:groups="",resources=services,verbs=get;list;watch;create;update;delete
//+kubebuilder:rbac:groups="",resources=serviceaccounts,verbs=get;list;watch;create;update;delete
//+kubebuilder:rbac:groups="",resources=secrets,verbs=get;list;watch;create;update;delete
//+kubebuilder:rbac:groups="",resources=configmaps,verbs=get;list;watch;create;update;delete
//+kubebuilder:rbac:groups="apps",resources=deployments,verbs=get;list;watch;create;update;delete
//+kubebuilder:rbac:groups="batch",resources=jobs,verbs=get;list;watch;create;delete;list;watch
//+kubebuilder:rbac:groups="networking.k8s.io",resources=ingresses,verbs=get;list;watch;create;update;delete
//+kubebuilder:rbac:groups="networking.k8s.io",resources=networkpolicies,verbs=get;list;watch;create;update;delete
//+kubebuilder:rbac:groups="rbac.authorization.k8s.io",resources=roles,verbs=get;list;watch;create;update;delete
//+kubebuilder:rbac:groups="rbac.authorization.k8s.io",resources=rolebindings,verbs=get;list;watch;create;update;delete
//+kubebuilder:rbac:groups="rbac.authorization.k8s.io",resources=clusterroles,verbs=get;list;watch;create;update;delete
//+kubebuilder:rbac:groups="rbac.authorization.k8s.io",resources=clusterrolebindings,verbs=get;list;watch;create;update;delete

// Reconcile is part of the main kubernetes reconciliation loop which aims to
// move the current state of the cluster closer to the desired state.
// TODO(user): Modify the Reconcile function to compare the state specified by
// the MultiClusterGlobalHub object against the actual cluster state, and then
// perform operations to make the cluster state reflect the state specified by
// the user.
//
// For more details, check Reconcile and its Result here:
// - https://pkg.go.dev/sigs.k8s.io/controller-runtime@v0.11.0/pkg/reconcile
func (r *MultiClusterGlobalHubReconciler) Reconcile(ctx context.Context, req ctrl.Request) (ctrl.Result, error) {
	log := ctrllog.FromContext(ctx)
	log.Info("Reconciling", "namespacedname", req.NamespacedName)

	// Fetch the hub-of-hubs multiclusterglobalhub instance
	mgh := &operatorv1alpha1.MultiClusterGlobalHub{}
	err := r.Get(ctx, req.NamespacedName, mgh)
	if err != nil {
		if errors.IsNotFound(err) {
			// Request object not found, could have been deleted after reconcile request.
			// Owned objects are automatically garbage collected. For additional cleanup logic use finalizers.
			// Return and don't requeue
			log.Info("MultiClusterGlobalHub resource not found. Ignoring since object must be deleted")
			return ctrl.Result{}, nil
		}
		// Error reading the object - requeue the request.
		log.Error(err, "Failed to get MultiClusterGlobalHub")
		return ctrl.Result{}, err
	}

	if err := condition.SetConditionResourceFound(ctx, r.Client, mgh); err != nil {
		log.Error(err, "Failed to set condition resource found")
		return ctrl.Result{}, err
	}

	// create new HoHRenderer and HoHDeployer
	hohRenderer := renderer.NewHoHRenderer(fs)

	// handle gc
	isTerminating, err := r.initFinalization(ctx, mgh, hohRenderer, log)
	if err != nil {
		return ctrl.Result{}, err
	}

	if isTerminating {
		log.Info("multiclusterglobalhub is terminating, skip the reconcile")
		return ctrl.Result{}, err
	}

	// init DB and transport here
	err = r.reconcileDatabase(ctx, mgh, types.NamespacedName{
		Name:      mgh.Spec.PostgreSQL.Name,
		Namespace: constants.HOHDefaultNamespace,
	})
	if err != nil {
		return ctrl.Result{}, err
	}

	// reconcile hoh-system namespace and hub-of-hubs configuration
	err = r.reconcileHoHResources(ctx, mgh)
	if err != nil {
		return ctrl.Result{}, err
	}

	annotations := mgh.GetAnnotations()
	hohRBACObjects, err := hohRenderer.Render("manifests/rbac", func(component string) (interface{}, error) {
		hohRBACConfig := struct {
			Image string
		}{
			Image: config.GetImage(annotations, "hub_of_hubs_rbac"),
		}

		return hohRBACConfig, nil
	})
	if err != nil {
		return ctrl.Result{}, err
	}

	err = r.manipulateObj(ctx, hohRBACObjects, mgh, condition.SetConditionRBACDeployed, log)
	if err != nil {
		return ctrl.Result{}, err
	}

	// retrieve bootstrapserver and CA of kafka from secret
	kafkaBootstrapServer, kafkaCA, err := getKafkaConfig(ctx, r.Client, log, mgh)
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
		managerConfig := struct {
			Image                string
			DBSecret             string
			KafkaCA              string
			KafkaBootstrapServer string
		}{
			Image:                config.GetImage(annotations, "hub_of_hubs_manager"),
			DBSecret:             mgh.Spec.PostgreSQL.Name,
			KafkaCA:              kafkaCA,
			KafkaBootstrapServer: kafkaBootstrapServer,
		}

		return managerConfig, nil
	})
	if err != nil {
		return ctrl.Result{}, err
	}

	err = r.manipulateObj(ctx, managerObjects, mgh, condition.SetConditionManagerDeployed, log)
	if err != nil {
		return ctrl.Result{}, err
	}

	// render the default placement
	placementObjects, err := hohRenderer.Render("manifests/placement", func(component string) (interface{}, error) {
		return struct{}{}, nil
	})
	if err != nil {
		return ctrl.Result{}, err
	}

	err = r.manipulateObj(ctx, placementObjects, mgh, nil, log)
	if err != nil {
		return ctrl.Result{}, err
	}

	if mgh.GetAnnotations()[constants.GlobalHubSkipConsoleInstallAnnotationKey] != "true" {
		// render the console setup job
		consoleSetupObjects, err := hohRenderer.Render("manifests/console-setup", func(
			component string,
		) (interface{}, error) {
			consoleJobConfig := struct {
				Image string
			}{
				Image: config.GetImage(annotations, "hub_of_hubs_console_job"),
			}

			return consoleJobConfig, nil
		})
		if err != nil {
			return ctrl.Result{}, err
		}

		err = r.manipulateObj(ctx, consoleSetupObjects, mgh,
			condition.SetConditionConsoleDeployed, log)
		if err != nil {
			return ctrl.Result{}, err
		}

		log.Info("wait at most 300s for the hub-of-hubs console is set up")
		consoleSetupJob := &batchv1.Job{}
		if errPoll := wait.Poll(10*time.Second, 300*time.Second, func() (bool, error) {
			if err := r.Client.Get(ctx,
				types.NamespacedName{
					Name:      "hoh-of-hubs-console-setup",
					Namespace: constants.HOHDefaultNamespace,
				}, consoleSetupJob); err != nil {
				return false, err
			}
			if consoleSetupJob.Status.Succeeded > 0 {
				return true, nil
			}
			return false, nil
		}); errPoll != nil {
			log.Error(errPoll, "hub-of-hubs console setup job failed",
				"namespace", constants.HOHDefaultNamespace,
				"name", "hoh-of-hubs-console-setup")
			if conditionError := condition.SetConditionConsoleDeployed(ctx, r.Client, mgh,
				condition.CONDITION_STATUS_FALSE); conditionError != nil {
				return ctrl.Result{}, fmt.Errorf("failed to set condition(%s): %w",
					condition.CONDITION_STATUS_FALSE, conditionError)
			}
			return ctrl.Result{}, errPoll
		}
		log.Info("hub-of-hubs console is set up")
		if conditionError := condition.SetConditionConsoleDeployed(ctx, r.Client, mgh,
			condition.CONDITION_STATUS_TRUE); conditionError != nil {
			return ctrl.Result{}, fmt.Errorf("failed to set condition(%s): %w",
				condition.CONDITION_STATUS_TRUE, conditionError)
		}
	}

	// try to start leafhub controller if it is not running
	if !isLeafHubControllerRunnning {
		dynamicClient, err := dynamic.NewForConfig(r.Manager.GetConfig())
		if err != nil {
			log.Error(err, "failed to create dynamic client")
			return ctrl.Result{}, err
		}
		if err := (&leafhubscontroller.LeafHubReconciler{
			DynamicClient: dynamicClient,
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

func (r *MultiClusterGlobalHubReconciler) manipulateObj(ctx context.Context, objs []*unstructured.Unstructured,
	mgh *operatorv1alpha1.MultiClusterGlobalHub, setConditionFunc condition.SetConditionFunc, log logr.Logger,
) error {
	hohDeployer := deployer.NewHoHDeployer(r.Client)
	// manipulate the object
	for _, obj := range objs {
		// TODO: more solid way to check if the object is namespace scoped resource
		if obj.GetNamespace() != "" {
			// set ownerreference of controller
			if err := controllerutil.SetControllerReference(mgh, obj, r.Scheme); err != nil {
				log.Error(err, "failed to set controller reference", "kind", obj.GetKind(),
					"namespace", obj.GetNamespace(), "name", obj.GetName())
			}
		}
		// set owner labels
		labels := obj.GetLabels()
		if labels == nil {
			labels = make(map[string]string)
		}
		labels[constants.HoHOperatorOwnerLabelKey] = constants.HoHOperatorOwnerLabelVal
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

func (r *MultiClusterGlobalHubReconciler) initFinalization(ctx context.Context, mgh *operatorv1alpha1.MultiClusterGlobalHub,
	hohRenderer renderer.Renderer, log logr.Logger,
) (bool, error) {
	if mgh.GetDeletionTimestamp() != nil &&
		utils.Contains(mgh.GetFinalizers(), constants.HoHOperatorFinalizer) {
		if mgh.GetAnnotations()[constants.GlobalHubSkipConsoleInstallAnnotationKey] != "true" {
			log.Info("cleanup hub-of-hubs console")
			// render the console cleanup job
			consoleCleanupObjects, err := hohRenderer.Render("manifests/console-cleanup",
				func(component string) (interface{}, error) {
					consoleJobConfig := struct {
						Image string
					}{
						Image: config.GetImage(mgh.GetAnnotations(), "hub_of_hubs_console_job"),
					}

					return consoleJobConfig, nil
				})
			if err != nil {
				return false, err
			}
			if err := r.manipulateObj(ctx, consoleCleanupObjects, mgh, nil, log); err != nil {
				return false, err
			}

			log.Info("wait at most 300s for the hub-of-hubs console is cleaned up")
			consoleCleanJob := &batchv1.Job{}
			if errPoll := wait.Poll(10*time.Second, 300*time.Second, func() (bool, error) {
				if err := r.Client.Get(ctx,
					types.NamespacedName{
						Name:      "hoh-of-hubs-console-cleanup",
						Namespace: constants.HOHDefaultNamespace,
					}, consoleCleanJob); err != nil {
					return false, err
				}
				if consoleCleanJob.Status.Succeeded > 0 {
					return true, nil
				}
				return false, nil
			}); errPoll != nil {
				log.Error(errPoll, "hub-of-hubs console cleanup job failed",
					"namespace", constants.HOHDefaultNamespace,
					"name", "hoh-of-hubs-console-cleanup")
				return false, errPoll
			}
			log.Info("hub-of-hubs console is cleaned up")
		}

		log.Info("to delete hoh resources")
		// clean up the cluster resources, eg. clusterrole, clusterrolebinding, etc
		if err := r.pruneGlobalResources(ctx); err != nil {
			log.Error(err, "failed to remove cluster scoped resources")
			return false, err
		}

		// clean up hoh-system namespace and hub-of-hubs configuration
		if err := r.pruneHoHResources(ctx); err != nil {
			log.Error(err, "failed to remove hub-of-hubs resources")
			return false, err
		}

		mgh.SetFinalizers(utils.Remove(mgh.GetFinalizers(), constants.HoHOperatorFinalizer))
		if err := r.Client.Update(context.TODO(), mgh); err != nil {
			log.Error(err, "failed to remove finalizer from multiclusterglobalhub resource")
			return false, err
		}
		log.Info("finalizer is removed from multiclusterglobalhub resource")

		return true, nil
	}
	if !utils.Contains(mgh.GetFinalizers(), constants.HoHOperatorFinalizer) {
		mgh.SetFinalizers(append(mgh.GetFinalizers(), constants.HoHOperatorFinalizer))
		err := r.Client.Update(context.TODO(), mgh)
		if err != nil {
			log.Error(err, "failed to add finalizer to multiclusterglobalhub resource")
			return false, err
		}
		log.Info("finalizer is added to multiclusterglobalhub resource")
	}

	return false, nil
}

// pruneGlobalResources deletes the cluster scoped resources created by the multicluster-globalhub-operator
// cluster scoped resources need to be deleted manually because they don't have ownerrefenence set
func (r *MultiClusterGlobalHubReconciler) pruneGlobalResources(ctx context.Context) error {
	listOpts := []client.ListOption{
		client.MatchingLabels(map[string]string{constants.HoHOperatorOwnerLabelKey: constants.HoHOperatorOwnerLabelVal}),
	}

	clusterRoleList := &rbacv1.ClusterRoleList{}
	err := r.Client.List(ctx, clusterRoleList, listOpts...)
	if err != nil {
		return err
	}
	for idx := range clusterRoleList.Items {
		if err := r.Client.Delete(ctx, &clusterRoleList.Items[idx]); err != nil &&
			!errors.IsNotFound(err) {
			return err
		}
	}

	clusterRoleBindingList := &rbacv1.ClusterRoleBindingList{}
	err = r.Client.List(ctx, clusterRoleBindingList, listOpts...)
	if err != nil {
		return err
	}
	for idx := range clusterRoleBindingList.Items {
		if err := r.Client.Delete(ctx, &clusterRoleBindingList.Items[idx]); err != nil &&
			!errors.IsNotFound(err) {
			return err
		}
	}

	return nil
}

// reconcileHoHResources tries to create hoh resources if they don't exist
func (r *MultiClusterGlobalHubReconciler) reconcileHoHResources(ctx context.Context, mgh *operatorv1alpha1.MultiClusterGlobalHub) error {
	if err := r.Client.Get(ctx,
		types.NamespacedName{
			Name: constants.HOHSystemNamespace,
		}, &corev1.Namespace{}); err != nil {
		if errors.IsNotFound(err) {
			if err := r.Client.Create(ctx, &corev1.Namespace{
				ObjectMeta: metav1.ObjectMeta{
					Name: constants.HOHSystemNamespace,
					Labels: map[string]string{
						constants.HoHOperatorOwnerLabelKey: constants.HoHOperatorOwnerLabelVal,
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
				constants.HoHOperatorOwnerLabelKey: constants.HoHOperatorOwnerLabelVal,
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

// pruneHoHResources tries to delete hoh resources
func (r *MultiClusterGlobalHubReconciler) pruneHoHResources(ctx context.Context) error {
	// hoh-system namespace and multicluster-globalhub-config configmap
	hohSystemNamespace := &corev1.Namespace{
		ObjectMeta: metav1.ObjectMeta{
			Name: constants.HOHSystemNamespace,
			Labels: map[string]string{
				constants.HoHOperatorOwnerLabelKey: constants.HoHOperatorOwnerLabelVal,
			},
		},
	}
	existingHoHConfigMap := &corev1.ConfigMap{}
	err := r.Client.Get(ctx,
		types.NamespacedName{
			Namespace: constants.HOHSystemNamespace,
			Name:      constants.HOHConfigName,
		}, existingHoHConfigMap)
	if err != nil {
		if !errors.IsNotFound(err) {
			return err
		}

		// cleanup hoh-system namespace
		if err := r.Client.Delete(ctx, hohSystemNamespace); err != nil && !errors.IsNotFound(err) {
			return err
		}

		return nil
	}

	// clean the finalizers added by multicluster-globalhub-manager
	existingHoHConfigMap.SetFinalizers([]string{})
	if err := r.Client.Update(ctx, existingHoHConfigMap); err != nil {
		return err
	}

	if err := r.Client.Delete(ctx, existingHoHConfigMap); err != nil && !errors.IsNotFound(err) {
		return err
	}

	// cleanup hoh-system namespace
	if err := r.Client.Delete(ctx, hohSystemNamespace); err != nil && !errors.IsNotFound(err) {
		return err
	}

	return nil
}

// SetupWithManager sets up the controller with the Manager.
func (r *MultiClusterGlobalHubReconciler) SetupWithManager(mgr ctrl.Manager) error {
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

	return ctrl.NewControllerManagedBy(mgr).
		For(&operatorv1alpha1.MultiClusterGlobalHub{}, builder.WithPredicates(mghPred)).
		Owns(&appsv1.Deployment{}, builder.WithPredicates(ownPred)).
		Owns(&batchv1.Job{}, builder.WithPredicates(ownPred)).
		Owns(&corev1.Service{}, builder.WithPredicates(ownPred)).
		Owns(&corev1.ServiceAccount{}, builder.WithPredicates(ownPred)).
		Owns(&corev1.ConfigMap{}, builder.WithPredicates(ownPred)).
		Owns(&corev1.Secret{}, builder.WithPredicates(ownPred)).
		Owns(&rbacv1.Role{}, builder.WithPredicates(ownPred)).
		Owns(&rbacv1.RoleBinding{}, builder.WithPredicates(ownPred)).
		Owns(&networkingv1.Ingress{}, builder.WithPredicates(ownPred)).
		Owns(&networkingv1.NetworkPolicy{}, builder.WithPredicates(ownPred)).
		Owns(&clusterv1beta1.ManagedClusterSetBinding{}, builder.WithPredicates(ownPred)).
		Owns(&clusterv1beta1.Placement{}, builder.WithPredicates(ownPred)).
		Complete(r)
}

// getKafkaConfig retrieves kafka server and CA from kafka secret
func getKafkaConfig(ctx context.Context, c client.Client, log logr.Logger, mgh *operatorv1alpha1.MultiClusterGlobalHub) (
	string, string, error,
) {
	// for local dev/test
	kafkaBootstrapServer, ok := mgh.GetAnnotations()[constants.HoHKafkaBootstrapServerKey]
	if ok && kafkaBootstrapServer != "" {
		log.Info("Kafka bootstrap server from annotation", "server", kafkaBootstrapServer, "certificate", "")
		return kafkaBootstrapServer, "", nil
	}

	kafkaSecret := &corev1.Secret{}
	if err := c.Get(ctx, types.NamespacedName{
		Namespace: constants.HOHDefaultNamespace,
		Name:      mgh.Spec.Kafka.Name,
	}, kafkaSecret); err != nil {
		return "", "", err
	}

	return string(kafkaSecret.Data["bootstrap_server"]),
		base64.RawStdEncoding.EncodeToString(kafkaSecret.Data["CA"]), nil
}
