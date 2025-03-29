package agent

import (
	"context"
	"fmt"
	"reflect"
	"strconv"

	kafkav1beta2 "github.com/RedHatInsights/strimzi-client-go/apis/kafka.strimzi.io/v1beta2"
	appsv1 "k8s.io/api/apps/v1"
	corev1 "k8s.io/api/core/v1"
	rbacv1 "k8s.io/api/rbac/v1"
	"k8s.io/apimachinery/pkg/api/errors"
	"k8s.io/apimachinery/pkg/api/resource"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/client-go/discovery"
	"k8s.io/client-go/discovery/cached/memory"
	"k8s.io/client-go/restmapper"
	ctrl "sigs.k8s.io/controller-runtime"
	"sigs.k8s.io/controller-runtime/pkg/builder"
	"sigs.k8s.io/controller-runtime/pkg/client"
	"sigs.k8s.io/controller-runtime/pkg/handler"

	"github.com/stolostron/multicluster-global-hub/operator/api/operator/v1alpha4"
	"github.com/stolostron/multicluster-global-hub/operator/pkg/config"
	operatorconstants "github.com/stolostron/multicluster-global-hub/operator/pkg/constants"
	"github.com/stolostron/multicluster-global-hub/operator/pkg/controllers/agent/addon"
	"github.com/stolostron/multicluster-global-hub/operator/pkg/deployer"
	"github.com/stolostron/multicluster-global-hub/operator/pkg/renderer"
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
			&handler.EnqueueRequestForObject{}, builder.WithPredicates(config.GeneralPredicate)).
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

// +kubebuilder:rbac:groups=operator.open-cluster-management.io,resources=multiclusterglobalhubagents,verbs=get;list;watch;create;update;patch;delete
// +kubebuilder:rbac:groups=operator.open-cluster-management.io,resources=multiclusterglobalhubagents/status,verbs=get;update;patch
// +kubebuilder:rbac:groups=operator.open-cluster-management.io,resources=multiclusterglobalhubagents/finalizers,verbs=update
// +kubebuilder:rbac:groups="config.openshift.io",resources=infrastructures;clusterversions,verbs=get;list;watch
// +kubebuilder:rbac:groups="policy.open-cluster-management.io",resources=policyautomations;policysets;placementbindings;policies,verbs=get;list;watch;patch;update
// +kubebuilder:rbac:groups="cluster.open-cluster-management.io",resources=placements;managedclustersets;managedclustersetbindings,verbs=get;list;watch;patch;update
// +kubebuilder:rbac:groups="cluster.open-cluster-management.io",resources=managedclusters;managedclusters/finalizers;placementdecisions;placementdecisions/finalizers;placements;placements/finalizers,verbs=get;list;watch;patch;update
// +kubebuilder:rbac:groups="cluster.open-cluster-management.io",resources=clusterclaims,verbs=create;get;list;watch;patch;update;delete
// +kubebuilder:rbac:groups="",resources=namespaces;pods;events,verbs=create;get;list;watch;patch;update;delete
// +kubebuilder:rbac:groups="apiextensions.k8s.io",resources=customresourcedefinitions,verbs=list;watch
// +kubebuilder:rbac:groups="route.openshift.io",resources=routes,verbs=get;list;watch
// +kubebuilder:rbac:groups="internal.open-cluster-management.io",resources=managedclusterinfos,verbs=get;list;watch;update
// +kubebuilder:rbac:groups="apps.open-cluster-management.io",resources=placementrules;subscriptionreports,verbs=get;list;watch;update;patch
// +kubebuilder:rbac:groups="coordination.k8s.io",resources=leases,verbs=create;get;list;watch;patch;update
// +kubebuilder:rbac:groups="rbac.authorization.k8s.io",resources=clusterrolebindings;clusterroles,verbs=get;list;watch;create;update;delete;deletecollection
// +kubebuilder:rbac:groups="",resources=services;secrets;configmaps;serviceaccounts,verbs=get;list;watch;create;update;delete;deletecollection
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
	log.Debugf("render agent resources")
	// create new HoHRenderer and HoHDeployer
	hohRenderer, hohDeployer := renderer.NewHoHRenderer(fs), deployer.NewHoHDeployer(s.GetClient())

	// create discovery client
	dc, err := discovery.NewDiscoveryClientForConfig(s.Manager.GetConfig())
	if err != nil {
		return ctrl.Result{}, err
	}

	imagePullPolicy := corev1.PullAlways
	if mgh.Spec.ImagePullPolicy != "" {
		imagePullPolicy = mgh.Spec.ImagePullPolicy
	}
	agentQPS, agentBurst := config.GetAgentRestConfig()

	// set resource requirements
	resourceReq := corev1.ResourceRequirements{}
	requests := corev1.ResourceList{
		corev1.ResourceName(corev1.ResourceMemory): resource.MustParse(operatorconstants.AgentMemoryRequest),
		corev1.ResourceName(corev1.ResourceCPU):    resource.MustParse(operatorconstants.AgentCPURequest),
	}
	if mgh.Spec.AdvancedSpec != nil && mgh.Spec.AdvancedSpec.Agent != nil && mgh.Spec.AdvancedSpec.Agent.Resources != nil {
		utils.SetResourcesFromCR(mgh.Spec.AdvancedSpec.Agent.Resources, requests)
		resourceReq.Requests = requests
	}

	electionConfig, err := config.GetElectionConfig()
	if err != nil {
		log.Errorw("failed to get election config", "error", err)
		return ctrl.Result{}, err
	}

	// create restmapper for deployer to find GVR
	mapper := restmapper.NewDeferredDiscoveryRESTMapper(memory.NewMemCacheClient(dc))

	agentObjects, err := hohRenderer.Render("manifests", "", func(profile string) (interface{}, error) {
		return struct {
			Image           string
			ImagePullSecret string
			ImagePullPolicy string
			Namespace       string
			NodeSelector    map[string]string
			Tolerations     []corev1.Toleration
			LeaseDuration   string
			RenewDeadline   string
			RetryPeriod     string
			AgentQPS        float32
			AgentBurst      int
			LogLevel        string
			ClusterId       string
			Resources       *corev1.ResourceRequirements
		}{
			Image:           config.GetImage(config.GlobalHubAgentImageKey),
			ImagePullSecret: mgh.Spec.ImagePullSecret,
			ImagePullPolicy: string(imagePullPolicy),
			Namespace:       mgh.Namespace,
			NodeSelector:    mgh.Spec.NodeSelector,
			Tolerations:     mgh.Spec.Tolerations,
			LeaseDuration:   strconv.Itoa(electionConfig.LeaseDuration),
			RenewDeadline:   strconv.Itoa(electionConfig.RenewDeadline),
			RetryPeriod:     strconv.Itoa(electionConfig.RetryPeriod),
			AgentQPS:        agentQPS,
			AgentBurst:      agentBurst,
			ClusterId:       clusterName,
			Resources:       &resourceReq,
		}, nil
	})
	if err != nil {
		return ctrl.Result{}, fmt.Errorf("failed to render local agent objects: %v", err)
	}
	if err = utils.ManipulateGlobalHubObjects(agentObjects, mgh, hohDeployer, mapper, s.GetScheme()); err != nil {
		return ctrl.Result{}, fmt.Errorf("failed to create/update local agent objects: %v", err)
	}
	log.Debugf("render agent resources success")
	return ctrl.Result{}, nil
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

	err = c.Delete(ctx, &kafkav1beta2.KafkaUser{
		ObjectMeta: metav1.ObjectMeta{
			Name:      config.GetKafkaUserName(constants.LocalClusterName),
			Namespace: namespace,
		},
	})
	if err != nil && !errors.IsNotFound(err) {
		return err
	}

	err = c.Delete(ctx, &kafkav1beta2.KafkaTopic{
		ObjectMeta: metav1.ObjectMeta{
			Name:      config.GetStatusTopic(constants.LocalClusterName),
			Namespace: namespace,
		},
	})
	if err != nil && !errors.IsNotFound(err) {
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
