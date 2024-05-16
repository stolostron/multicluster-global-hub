package main

import (
	"context"
	"flag"
	"fmt"
	"os"
	"time"

	routev1 "github.com/openshift/api/route/v1"
	"github.com/spf13/pflag"
	coordinationv1 "k8s.io/api/coordination/v1"
	corev1 "k8s.io/api/core/v1"
	apiextensionsv1 "k8s.io/apiextensions-apiserver/pkg/apis/apiextensions/v1"
	"k8s.io/apimachinery/pkg/fields"
	"k8s.io/client-go/kubernetes"
	"k8s.io/client-go/kubernetes/scheme"
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

	"github.com/stolostron/multicluster-global-hub/agent/pkg/config"
	"github.com/stolostron/multicluster-global-hub/agent/pkg/controllers"
	agentscheme "github.com/stolostron/multicluster-global-hub/agent/pkg/scheme"
	"github.com/stolostron/multicluster-global-hub/pkg/constants"
	"github.com/stolostron/multicluster-global-hub/pkg/jobs"
	commonobjects "github.com/stolostron/multicluster-global-hub/pkg/objects"
	"github.com/stolostron/multicluster-global-hub/pkg/transport"
	"github.com/stolostron/multicluster-global-hub/pkg/transport/producer"
	"github.com/stolostron/multicluster-global-hub/pkg/utils"
)

const (
	metricsHost                = "0.0.0.0"
	metricsPort          int32 = 8384
	leaderElectionLockID       = "multicluster-global-hub-agent-lock"
)

var setupLog = ctrl.Log.WithName("setup")

func init() {
	agentscheme.AddToScheme(scheme.Scheme)
}

func main() {
	// adding and parsing flags should be done before the call of 'ctrl.GetConfigOrDie()',
	// otherwise kubeconfig will not be passed to agent main process
	agentConfig := parseFlags()
	utils.PrintVersion(setupLog)

	restConfig := ctrl.GetConfigOrDie()

	restConfig.QPS = agentConfig.QPS
	restConfig.Burst = agentConfig.Burst

	if agentConfig.Terminating {
		os.Exit(doTermination(ctrl.SetupSignalHandler(), restConfig))
	}

	os.Exit(doMain(ctrl.SetupSignalHandler(), restConfig, agentConfig))
}

func doTermination(ctx context.Context, restConfig *rest.Config) int {
	client, err := client.New(restConfig, client.Options{})
	if err != nil {
		setupLog.Error(err, "failed to int controller runtime client")
		return 1
	}
	if err := jobs.NewPruneFinalizer(ctx, client).Run(); err != nil {
		setupLog.Error(err, "failed to prune resources finalizer")
		return 1
	}
	return 0
}

// function to handle defers with exit, see https://stackoverflow.com/a/27629493/553720.
func doMain(ctx context.Context, restConfig *rest.Config, agentConfig *config.AgentConfig) int {
	if err := completeConfig(agentConfig); err != nil {
		setupLog.Error(err, "failed to get managed hub configuration from command line flags")
		return 1
	}

	if agentConfig.EnablePprof {
		go utils.StartDefaultPprofServer()
	}

	mgr, err := createManager(ctx, restConfig, agentConfig)
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

func parseFlags() *config.AgentConfig {
	agentConfig := &config.AgentConfig{
		ElectionConfig: &commonobjects.LeaderElectionConfig{},
		TransportConfig: &transport.TransportConfig{
			KafkaConfig: &transport.KafkaConfig{
				Topics:         &transport.ClusterTopic{},
				ProducerConfig: &transport.KafkaProducerConfig{},
				ConsumerConfig: &transport.KafkaConsumerConfig{},
			},
		},
	}

	// add flags for logger
	opts := utils.CtrlZapOptions()
	defaultFlags := flag.CommandLine
	opts.BindFlags(defaultFlags)
	pflag.CommandLine.AddGoFlagSet(defaultFlags)

	pflag.StringVar(&agentConfig.LeafHubName, "leaf-hub-name", "", "The name of the leaf hub.")
	pflag.StringVar(&agentConfig.TransportConfig.KafkaConfig.BootstrapServer, "kafka-bootstrap-server", "",
		"The bootstrap server for kafka.")
	pflag.StringVar(&agentConfig.TransportConfig.KafkaConfig.CaCertPath, "kafka-ca-cert-path", "",
		"The path of CA certificate for kafka bootstrap server.")
	pflag.StringVar(&agentConfig.TransportConfig.KafkaConfig.ClientCertPath, "kafka-client-cert-path", "",
		"The path of client certificate for kafka bootstrap server.")
	pflag.StringVar(&agentConfig.TransportConfig.KafkaConfig.ClientKeyPath, "kafka-client-key-path", "",
		"The path of client key for kafka bootstrap server.")
	pflag.StringVar(&agentConfig.TransportConfig.KafkaConfig.ProducerConfig.ProducerID, "kafka-producer-id", "",
		"Producer Id for the kafka, default is the leaf hub name.")
	pflag.StringVar(&agentConfig.TransportConfig.KafkaConfig.Topics.StatusTopic, "kafka-producer-topic",
		"status", "Topic for the kafka producer.")
	pflag.IntVar(&agentConfig.TransportConfig.KafkaConfig.ProducerConfig.MessageSizeLimitKB,
		"kafka-message-size-limit", 940, "The limit for kafka message size in KB.")
	pflag.StringVar(&agentConfig.TransportConfig.KafkaConfig.Topics.SpecTopic, "kafka-consumer-topic",
		"spec", "Topic for the kafka consumer.")
	pflag.StringVar(&agentConfig.TransportConfig.KafkaConfig.Topics.EventTopic, "kafka-event-topic",
		"event", "Topic for the kafka events.")
	pflag.StringVar(&agentConfig.TransportConfig.KafkaConfig.ConsumerConfig.ConsumerID, "kafka-consumer-id",
		"multicluster-global-hub-agent", "ID for the kafka consumer.")
	pflag.StringVar(&agentConfig.PodNameSpace, "pod-namespace", constants.GHAgentNamespace,
		"The agent running namespace, also used as leader election namespace")
	pflag.StringVar(&agentConfig.TransportConfig.TransportType, "transport-type", "kafka",
		"The transport type, 'kafka'")
	pflag.IntVar(&agentConfig.SpecWorkPoolSize, "consumer-worker-pool-size", 10,
		"The goroutine number to propagate the bundles on managed cluster.")
	pflag.BoolVar(&agentConfig.SpecEnforceHohRbac, "enforce-hoh-rbac", false,
		"enable hoh RBAC or not, default false")
	pflag.StringVar(&agentConfig.TransportConfig.MessageCompressionType,
		"transport-message-compression-type", "gzip",
		"The message compression type for transport layer, 'gzip' or 'no-op'.")
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
	pflag.Parse()

	// set zap logger
	ctrl.SetLogger(zap.New(zap.UseFlagOptions(&opts)))

	return agentConfig
}

func completeConfig(agentConfig *config.AgentConfig) error {
	if agentConfig.LeafHubName == "" {
		return fmt.Errorf("flag managed-hub-name can't be empty")
	}
	if agentConfig.TransportConfig.KafkaConfig.ProducerConfig.ProducerID == "" {
		agentConfig.TransportConfig.KafkaConfig.ProducerConfig.ProducerID = agentConfig.LeafHubName
	}
	if agentConfig.SpecWorkPoolSize < 1 ||
		agentConfig.SpecWorkPoolSize > 100 {
		return fmt.Errorf("flag consumer-worker-pool-size should be in the scope [1, 100]")
	}

	if agentConfig.TransportConfig.KafkaConfig.ProducerConfig.MessageSizeLimitKB > producer.MaxMessageKBLimit {
		return fmt.Errorf("flag kafka-message-size-limit %d must not exceed %d",
			agentConfig.TransportConfig.KafkaConfig.ProducerConfig.MessageSizeLimitKB, producer.MaxMessageKBLimit)
	}
	agentConfig.TransportConfig.KafkaConfig.EnableTLS = true
	if agentConfig.MetricsAddress == "" {
		agentConfig.MetricsAddress = fmt.Sprintf("%s:%d", metricsHost, metricsPort)
	}
	return nil
}

func createManager(ctx context.Context, restConfig *rest.Config, agentConfig *config.AgentConfig) (
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
		Scheme:                  scheme.Scheme,
		LeaderElectionConfig:    leaderElectionConfig,
		LeaderElectionID:        leaderElectionLockID,
		LeaderElectionNamespace: agentConfig.PodNameSpace,
		LeaseDuration:           &leaseDuration,
		RenewDeadline:           &renewDeadline,
		RetryPeriod:             &retryPeriod,
		NewCache:                initCache,
	}

	mgr, err := ctrl.NewManager(restConfig, options)
	if err != nil {
		return nil, fmt.Errorf("failed to create a new manager: %w", err)
	}
	kubeClient, err := kubernetes.NewForConfig(restConfig)
	if err != nil {
		return nil, fmt.Errorf("failed to create kubeclient: %w", err)
	}
	// Need this controller to update the value of clusterclaim hub.open-cluster-management.io
	// we use the value to decide whether install the ACM or not
	if err := controllers.AddHubClusterClaimController(mgr); err != nil {
		return nil, fmt.Errorf("failed to add hub.open-cluster-management.io clusterclaim controller: %w", err)
	}

	if err := controllers.AddCRDController(mgr, restConfig, agentConfig); err != nil {
		return nil, fmt.Errorf("failed to add crd controller: %w", err)
	}

	if err := controllers.AddCertController(mgr, kubeClient); err != nil {
		return nil, fmt.Errorf("failed to add crd controller: %w", err)
	}

	return mgr, nil
}

func initCache(config *rest.Config, cacheOpts cache.Options) (cache.Cache, error) {
	cacheOpts.ByObject = map[client.Object]cache.ByObject{
		&apiextensionsv1.CustomResourceDefinition{}: {
			Field: fields.OneTermEqualSelector("metadata.name", "clustermanagers.operator.open-cluster-management.io"),
		},
		&policyv1.Policy{}:                  {},
		&clusterv1.ManagedCluster{}:         {},
		&clustersv1alpha1.ClusterClaim{}:    {},
		&routev1.Route{}:                    {},
		&placementrulev1.PlacementRule{}:    {},
		&clusterv1beta1.Placement{}:         {},
		&clusterv1beta1.PlacementDecision{}: {},
		&appsv1alpha1.SubscriptionReport{}:  {},
		&coordinationv1.Lease{}: {
			Field: fields.OneTermEqualSelector("metadata.namespace", constants.GHAgentNamespace),
		},
		&corev1.Event{}: {}, // TODO: need a filter for the target events
		&corev1.Secret{}: {
			Field: fields.OneTermEqualSelector("metadata.namespace", constants.GHAgentNamespace),
		},
	}
	return cache.New(config, cacheOpts)
}
