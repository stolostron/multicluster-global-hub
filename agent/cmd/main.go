package main

import (
	"context"
	"flag"
	"fmt"
	"os"
	"time"

	configv1 "github.com/openshift/api/config/v1"
	routev1 "github.com/openshift/api/route/v1"
	"github.com/spf13/pflag"
	clusterinfov1beta1 "github.com/stolostron/cluster-lifecycle-api/clusterinfo/v1beta1"
	coordinationv1 "k8s.io/api/coordination/v1"
	corev1 "k8s.io/api/core/v1"
	apiextensionsv1 "k8s.io/apiextensions-apiserver/pkg/apis/apiextensions/v1"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
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
	"sigs.k8s.io/controller-runtime/pkg/log/zap"
	metricsserver "sigs.k8s.io/controller-runtime/pkg/metrics/server"

	"github.com/stolostron/multicluster-global-hub/agent/pkg/configs"
	"github.com/stolostron/multicluster-global-hub/agent/pkg/controllers"
	"github.com/stolostron/multicluster-global-hub/pkg/constants"
	"github.com/stolostron/multicluster-global-hub/pkg/jobs"
	commonobjects "github.com/stolostron/multicluster-global-hub/pkg/objects"
	"github.com/stolostron/multicluster-global-hub/pkg/transport"
	"github.com/stolostron/multicluster-global-hub/pkg/transport/controller"
	"github.com/stolostron/multicluster-global-hub/pkg/utils"
)

const (
	metricsHost                = "0.0.0.0"
	metricsPort          int32 = 8384
	leaderElectionLockID       = "multicluster-global-hub-agent-lock"
)

var setupLog = ctrl.Log.WithName("setup")

func main() {
	// adding and parsing flags should be done before the call of 'ctrl.GetConfigOrDie()',
	// otherwise kubeconfig will not be passed to agent main process
	agentConfig := parseFlags()
	utils.PrintVersion(setupLog)

	restConfig := ctrl.GetConfigOrDie()

	restConfig.QPS = agentConfig.QPS
	restConfig.Burst = agentConfig.Burst

	c, err := client.New(restConfig, client.Options{Scheme: configs.GetRuntimeScheme()})
	if err != nil {
		setupLog.Error(err, "failed to int controller runtime client")
		os.Exit(1)
	}

	if agentConfig.Terminating {
		os.Exit(doTermination(ctrl.SetupSignalHandler(), c))
	}

	os.Exit(doMain(ctrl.SetupSignalHandler(), restConfig, agentConfig, c))
}

func doTermination(ctx context.Context, c client.Client) int {
	if err := jobs.NewPruneFinalizer(ctx, c).Run(); err != nil {
		setupLog.Error(err, "failed to prune resources finalizer")
		return 1
	}
	return 0
}

// function to handle defers with exit, see https://stackoverflow.com/a/27629493/553720.
func doMain(ctx context.Context, restConfig *rest.Config, agentConfig *configs.AgentConfig, c client.Client) int {
	if err := completeConfig(ctx, c, agentConfig); err != nil {
		setupLog.Error(err, "failed to get managed hub configuration from command line flags")
		return 1
	}

	if agentConfig.EnablePprof {
		go utils.StartDefaultPprofServer()
	}

	mgr, err := createManager(restConfig, agentConfig)
	if err != nil {
		setupLog.Error(err, "failed to create manager")
		return 1
	}

	setupLog.Info("starting the agent controller manager")
	if err := mgr.Start(ctx); err != nil {
		setupLog.Error(err, "manager exited non-zero")
		return 1
	}
	return 0
}

func parseFlags() *configs.AgentConfig {
	agentConfig := &configs.AgentConfig{
		ElectionConfig: &commonobjects.LeaderElectionConfig{},
		TransportConfig: &transport.TransportInternalConfig{
			// IsManager specifies the send/receive topics from specTopic and statusTopic
			// For example, SpecTopic sends and statusTopic receives on the manager; the agent is the opposite
			IsManager: false,
			// EnableDatabaseOffset affects only the manager, deciding if consumption starts from a database-stored offset
			EnableDatabaseOffset: false,
		},
	}

	// add flags for logger
	opts := utils.CtrlZapOptions()
	defaultFlags := flag.CommandLine
	opts.BindFlags(defaultFlags)
	pflag.CommandLine.AddGoFlagSet(defaultFlags)

	pflag.StringVar(&agentConfig.LeafHubName, "leaf-hub-name", "", "The name of the leaf hub.")
	pflag.StringVar(&agentConfig.PodNamespace, "pod-namespace", constants.GHAgentNamespace,
		"The agent running namespace, also used as leader election namespace")
	pflag.IntVar(&agentConfig.SpecWorkPoolSize, "consumer-worker-pool-size", 10,
		"The goroutine number to propagate the bundles on managed cluster.")
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
	pflag.BoolVar(&agentConfig.Standalone, "standalone", false, "Whether to deploy the agent with standalone mode")
	pflag.BoolVar(&agentConfig.EnableStackroxIntegration, "enable-stackrox-integration", false,
		"Enable StackRox integration")
	pflag.DurationVar(&agentConfig.StackroxPollInterval, "stackrox-poll-interval", 30*time.Minute,
		"The interval between each StackRox polling")
	pflag.Parse()

	// set zap logger
	ctrl.SetLogger(zap.New(zap.UseFlagOptions(&opts)))

	return agentConfig
}

func completeConfig(ctx context.Context, c client.Client, agentConfig *configs.AgentConfig) error {
	if !agentConfig.Standalone && agentConfig.LeafHubName == "" {
		return fmt.Errorf("the leaf-hub-name must not be empty")
	}
	if agentConfig.LeafHubName == "" {
		clusterVersion := &configv1.ClusterVersion{
			ObjectMeta: metav1.ObjectMeta{Name: "version"},
		}
		err := c.Get(ctx, client.ObjectKeyFromObject(clusterVersion), clusterVersion)
		if err != nil {
			return fmt.Errorf("failed to get the ClusterVersion(version): %w", err)
		}

		clusterID := string(clusterVersion.Spec.ClusterID)
		if clusterID == "" {
			return fmt.Errorf("the clusterId from ClusterVersion must not be empty")
		}
		agentConfig.LeafHubName = clusterID
	}

	if agentConfig.MetricsAddress == "" {
		agentConfig.MetricsAddress = fmt.Sprintf("%s:%d", metricsHost, metricsPort)
	}
	// if deploy the agent as a event exporter, then disable the consumer features
	if agentConfig.Standalone {
		agentConfig.TransportConfig.ConsumerGroupId = ""
		agentConfig.SpecWorkPoolSize = 0
	} else {
		agentConfig.TransportConfig.ConsumerGroupId = agentConfig.LeafHubName
		if agentConfig.SpecWorkPoolSize < 1 || agentConfig.SpecWorkPoolSize > 100 {
			return fmt.Errorf("flag consumer-worker-pool-size should be in the scope [1, 100]")
		}
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
	// Need this controller to update the value of clusterclaim hub.open-cluster-management.io

	err = controller.NewTransportCtrl(
		agentConfig.PodNamespace,
		constants.GHTransportConfigSecret,
		transportCallback(mgr, agentConfig),
		agentConfig.TransportConfig,
	).SetupWithManager(mgr)
	if err != nil {
		return nil, err
	}
	setupLog.Info("add the transport controller to agent")
	return mgr, nil
}

// if the transport consumer and producer is ready then the func will be invoked by the transport controller
func transportCallback(mgr ctrl.Manager, agentConfig *configs.AgentConfig,
) controller.TransportCallback {
	return func(transportClient transport.TransportClient) error {
		if err := controllers.AddInitController(mgr, mgr.GetConfig(), agentConfig, transportClient); err != nil {
			return fmt.Errorf("failed to add crd controller: %w", err)
		}

		setupLog.Info("add the agent controllers to manager")
		return nil
	}
}

func initCache(restConfig *rest.Config, cacheOpts cache.Options) (cache.Cache, error) {
	cacheOpts.ByObject = map[client.Object]cache.ByObject{
		&apiextensionsv1.CustomResourceDefinition{}: {},
		&policyv1.Policy{}:                          {},
		&clusterv1.ManagedCluster{}:                 {},
		&clusterinfov1beta1.ManagedClusterInfo{}:    {},
		&clustersv1alpha1.ClusterClaim{}:            {},
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
	return cache.New(restConfig, cacheOpts)
}
