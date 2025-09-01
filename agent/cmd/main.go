package main

import (
	"context"
	"flag"
	"fmt"
	"os"
	"time"

	routev1 "github.com/openshift/api/route/v1"
	"github.com/spf13/pflag"
	clusterinfov1beta1 "github.com/stolostron/cluster-lifecycle-api/clusterinfo/v1beta1"
	coordinationv1 "k8s.io/api/coordination/v1"
	corev1 "k8s.io/api/core/v1"
	apiextensionsv1 "k8s.io/apiextensions-apiserver/pkg/apis/apiextensions/v1"
	"k8s.io/apimachinery/pkg/fields"
	_ "k8s.io/client-go/plugin/pkg/client/auth"
	"k8s.io/client-go/rest"
	clusterv1 "open-cluster-management.io/api/cluster/v1"
	clustersv1alpha1 "open-cluster-management.io/api/cluster/v1alpha1"
	clusterv1beta1 "open-cluster-management.io/api/cluster/v1beta1"
	policyv1 "open-cluster-management.io/governance-policy-propagator/api/v1"
	placementrulev1 "open-cluster-management.io/multicloud-operators-subscription/pkg/apis/apps/placementrule/v1"
	appsv1alpha1 "open-cluster-management.io/multicloud-operators-subscription/pkg/apis/apps/v1alpha1"
	ctrl "sigs.k8s.io/controller-runtime"
	"sigs.k8s.io/controller-runtime/pkg/cache"
	"sigs.k8s.io/controller-runtime/pkg/client"
	metricsserver "sigs.k8s.io/controller-runtime/pkg/metrics/server"

	"github.com/stolostron/multicluster-global-hub/agent/pkg/configs"
	"github.com/stolostron/multicluster-global-hub/agent/pkg/controllers"
	"github.com/stolostron/multicluster-global-hub/agent/pkg/status/syncers/configmap"
	"github.com/stolostron/multicluster-global-hub/pkg/constants"
	"github.com/stolostron/multicluster-global-hub/pkg/jobs"
	"github.com/stolostron/multicluster-global-hub/pkg/logger"
	commonobjects "github.com/stolostron/multicluster-global-hub/pkg/objects"
	"github.com/stolostron/multicluster-global-hub/pkg/transport"
	"github.com/stolostron/multicluster-global-hub/pkg/transport/controller"
	"github.com/stolostron/multicluster-global-hub/pkg/utils"
)

const (
	metricsHost                       = "0.0.0.0"
	metricsPort                 int32 = 8384
	leaderElectionLockID              = "multicluster-global-hub-agent-lock"
	hostingLeaderElectionLockID       = "multicluster-global-hub-agent-hosting-lock"
)

func main() {
	log := logger.DefaultZapLogger()
	defer func() { _ = log.Desugar().Sync() }() // ensure it's invoked only once within the main process
	utils.PrintRuntimeInfo()

	// adding and parsing flags should be done before the call of 'ctrl.GetConfigOrDie()',
	// otherwise kubeconfig will not be passed to agent main process
	agentConfig := parseFlags()

	restConfig := ctrl.GetConfigOrDie()
	restConfig.QPS = agentConfig.QPS
	restConfig.Burst = agentConfig.Burst

	if err := doMain(ctrl.SetupSignalHandler(), agentConfig, restConfig); err != nil {
		log.Fatalf("failed to run the agent: %v", err)
	}
}

func doMain(ctx context.Context, agentConfig *configs.AgentConfig, restConfig *rest.Config) error {
	c, err := client.New(restConfig, client.Options{Scheme: configs.GetRuntimeScheme()})
	if err != nil {
		return fmt.Errorf("failed to init the runtime client: %w", err)
	}

	if agentConfig.Terminating {
		if err := jobs.NewPruneFinalizer(ctx, c).Run(); err != nil {
			return fmt.Errorf("failed to prune the resources finalizer: %w", err)
		}
		return nil
	}

	// start the controller manager
	if err := completeConfig(ctx, c, agentConfig); err != nil {
		return fmt.Errorf("failed to complete configuration: %w", err)
	}
	if agentConfig.EnablePprof {
		go utils.StartDefaultPprofServer()
	}

	// init manager
	mgr, err := createManager(restConfig, agentConfig)
	if err != nil {
		return fmt.Errorf("failed to create manager: %w", err)
	}

	// add configmap controller
	if err := configmap.AddConfigMapController(mgr, agentConfig); err != nil {
		return fmt.Errorf("failed to add ConfigMap controller: %w", err)
	}

	transportSecretName := constants.GHTransportConfigSecret
	if agentConfig.TransportConfigSecretName != "" {
		transportSecretName = agentConfig.TransportConfigSecretName
	}
	// add transport ctrl to manager, also load the transportConfig(from secret) into the agentConfig
	transportCtrl := controller.NewTransportCtrl(
		agentConfig.PodNamespace,
		transportSecretName,
		transportCallback(mgr, agentConfig),
		agentConfig.TransportConfig,
		false,
	)

	if agentConfig.DeployMode == string(constants.StandaloneMode) {
		transportCtrl.DisableConsumer()
	}

	err = transportCtrl.SetupWithManager(mgr)
	if err != nil {
		return fmt.Errorf("failed to add transport to manager: %w", err)
	}

	if err := mgr.Start(ctx); err != nil {
		return fmt.Errorf("failed to start the controller manager: %w", err)
	}
	return nil
}

func parseFlags() *configs.AgentConfig {
	agentConfig := &configs.AgentConfig{
		ElectionConfig: &commonobjects.LeaderElectionConfig{},
		TransportConfig: &transport.TransportInternalConfig{
			// EnableDatabaseOffset affects only the manager, deciding if consumption starts from a database-stored offset
			EnableDatabaseOffset: false,
		},
	}

	pflag.CommandLine.AddGoFlagSet(flag.CommandLine)
	pflag.StringVar(&agentConfig.LeafHubName, "leaf-hub-name", "", "The name of the leaf hub.")
	pflag.StringVar(&agentConfig.TransportConfigSecretName, "transport-config-secret",
		"", "The name of the transport secret.")
	pflag.StringVar(&agentConfig.PodNamespace, "pod-namespace", constants.GHAgentNamespace,
		"The agent running namespace, also used as leader election namespace")
	pflag.IntVar(&agentConfig.SpecWorkPoolSize, "consumer-worker-pool-size", 10,
		"The goroutine number to propagate the bundles on managed cluster.")
	pflag.IntVar(&agentConfig.TransportConfig.FailureThreshold, "transport-failure-threshold", 10,
		"Restart the pod if the transport error count exceeds the transport-failure-threshold within 5 minutes.")
	pflag.BoolVar(&agentConfig.SpecEnforceHohRbac, "enforce-hoh-rbac", false,
		"enable hoh RBAC or not, default false")
	pflag.IntVar(&agentConfig.StatusDeltaCountSwitchFactor,
		"status-delta-count-switch-factor", 100,
		"default with 100.")
	pflag.IntVar(&agentConfig.ElectionConfig.LeaseDuration, "lease-duration", 137,
		"leader election lease duration")
	pflag.IntVar(&agentConfig.ElectionConfig.RenewDeadline, "renew-deadline", 107,
		"leader election renew deadline")
	pflag.IntVar(&agentConfig.ElectionConfig.RetryPeriod, "retry-period", 26,
		"leader election retry period")
	pflag.BoolVar(&agentConfig.Terminating, "terminating", false,
		"true is to trigger the PreStop hook to do cleanup. For example: removing finalizer")
	pflag.BoolVar(&agentConfig.EnableGlobalResource, "enable-global-resource", false,
		"Enable the global resource feature.")
	pflag.Float32Var(&agentConfig.QPS, "qps", 150,
		"QPS for the multicluster global hub agent")
	pflag.IntVar(&agentConfig.Burst, "burst", 300,
		"Burst for the multicluster global hub agent")
	pflag.BoolVar(&agentConfig.EnablePprof, "enable-pprof", false, "Enable the pprof tool.")
	pflag.StringVar(&agentConfig.DeployMode, "deploy-mode", "default",
		"The deploy mode for the agent. support values: default, local, standalone")
	pflag.BoolVar(&agentConfig.EnableStackroxIntegration, "enable-stackrox-integration", false,
		"Enable StackRox integration")
	pflag.DurationVar(&agentConfig.StackroxPollInterval, "stackrox-poll-interval", 30*time.Minute,
		"The interval between each StackRox polling")
	pflag.Parse()

	return agentConfig
}

func completeConfig(ctx context.Context, c client.Client, agentConfig *configs.AgentConfig) error {
	if agentConfig.LeafHubName == "" {
		if agentConfig.DeployMode != string(constants.StandaloneMode) {
			return fmt.Errorf("the leaf-hub-name must not be empty")
		}
		clusterID, err := utils.GetClusterIdFromClusterVersion(c, ctx)
		if err != nil {
			return err
		}
		agentConfig.LeafHubName = clusterID
	}

	if agentConfig.MetricsAddress == "" {
		agentConfig.MetricsAddress = fmt.Sprintf("%s:%d", metricsHost, metricsPort)
	}

	if agentConfig.SpecWorkPoolSize < 1 || agentConfig.SpecWorkPoolSize > 100 {
		return fmt.Errorf("flag consumer-worker-pool-size should be in the scope [1, 100]")
	}

	configs.SetAgentConfig(agentConfig)
	return nil
}

func createManager(restConfig *rest.Config, agentConfig *configs.AgentConfig) (
	ctrl.Manager, error,
) {
	leaseDuration := time.Duration(agentConfig.ElectionConfig.LeaseDuration) * time.Second
	renewDeadline := time.Duration(agentConfig.ElectionConfig.RenewDeadline) * time.Second
	retryPeriod := time.Duration(agentConfig.ElectionConfig.RetryPeriod) * time.Second

	var leaderElectionConfig *rest.Config
	if isAgentTesting, ok := os.LookupEnv("AGENT_TESTING"); ok && isAgentTesting == "true" {
		leaderElectionConfig = restConfig
	} else {
		var err error
		leaderElectionConfig, err = rest.InClusterConfig()
		if err != nil {
			return nil, err
		}
	}

	options := ctrl.Options{
		Metrics: metricsserver.Options{
			BindAddress: agentConfig.MetricsAddress,
		},
		LeaderElection:          true,
		Scheme:                  configs.GetRuntimeScheme(),
		LeaderElectionConfig:    leaderElectionConfig,
		LeaderElectionID:        leaderElectionLockID,
		LeaderElectionNamespace: agentConfig.PodNamespace,
		LeaseDuration:           &leaseDuration,
		RenewDeadline:           &renewDeadline,
		RetryPeriod:             &retryPeriod,
		NewCache:                initCache,
	}

	mgr, err := ctrl.NewManager(restConfig, options)
	if err != nil {
		return nil, fmt.Errorf("failed to create a new manager: %w", err)
	}
	return mgr, nil
}

// if the transport consumer and producer is ready then the func will be invoked by the transport controller
func transportCallback(mgr ctrl.Manager, agentConfig *configs.AgentConfig) controller.TransportCallback {
	return func(transportClient transport.TransportClient) error {
		if err := controllers.AddInitController(mgr, mgr.GetConfig(), agentConfig, transportClient); err != nil {
			return fmt.Errorf("failed to add crd controller: %w", err)
		}
		logger.DefaultZapLogger().Info("add the init controller to manager")
		return nil
	}
}

func initCache(restConfig *rest.Config, cacheOpts cache.Options) (cache.Cache, error) {
	cacheOpts.ByObject = map[client.Object]cache.ByObject{
		&apiextensionsv1.CustomResourceDefinition{}: {},
		&policyv1.Policy{}:                          {},
		&clusterv1.ManagedCluster{}:                 {},
		&clusterinfov1beta1.ManagedClusterInfo{}:    {},
		&routev1.Route{}:                            {},
		&placementrulev1.PlacementRule{}:            {},
		&clusterv1beta1.Placement{}:                 {},
		&clusterv1beta1.PlacementDecision{}:         {},
		&appsv1alpha1.SubscriptionReport{}:          {},
		&coordinationv1.Lease{}: {
			Field: fields.OneTermEqualSelector("metadata.namespace", configs.GetAgentConfig().PodNamespace),
		},
		&corev1.Event{}: {},
	}
	if configs.GetAgentConfig().DeployMode == string(constants.DefaultMode) {
		cacheOpts.ByObject[&clustersv1alpha1.ClusterClaim{}] = cache.ByObject{}
	}
	return cache.New(restConfig, cacheOpts)
}
