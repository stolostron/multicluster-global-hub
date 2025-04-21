package agent

import (
	"context"
	"fmt"
	"reflect"

	appsv1 "k8s.io/api/apps/v1"
	corev1 "k8s.io/api/core/v1"
	rbacv1 "k8s.io/api/rbac/v1"
	"k8s.io/apimachinery/pkg/api/errors"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	ctrl "sigs.k8s.io/controller-runtime"
	"sigs.k8s.io/controller-runtime/pkg/builder"
	"sigs.k8s.io/controller-runtime/pkg/client"
	"sigs.k8s.io/controller-runtime/pkg/handler"

	"github.com/stolostron/multicluster-global-hub/operator/api/operator/shared"
	"github.com/stolostron/multicluster-global-hub/operator/api/operator/v1alpha4"
	"github.com/stolostron/multicluster-global-hub/operator/pkg/config"
	"github.com/stolostron/multicluster-global-hub/operator/pkg/controllers/agent/addon"
	"github.com/stolostron/multicluster-global-hub/operator/pkg/utils"
	"github.com/stolostron/multicluster-global-hub/pkg/constants"
	"github.com/stolostron/multicluster-global-hub/pkg/logger"
)

var (
	log                  = logger.DefaultZapLogger()
	isResourceRemoved    = true
	localAgentReconciler *LocalAgentController
	clusterName          = constants.LocalClusterName
	transportSecretName  = constants.GHTransportConfigSecret + "-" + clusterName
)

type LocalAgentController struct {
	ctrl.Manager
}

func StartLocalAgentController(initOption config.ControllerOption) (config.ControllerInterface, error) {
	if localAgentReconciler != nil {
		return localAgentReconciler, nil
	}

	log.Info("start local agent controller")

	if config.GetTransporterConn() == nil {
		return nil, nil
	}

	localAgentReconciler = &LocalAgentController{
		Manager: initOption.Manager,
	}

	err := ctrl.NewControllerManagedBy(initOption.Manager).
		Named("local-agent-reconciler").
		Watches(&v1alpha4.MulticlusterGlobalHub{},
			&handler.EnqueueRequestForObject{}).
		Watches(&appsv1.Deployment{},
			&handler.EnqueueRequestForObject{}, builder.WithPredicates(deplomentPred)).
		Watches(&corev1.ConfigMap{},
			&handler.EnqueueRequestForObject{}, builder.WithPredicates(configMapPredicate)).
		Watches(&corev1.ServiceAccount{},
			&handler.EnqueueRequestForObject{}, builder.WithPredicates(config.GeneralPredicate)).
		Watches(&rbacv1.ClusterRole{},
			&handler.EnqueueRequestForObject{}, builder.WithPredicates(config.GeneralPredicate)).
		Watches(&rbacv1.ClusterRoleBinding{},
			&handler.EnqueueRequestForObject{}, builder.WithPredicates(config.GeneralPredicate)).
		Complete(localAgentReconciler)
	if err != nil {
		return nil, err
	}

	return localAgentReconciler, nil
}

func (s *LocalAgentController) IsResourceRemoved() bool {
	log.Infof("LocalAgentController resource removed: %v", isResourceRemoved)
	return isResourceRemoved
}

// +kubebuilder:rbac:groups=app.k8s.io,resources=applications,verbs=create;delete;get;list;patch;update;watch
// +kubebuilder:rbac:groups=apps.open-cluster-management.io,resources=channels;placementrules;subscriptionreports;subscriptions;subscriptionstatuses,verbs=create;delete;get;list;patch;update;watch
// +kubebuilder:rbac:groups=policy.open-cluster-management.io,resources=placementbindings;policies;policyautomations;policysets,verbs=create;delete;get;list;patch;update;watch
// +kubebuilder:rbac:groups=cluster.open-cluster-management.io,resources=placements;managedclustersets;managedclustersetbindings,verbs=create;delete;get;list;patch;update;watch
// +kubebuilder:rbac:groups=cluster.open-cluster-management.io,resources=managedclustersets/join;managedclustersets/bind,verbs=create;delete
// +kubebuilder:rbac:groups=cluster.open-cluster-management.io,resources=managedclusters;managedclusters/finalizers;placementdecisions;placementdecisions/finalizers;placements;placements/finalizers,verbs=get;list;watch;update
// +kubebuilder:rbac:groups="",resources=namespaces;pods;configmaps;events;secrets;services;serviceaccounts,verbs=create;delete;get;list;patch;update;watch;deletecollection
// +kubebuilder:rbac:groups=coordination.k8s.io,resources=leases,verbs=get;create;update;delete
// +kubebuilder:rbac:groups="",resources=users;groups;serviceaccounts,verbs=impersonate
// +kubebuilder:rbac:groups=coordination.k8s.io,resources=leases,verbs=create;delete;get;list;patch;update;watch
// +kubebuilder:rbac:groups=rbac.authorization.k8s.io,resources=clusterrolebindings;clusterroles;rolebindings;roles,verbs=create;delete;get;list;patch;update;watch;deletecollection
// +kubebuilder:rbac:groups=apiextensions.k8s.io,resources=customresourcedefinitions,verbs=get;list;watch
// +kubebuilder:rbac:groups=operator.open-cluster-management.io,resources=multiclusterhubs;clustermanagers,verbs=get;list;watch;patch;update
// +kubebuilder:rbac:groups=cluster.open-cluster-management.io,resources=clusterclaims,verbs=create;delete;get;list;patch;update;watch
// +kubebuilder:rbac:groups=route.openshift.io,resources=routes,verbs=list;watch;get
// +kubebuilder:rbac:groups=platform.stackrox.io,resources=centrals,verbs=get;list;watch
// +kubebuilder:rbac:groups=config.openshift.io,resources=clusterversions,verbs=get;list;watch
// +kubebuilder:rbac:groups=internal.open-cluster-management.io,resources=managedclusterinfos,verbs=get;list;watch;update
// +kubebuilder:rbac:groups=config.open-cluster-management.io,resources=klusterletconfigs,verbs=create;delete;get;list;patch;update;watch
// +kubebuilder:rbac:groups=authorization.k8s.io,resources=subjectaccessreviews,verbs=create
// +kubebuilder:rbac:groups=certificates.k8s.io,resources=certificatesigningrequests,verbs=create;get;list;watch
// +kubebuilder:rbac:groups=cluster.open-cluster-management.io,resources=managedclusters,verbs=get;create;update;delete;watch;list
// +kubebuilder:rbac:groups=agent.open-cluster-management.io,resources=klusterletaddonconfigs,verbs=get;create;watch;list
// +kubebuilder:rbac:groups=register.open-cluster-management.io,resources=managedclusters/accept,verbs=update
// +kubebuilder:rbac:groups="apps",resources=deployments,verbs=get;list;watch;create;update;delete;deletecollection

func (s *LocalAgentController) Reconcile(ctx context.Context, req ctrl.Request) (ctrl.Result, error) {
	log.Debug("reconcile local agent controller")
	mgh, err := config.GetMulticlusterGlobalHub(ctx, s.GetClient())
	if err != nil {
		return ctrl.Result{}, nil
	}
	if mgh == nil || config.IsPaused(mgh) {
		return ctrl.Result{}, nil
	}
	if mgh.DeletionTimestamp != nil || !mgh.Spec.InstallAgentOnLocal {
		if isResourceRemoved {
			return ctrl.Result{}, nil
		}
		err := pruneAgentResources(ctx, s.GetClient(), mgh.Namespace)
		if err != nil {
			return ctrl.Result{}, err
		}
		isResourceRemoved = true
		return ctrl.Result{}, nil
	}
	isResourceRemoved = false
	log.Debugf("generate local agent credential")
	err = GenerateLocalAgentCredential(ctx, s.Manager.GetClient(), mgh.Namespace)
	if err != nil {
		return ctrl.Result{}, err
	}

	var resources *shared.ResourceRequirements
	if mgh.Spec.AdvancedSpec != nil &&
		mgh.Spec.AdvancedSpec.Agent != nil &&
		mgh.Spec.AdvancedSpec.Agent.Resources != nil {
		resources = mgh.Spec.AdvancedSpec.Agent.Resources
	}

	log.Debugf("render agent resources")
	return renderAgentManifests(
		ctx,
		mgh.Namespace,
		mgh.Spec.ImagePullPolicy,
		s.Manager,
		resources,
		mgh.Spec.ImagePullSecret,
		mgh.Spec.NodeSelector,
		mgh.Spec.Tolerations,
		mgh,
		clusterName,
	)
}

func pruneAgentResources(ctx context.Context, c client.Client, namespace string) error {
	log.Debugf("prune agent resources in namespace: %v", namespace)
	// delete deployment

	err := utils.DeleteResourcesWithLabels(ctx, c, namespace, map[string]string{
		"component": "multicluster-global-hub-agent",
	},
		[]client.Object{
			&appsv1.Deployment{},
			&corev1.ServiceAccount{},
			&rbacv1.ClusterRole{},
			&rbacv1.ClusterRoleBinding{},
			&corev1.ConfigMap{},
			&corev1.Secret{},
		})
	if err != nil {
		return err
	}

	trans := config.GetTransporter()
	if trans == nil {
		return fmt.Errorf("failed to get the transporter")
	}
	err = trans.Prune(constants.LocalClusterName)
	if err != nil {
		return err
	}
	return nil
}

func GenerateLocalAgentCredential(ctx context.Context, c client.Client, namespace string) error {
	log.Debugf("generate local agent credential in namespace: %v", namespace)
	err := addon.EnsureTransportResource(clusterName)
	if err != nil {
		return err
	}

	// will block until the credential is ready
	kafkaConnection, err := config.GetTransporter().GetConnCredential(clusterName)
	if err != nil {
		return err
	}
	log.Debugf("kafkaConnection: %v", *kafkaConnection)
	kafkaConfigYaml, err := kafkaConnection.YamlMarshal(true)
	if err != nil {
		return fmt.Errorf("failed to marshalling the kafka config yaml: %w", err)
	}

	expectedSecret := &corev1.Secret{
		ObjectMeta: metav1.ObjectMeta{
			Name:      transportSecretName,
			Namespace: namespace,
			Labels: map[string]string{
				"component": "multicluster-global-hub-agent",
			},
		},
		Data: map[string][]byte{
			"kafka.yaml": kafkaConfigYaml,
		},
	}

	existingSecret := &corev1.Secret{}
	err = c.Get(ctx, client.ObjectKeyFromObject(expectedSecret), existingSecret)
	if err != nil {
		if !errors.IsNotFound(err) {
			return err
		}
		log.Infof("create transport secret %v for local agent", transportSecretName)
		err := c.Create(ctx, expectedSecret)
		if err != nil {
			return fmt.Errorf("failed to create transport secret %w", err)
		}
	}
	if reflect.DeepEqual(existingSecret.Data, expectedSecret.Data) {
		return nil
	}

	return c.Update(ctx, expectedSecret)
}
