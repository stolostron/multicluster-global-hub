package agent

import (
	"context"
	"fmt"
	"reflect"
	"strings"
	"time"

	appsv1 "k8s.io/api/apps/v1"
	corev1 "k8s.io/api/core/v1"
	rbacv1 "k8s.io/api/rbac/v1"
	"k8s.io/apimachinery/pkg/api/errors"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	clusterv1 "open-cluster-management.io/api/cluster/v1"
	ctrl "sigs.k8s.io/controller-runtime"
	"sigs.k8s.io/controller-runtime/pkg/builder"
	"sigs.k8s.io/controller-runtime/pkg/client"
	"sigs.k8s.io/controller-runtime/pkg/event"
	"sigs.k8s.io/controller-runtime/pkg/handler"
	"sigs.k8s.io/controller-runtime/pkg/predicate"

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
	agentName            = "multicluster-global-hub-agent"
)

type LocalAgentController struct {
	ctrl.Manager
}

func StartLocalAgentController(initOption config.ControllerOption) (config.ControllerInterface, error) {
	if localAgentReconciler != nil {
		return localAgentReconciler, nil
	}

	log.Info("start local agent controller")

	if !config.IsTransportConfigReady(initOption.Ctx, initOption.MulticlusterGlobalHub.Namespace,
		initOption.Manager.GetClient()) {
		return nil, nil
	}

	localAgentReconciler = &LocalAgentController{
		Manager: initOption.Manager,
	}

	err := ctrl.NewControllerManagedBy(initOption.Manager).
		Named("local-agent-reconciler").
		Watches(&v1alpha4.MulticlusterGlobalHub{},
			&handler.EnqueueRequestForObject{}).
		Watches(&clusterv1.ManagedCluster{},
			&handler.EnqueueRequestForObject{}, builder.WithPredicates(clusterPred)).
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

var clusterPred = predicate.Funcs{
	CreateFunc: func(e event.CreateEvent) bool {
		if e.Object.GetLabels() == nil {
			return false
		}
		return e.Object.GetLabels()[constants.LocalClusterName] == "true"
	},
	UpdateFunc: func(e event.UpdateEvent) bool {
		return false
	},
	DeleteFunc: func(e event.DeleteEvent) bool {
		if e.Object.GetLabels() == nil {
			return false
		}
		return e.Object.GetLabels()[constants.LocalClusterName] == "true"
	},
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
// +kubebuilder:rbac:groups=agent.open-cluster-management.io,resources=klusterletaddonconfigs,verbs=get;create;watch;list;delete;patch;update
// +kubebuilder:rbac:groups=extensions.hive.openshift.io,resources=imageclusterinstalls;imageclusterinstalls/status,verbs=create;delete;get;list;update;watch
// +kubebuilder:rbac:groups=hive.openshift.io,resources=clusterdeployments;clusterdeployments/status,verbs=create;delete;get;list;update;watch
// +kubebuilder:rbac:groups=metal3.io,resources=baremetalhosts;baremetalhosts/status;dataimages;dataimages/status;firmwareschemas;firmwareschemas/status;hostfirmwarecomponents;hostfirmwarecomponents/status;hostfirmwaresettings;hostfirmwaresettings/status,verbs=create;delete;get;list;update;watch
// +kubebuilder:rbac:groups=observability.open-cluster-management.io,resources=observabilityaddons,verbs=delete;get;list;update
// +kubebuilder:rbac:groups=register.open-cluster-management.io,resources=managedclusters/accept,verbs=update
// +kubebuilder:rbac:groups="apps",resources=deployments,verbs=get;list;watch;create;update;delete;deletecollection

func (s *LocalAgentController) Reconcile(ctx context.Context, req ctrl.Request) (ctrl.Result, error) {
	log.Debugf("reconcile local agent controller: %v", req)
	mgh, err := config.GetMulticlusterGlobalHub(ctx, s.GetClient())
	if err != nil {
		return ctrl.Result{}, nil
	}
	if mgh == nil || config.IsPaused(mgh) {
		return ctrl.Result{}, nil
	}
	if mgh.DeletionTimestamp != nil || !mgh.Spec.InstallAgentOnLocal {
		log.Debugf("deleting mgh in local agent controller")
		err = utils.HandleMghDelete(ctx, &isResourceRemoved, mgh.Namespace, s.pruneAgentResources)
		log.Debugf("deleted local agent resources, isResourceRemoved:%v", isResourceRemoved)
		return ctrl.Result{}, err
	}
	currentClusterName, err := getCurrentClusterName(ctx, s.GetClient(), mgh.Namespace)
	if err != nil {
		log.Errorf("failed to get the current cluster name: %v", err)
		return ctrl.Result{}, err
	}
	log.Debugf("current cluster name: %s", currentClusterName)

	localClusterName, err := GetLocalClusterName(ctx, s.GetClient(), mgh.Namespace)
	if err != nil {
		log.Errorf("failed to get the current cluster name: %v", err)
		return ctrl.Result{}, err
	}
	log.Debugf("local cluster name: %s", localClusterName)

	// If the local cluster name is different with cluster name in agent deploy,
	// we need to recreate the agent resources
	if localClusterName != "" && currentClusterName != "" && localClusterName != currentClusterName {
		log.Infof("local cluster name changed from %s to %s", currentClusterName, localClusterName)
		err := s.pruneAgentResources(ctx, mgh.Namespace)
		if err != nil {
			return ctrl.Result{}, err
		}
		isResourceRemoved = true
		return ctrl.Result{RequeueAfter: 5 * time.Second}, nil
	}

	isResourceRemoved = false
	if localClusterName != "" {
		clusterName = localClusterName
	}
	log.Debugf("generate local agent credential")
	err = GenerateLocalAgentCredential(ctx, s.GetClient(), mgh.Namespace)
	if err != nil {
		return ctrl.Result{}, err
	}

	log.Debugf("render agent resources")
	return renderAgentManifests(
		s.Manager,
		clusterName,
		getTransportSecretName(),
		nil, mgh,
		"local", // deploy mode is local agent
	)
}

// getCurrentClusterName returns the current cluster name from the agent deployment
func getCurrentClusterName(ctx context.Context, c client.Client, namespace string) (string, error) {
	deploy := &appsv1.Deployment{}
	err := c.Get(ctx, client.ObjectKey{
		Name:      agentName,
		Namespace: namespace,
	}, deploy)
	if err != nil {
		if errors.IsNotFound(err) {
			return "", nil
		}
		return "", fmt.Errorf("failed to get the deployment: %v", err)
	}
	for _, arg := range deploy.Spec.Template.Spec.Containers[0].Args {
		if !strings.HasPrefix(arg, "--leaf-hub-name=") {
			continue
		}
		return strings.ReplaceAll(arg, "--leaf-hub-name=", ""), nil
	}
	return "", nil
}

// getLocalClusterName returns the local cluster name from the managedcluster
func GetLocalClusterName(ctx context.Context, c client.Client, namespace string) (string, error) {
	mcList := &clusterv1.ManagedClusterList{}
	listOptions := []client.ListOption{
		client.MatchingLabels(map[string]string{
			constants.LocalClusterName: "true",
		}),
	}
	err := c.List(ctx, mcList, listOptions...)
	if err != nil {
		return "", fmt.Errorf("failed to list managedclusters: %v", err)
	}
	if len(mcList.Items) == 0 {
		config.SetLocalClusterName("")
		return "", nil
	}
	if len(mcList.Items) > 1 {
		return "", fmt.Errorf("found more than one local cluster: %v", mcList.Items)
	}
	config.SetLocalClusterName(mcList.Items[0].Name)
	return mcList.Items[0].Name, nil
}

func (s *LocalAgentController) pruneAgentResources(ctx context.Context, namespace string) error {
	log.Debugf("prune agent resources in namespace: %v", namespace)
	// delete deployment

	err := utils.DeleteResourcesWithLabels(ctx, s.GetClient(), namespace, map[string]string{
		"component": agentName,
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
	err = trans.Prune(clusterName)
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
	log.Debugf("kafkaConnection bootstrap server: %s, cluster ID: %s", kafkaConnection.BootstrapServer, kafkaConnection.ClusterID)
	kafkaConfigYaml, err := kafkaConnection.YamlMarshal(true)
	if err != nil {
		return fmt.Errorf("failed to marshalling the kafka config yaml: %w", err)
	}

	expectedSecret := &corev1.Secret{
		ObjectMeta: metav1.ObjectMeta{
			Name:      getTransportSecretName(),
			Namespace: namespace,
			Labels: map[string]string{
				"component": agentName,
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
		log.Infof("create transport secret %v for local agent", getTransportSecretName())
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

func getTransportSecretName() string {
	return constants.GHTransportConfigSecret + "-" + clusterName
}
