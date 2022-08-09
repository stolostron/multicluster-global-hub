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

	corev1 "k8s.io/api/core/v1"
	rbacv1 "k8s.io/api/rbac/v1"
	"k8s.io/apimachinery/pkg/api/equality"
	"k8s.io/apimachinery/pkg/api/errors"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/apis/meta/v1/unstructured"
	"k8s.io/apimachinery/pkg/runtime"
	"k8s.io/apimachinery/pkg/types"
	ctrl "sigs.k8s.io/controller-runtime"
	"sigs.k8s.io/controller-runtime/pkg/builder"
	"sigs.k8s.io/controller-runtime/pkg/client"
	"sigs.k8s.io/controller-runtime/pkg/controller/controllerutil"
	"sigs.k8s.io/controller-runtime/pkg/event"
	ctrllog "sigs.k8s.io/controller-runtime/pkg/log"
	"sigs.k8s.io/controller-runtime/pkg/manager"
	"sigs.k8s.io/controller-runtime/pkg/predicate"

	"github.com/go-logr/logr"
	hubofhubsv1alpha1 "github.com/stolostron/hub-of-hubs/operator/apis/hubofhubs/v1alpha1"
	"github.com/stolostron/hub-of-hubs/operator/pkg/config"
	"github.com/stolostron/hub-of-hubs/operator/pkg/constants"
	leafhubscontroller "github.com/stolostron/hub-of-hubs/operator/pkg/controllers/leafhub"

	// pmcontroller "github.com/stolostron/hub-of-hubs/operator/pkg/controllers/packagemanifest"
	"github.com/stolostron/hub-of-hubs/operator/pkg/condition"
	"github.com/stolostron/hub-of-hubs/operator/pkg/deployer"
	"github.com/stolostron/hub-of-hubs/operator/pkg/renderer"
	"github.com/stolostron/hub-of-hubs/operator/pkg/utils"
)

//go:embed manifests
var fs embed.FS

var isLeafHubControllerRunnning = false

// var isPackageManifestControllerRunnning = false

// ConfigReconciler reconciles a Config object
type ConfigReconciler struct {
	manager.Manager
	client.Client
	Scheme *runtime.Scheme
}

// SetConditionFunc is function type that receives the concrete condition method
type SetConditionFunc func(ctx context.Context, c client.Client, config *hubofhubsv1alpha1.Config,
	status metav1.ConditionStatus) error

//+kubebuilder:rbac:groups=hubofhubs.open-cluster-management.io,resources=configs,verbs=get;list;watch;create;update;patch;delete
//+kubebuilder:rbac:groups=hubofhubs.open-cluster-management.io,resources=configs/status,verbs=get;update;patch
//+kubebuilder:rbac:groups=hubofhubs.open-cluster-management.io,resources=configs/finalizers,verbs=update
//+kubebuilder:rbac:groups=cluster.open-cluster-management.io,resources=managedclustersetbindings,verbs=get;list;watch;create;update;patch;delete
//+kubebuilder:rbac:groups=cluster.open-cluster-management.io,resources=managedclustersets/join,verbs=create;delete
//+kubebuilder:rbac:groups=cluster.open-cluster-management.io,resources=managedclustersets/bind,verbs=create;delete

//+kubebuilder:rbac:groups="",resources=namespaces,verbs=get;list;watch;create;update;delete
//+kubebuilder:rbac:groups="",resources=services,verbs=get;create;update;delete
//+kubebuilder:rbac:groups="",resources=serviceaccounts,verbs=get;create;update;delete
//+kubebuilder:rbac:groups="",resources=secrets,verbs=get;list;watch;create;update;delete
//+kubebuilder:rbac:groups="",resources=configmaps,verbs=get;create;update;delete
//+kubebuilder:rbac:groups="apps",resources=deployments,verbs=get;create;update;delete
//+kubebuilder:rbac:groups="apps",resources=deployments,verbs=get;create;update;delete
//+kubebuilder:rbac:groups="networking.k8s.io",resources=ingresses,verbs=get;create;update;delete
//+kubebuilder:rbac:groups="networking.k8s.io",resources=networkpolicies,verbs=get;create;update;delete
//+kubebuilder:rbac:groups="rbac.authorization.k8s.io",resources=roles,verbs=get;create;update;delete
//+kubebuilder:rbac:groups="rbac.authorization.k8s.io",resources=rolebindings,verbs=get;create;update;delete
//+kubebuilder:rbac:groups="rbac.authorization.k8s.io",resources=clusterroles,verbs=get;list;create;update;delete
//+kubebuilder:rbac:groups="rbac.authorization.k8s.io",resources=clusterrolebindings,verbs=get;list;create;update;delete

// Reconcile is part of the main kubernetes reconciliation loop which aims to
// move the current state of the cluster closer to the desired state.
// TODO(user): Modify the Reconcile function to compare the state specified by
// the Config object against the actual cluster state, and then
// perform operations to make the cluster state reflect the state specified by
// the user.
//
// For more details, check Reconcile and its Result here:
// - https://pkg.go.dev/sigs.k8s.io/controller-runtime@v0.11.0/pkg/reconcile
func (r *ConfigReconciler) Reconcile(ctx context.Context, req ctrl.Request) (ctrl.Result, error) {
	log := ctrllog.FromContext(ctx)

	// Fetch the hub-of-hubs config instance
	hohConfig := &hubofhubsv1alpha1.Config{}
	err := r.Get(ctx, req.NamespacedName, hohConfig)
	if err != nil {
		if errors.IsNotFound(err) {
			// Request object not found, could have been deleted after reconcile request.
			// Owned objects are automatically garbage collected. For additional cleanup logic use finalizers.
			// Return and don't requeue
			log.Info("Config resource not found. Ignoring since object must be deleted")
			return ctrl.Result{}, nil
		}
		// Error reading the object - requeue the request.
		log.Error(err, "Failed to get Config")
		return ctrl.Result{}, err
	}

	if err := condition.SetConditionResourceFound(ctx, r.Client, hohConfig); err != nil {
		log.Error(err, "Failed to set condition resource found")
		return ctrl.Result{}, err
	}

	// handle gc
	isTerminating, err := r.initFinalization(ctx, hohConfig, log)
	if err != nil {
		return ctrl.Result{}, err
	}

	if isTerminating {
		log.Info("hoh config is terminating, skip the reconcile")
		return ctrl.Result{}, err
	}

	// init DB and transport here
	err = r.reconcileDatabase(ctx, hohConfig, types.NamespacedName{
		Name:      hohConfig.Spec.PostgreSQL.Name,
		Namespace: constants.HOHDefaultNamespace,
	})
	if err != nil {
		return ctrl.Result{}, err
	}

	// reconcile hoh-system namespace and hub-of-hubs configuration
	err = r.reconcileHoHResources(ctx, hohConfig)
	if err != nil {
		return ctrl.Result{}, err
	}

	// create new HoHRenderer and HoHDeployer
	hohRenderer := renderer.NewHoHRenderer(fs)

	annotations := hohConfig.GetAnnotations()
	hohRBACObjects, err := hohRenderer.Render("manifests/rbac", func(component string) (interface{}, error) {
		hohRBACConfig := struct {
			Image string
		}{
			Image: config.GetImage(annotations, "hub_of_hubs_rbac"),
		}

		return hohRBACConfig, err
	})
	if err != nil {
		if conditionError := condition.SetConditionRBACDeployed(ctx, r.Client, hohConfig, condition.CONDITION_STATUS_FALSE); conditionError != nil {
			return ctrl.Result{}, fmt.Errorf("failed to set condition(%s): %w", condition.CONDITION_STATUS_FALSE, conditionError)
		}
		return ctrl.Result{}, err
	}

	err = r.manipulateObj(ctx, hohRBACObjects, hohConfig, condition.SetConditionRBACDeployed, log)
	if err != nil {
		return ctrl.Result{}, err
	}

	if conditionError := condition.SetConditionRBACDeployed(ctx, r.Client, hohConfig, condition.CONDITION_STATUS_TRUE); conditionError != nil {
		return ctrl.Result{}, fmt.Errorf("failed to set condition(%s): %w", condition.CONDITION_STATUS_TRUE, conditionError)
	}

	// retrieve bootstrapserver and CA of kafka from secret
	kafkaBootstrapServer, kafkaCA, err := getKafkaConfig(ctx, r.Client, log, hohConfig)
	if err != nil {
		if conditionError := condition.SetConditionTransportInit(ctx, r.Client, hohConfig, condition.CONDITION_STATUS_FALSE); conditionError != nil {
			return ctrl.Result{}, fmt.Errorf("failed to set condition(%s): %w", condition.CONDITION_STATUS_FALSE, conditionError)
		}
		return ctrl.Result{}, err
	}

	if conditionError := condition.SetConditionTransportInit(ctx, r.Client, hohConfig, condition.CONDITION_STATUS_TRUE); conditionError != nil {
		return ctrl.Result{}, fmt.Errorf("failed to set condition(%s): %w", condition.CONDITION_STATUS_TRUE, conditionError)
	}

	managerObjects, err := hohRenderer.Render("manifests/manager", func(component string) (interface{}, error) {
		managerConfig := struct {
			Image                string
			DBSecret             string
			KafkaCA              string
			KafkaBootstrapServer string
		}{
			Image:                config.GetImage(annotations, "hub_of_hubs_manager"),
			DBSecret:             hohConfig.Spec.PostgreSQL.Name,
			KafkaCA:              kafkaCA,
			KafkaBootstrapServer: kafkaBootstrapServer,
		}

		return managerConfig, err
	})
	if err != nil {
		if conditionError := condition.SetConditionManagerDeployed(ctx, r.Client, hohConfig, condition.CONDITION_STATUS_FALSE); conditionError != nil {
			return ctrl.Result{}, fmt.Errorf("failed to set condition(%s): %w", condition.CONDITION_STATUS_FALSE, conditionError)
		}
		return ctrl.Result{}, err
	}

	err = r.manipulateObj(ctx, managerObjects, hohConfig, condition.SetConditionManagerDeployed, log)
	if err != nil {
		return ctrl.Result{}, err
	}

	// render the default placement
	placementObjects, err := hohRenderer.Render("manifests/placement", func(component string) (interface{}, error) {
		return struct{}{}, err
	})
	if err != nil {
		return ctrl.Result{}, err
	}

	err = r.manipulateObj(ctx, placementObjects, hohConfig, nil, log)
	if err != nil {
		return ctrl.Result{}, err
	}

	if conditionError := condition.SetConditionManagerDeployed(ctx, r.Client, hohConfig, condition.CONDITION_STATUS_TRUE); conditionError != nil {
		return ctrl.Result{}, fmt.Errorf("failed to set condition(%s): %w", condition.CONDITION_STATUS_TRUE, conditionError)
	}

	// try to start leafhub controller if it is not running
	if !isLeafHubControllerRunnning {
		if err := (&leafhubscontroller.LeafHubReconciler{
			Client: r.Client,
			Scheme: r.Scheme,
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

func (r *ConfigReconciler) manipulateObj(ctx context.Context, objs []*unstructured.Unstructured,
	hohConfig *hubofhubsv1alpha1.Config, setConditionFunc SetConditionFunc, log logr.Logger) error {
	hohDeployer := deployer.NewHoHDeployer(r.Client)
	// manipulate the object
	for _, obj := range objs {
		// TODO: more solid way to check if the object is namespace scoped resource
		if obj.GetNamespace() != "" {
			// set ownerreference of controller
			if err := controllerutil.SetControllerReference(hohConfig, obj, r.Scheme); err != nil {
				log.Error(err, "failed to set controller reference", "kind", obj.GetKind(),
					"namespace", obj.GetNamespace(), "name", obj.GetName())
			}
		}
		// set owner labels
		labels := obj.GetLabels()
		if labels == nil {
			labels = make(map[string]string)
		}
		labels[constants.HoHOperatorOwnerLabelKey] = hohConfig.GetName()
		obj.SetLabels(labels)

		log.Info("Creating or updating object", "object", obj)
		if err := hohDeployer.Deploy(obj); err != nil {
			if setConditionFunc != nil {
				conditionError := setConditionFunc(ctx, r.Client, hohConfig, condition.CONDITION_STATUS_FALSE)
				if conditionError != nil {
					return fmt.Errorf("failed to set condition(%s): %w", condition.CONDITION_STATUS_FALSE, conditionError)
				}
			}
			return err
		}
	}
	return nil
}

func (r *ConfigReconciler) initFinalization(ctx context.Context, hohConfig *hubofhubsv1alpha1.Config,
	log logr.Logger) (bool, error) {
	if hohConfig.GetDeletionTimestamp() != nil &&
		utils.Contains(hohConfig.GetFinalizers(), constants.HoHOperatorFinalizer) {
		log.Info("to delete hoh resources")
		// clean up the cluster resources, eg. clusterrole, clusterrolebinding, etc
		if err := r.pruneGlobalResources(ctx, hohConfig); err != nil {
			log.Error(err, "failed to remove cluster scoped resources")
			return false, err
		}

		// clean up hoh-system namespace and hub-of-hubs configuration
		if err := r.pruneHoHResources(ctx, hohConfig); err != nil {
			log.Error(err, "failed to remove hub-of-hubs resources")
			return false, err
		}

		hohConfig.SetFinalizers(utils.Remove(hohConfig.GetFinalizers(), constants.HoHOperatorFinalizer))
		err := r.Client.Update(context.TODO(), hohConfig)
		if err != nil {
			log.Error(err, "failed to remove finalizer from hoh config resource")
			return false, err
		}
		log.Info("finalizer is removed from hoh config resource")

		return true, nil
	}
	if !utils.Contains(hohConfig.GetFinalizers(), constants.HoHOperatorFinalizer) {
		hohConfig.SetFinalizers(append(hohConfig.GetFinalizers(), constants.HoHOperatorFinalizer))
		err := r.Client.Update(context.TODO(), hohConfig)
		if err != nil {
			log.Error(err, "failed to add finalizer to hoh config resource")
			return false, err
		}
		log.Info("finalizer is added to hoh config resource")
	}

	return false, nil
}

// pruneGlobalResources deletes the cluster scoped resources created by the hub-of-hubs-operator
// cluster scoped resources need to be deleted manually because they don't have ownerrefenence set
func (r *ConfigReconciler) pruneGlobalResources(ctx context.Context, hohConfig *hubofhubsv1alpha1.Config) error {
	listOpts := []client.ListOption{
		client.MatchingLabels(map[string]string{constants.HoHOperatorOwnerLabelKey: hohConfig.GetName()}),
	}

	clusterRoleList := &rbacv1.ClusterRoleList{}
	err := r.Client.List(ctx, clusterRoleList, listOpts...)
	if err != nil {
		return err
	}
	for idx := range clusterRoleList.Items {
		if err := r.Client.Delete(ctx, &clusterRoleList.Items[idx], &client.DeleteOptions{}); err != nil &&
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
		if err := r.Client.Delete(ctx, &clusterRoleBindingList.Items[idx], &client.DeleteOptions{}); err != nil &&
			!errors.IsNotFound(err) {
			return err
		}
	}

	return nil
}

// reconcileHoHResources tries to create hoh resources if they don't exist
func (r *ConfigReconciler) reconcileHoHResources(ctx context.Context, hohConfig *hubofhubsv1alpha1.Config) error {
	if err := r.Client.Get(ctx,
		types.NamespacedName{
			Name: constants.HOHSystemNamespace,
		}, &corev1.Namespace{}); err != nil {
		if errors.IsNotFound(err) {
			if err := r.Client.Create(ctx, &corev1.Namespace{
				ObjectMeta: metav1.ObjectMeta{
					Name: constants.HOHSystemNamespace,
					Labels: map[string]string{
						constants.HoHOperatorOwnerLabelKey: hohConfig.GetName(),
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
				constants.HoHOperatorOwnerLabelKey: hohConfig.GetName(),
			},
		},
		Data: map[string]string{
			"aggregationLevel":    string(hohConfig.Spec.AggregationLevel),
			"enableLocalPolicies": strconv.FormatBool(hohConfig.Spec.EnableLocalPolicies),
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
func (r *ConfigReconciler) pruneHoHResources(ctx context.Context, hohConfig *hubofhubsv1alpha1.Config) error {
	// hoh configmap
	existingHoHConfigMap := &corev1.ConfigMap{}
	if err := r.Client.Get(ctx,
		types.NamespacedName{
			Namespace: constants.HOHSystemNamespace,
			Name:      constants.HOHConfigName,
		}, existingHoHConfigMap); err != nil && !errors.IsNotFound(err) {
		return err
	}

	// clean the finalizers added by hub-of-hubs-manager
	existingHoHConfigMap.SetFinalizers([]string{})
	if err := r.Client.Update(ctx, existingHoHConfigMap); err != nil {
		return err
	}

	if err := r.Client.Delete(ctx, existingHoHConfigMap); err != nil && !errors.IsNotFound(err) {
		return err
	}

	hohSystemNamespace := &corev1.Namespace{
		ObjectMeta: metav1.ObjectMeta{
			Name: constants.HOHSystemNamespace,
			Labels: map[string]string{
				constants.HoHOperatorOwnerLabelKey: hohConfig.GetName(),
			},
		},
	}

	if err := r.Client.Delete(ctx, hohSystemNamespace); err != nil && !errors.IsNotFound(err) {
		return err
	}

	return nil
}

// SetupWithManager sets up the controller with the Manager.
func (r *ConfigReconciler) SetupWithManager(mgr ctrl.Manager) error {
	hohConfigPred := predicate.Funcs{
		CreateFunc: func(e event.CreateEvent) bool {
			// set request name to be used in leafhub controller
			config.SetHoHConfigNamespacedName(types.NamespacedName{Namespace: e.Object.GetNamespace(), Name: e.Object.GetName()})
			return true
		},
		UpdateFunc: func(e event.UpdateEvent) bool {
			return e.ObjectOld.GetResourceVersion() != e.ObjectNew.GetResourceVersion()
		},
		DeleteFunc: func(e event.DeleteEvent) bool {
			return !e.DeleteStateUnknown
		},
	}

	return ctrl.NewControllerManagedBy(mgr).
		For(&hubofhubsv1alpha1.Config{}, builder.WithPredicates(hohConfigPred)).
		Complete(r)
}

// getKafkaConfig retrieves kafka server and CA from kafka secret
func getKafkaConfig(ctx context.Context, c client.Client, log logr.Logger, hohConfig *hubofhubsv1alpha1.Config) (
	string, string, error) {
	// for local dev/test
	kafkaBootstrapServer, ok := hohConfig.GetAnnotations()[constants.HoHKafkaBootstrapServerKey]
	if ok && kafkaBootstrapServer != "" {
		log.Info("Kafka bootstrap server from annotation", "server", kafkaBootstrapServer, "certificate", "")
		return kafkaBootstrapServer, "", nil
	}

	kafkaSecret := &corev1.Secret{}
	if err := c.Get(ctx, types.NamespacedName{
		Namespace: constants.HOHDefaultNamespace,
		Name:      hohConfig.Spec.Kafka.Name,
	}, kafkaSecret); err != nil {
		return "", "", err
	}

	return string(kafkaSecret.Data["bootstrap_server"]), base64.RawStdEncoding.EncodeToString(kafkaSecret.Data["CA"]), nil
}
