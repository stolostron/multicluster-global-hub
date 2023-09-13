package main

import (
	"context"
	"flag"
	"fmt"
	"os"
	"time"

	"github.com/spf13/pflag"
	mchv1 "github.com/stolostron/multiclusterhub-operator/api/v1"
	"k8s.io/apiextensions-apiserver/pkg/client/clientset/clientset/scheme"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/client-go/dynamic"
	_ "k8s.io/client-go/plugin/pkg/client/auth"
	"k8s.io/client-go/rest"
	operatorv1 "open-cluster-management.io/api/operator/v1"
	ctrl "sigs.k8s.io/controller-runtime"
	"sigs.k8s.io/controller-runtime/pkg/client"
	"sigs.k8s.io/controller-runtime/pkg/log/zap"

	"github.com/stolostron/multicluster-global-hub/agent/pkg/config"
	"github.com/stolostron/multicluster-global-hub/agent/pkg/controllers"
	"github.com/stolostron/multicluster-global-hub/agent/pkg/event"
	"github.com/stolostron/multicluster-global-hub/agent/pkg/lease"
	agentscheme "github.com/stolostron/multicluster-global-hub/agent/pkg/scheme"
	specController "github.com/stolostron/multicluster-global-hub/agent/pkg/spec/controller"
	statusController "github.com/stolostron/multicluster-global-hub/agent/pkg/status/controller"
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

func main() {
	// adding and parsing flags should be done before the call of 'ctrl.GetConfigOrDie()',
	// otherwise kubeconfig will not be passed to agent main process
	agentConfig := parseFlags()
	utils.PrintVersion(setupLog)
	if agentConfig.Terminating {
		os.Exit(doTermination(ctrl.SetupSignalHandler(), ctrl.GetConfigOrDie()))
	}
	os.Exit(doMain(ctrl.SetupSignalHandler(), ctrl.GetConfigOrDie(), agentConfig))
}

func doTermination(ctx context.Context, restConfig *rest.Config) int {
	if err := agentscheme.AddToScheme(scheme.Scheme); err != nil {
		setupLog.Error(err, "failed to add to scheme")
		return 1
	}
	client, err := client.New(restConfig, client.Options{Scheme: scheme.Scheme})
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
	pflag.StringVar(&agentConfig.TransportConfig.KafkaConfig.ProducerConfig.ProducerTopic, "kafka-producer-topic",
		"status", "Topic for the kafka producer.")
	pflag.IntVar(&agentConfig.TransportConfig.KafkaConfig.ProducerConfig.MessageSizeLimitKB,
		"kafka-message-size-limit", 940, "The limit for kafka message size in KB.")
	pflag.StringVar(&agentConfig.TransportConfig.KafkaConfig.ConsumerConfig.ConsumerTopic, "kafka-consumer-topic",
		"spec", "Topic for the kafka consumer.")
	pflag.StringVar(&agentConfig.TransportConfig.KafkaConfig.ConsumerConfig.ConsumerID, "kafka-consumer-id",
		"multicluster-global-hub-agent", "ID for the kafka consumer.")
	pflag.StringVar(&agentConfig.PodNameSpace, "pod-namespace", constants.GHAgentNamespace,
		"The agent running namespace, also used as leader election namespace")
	pflag.StringVar(&agentConfig.TransportConfig.TransportType, "transport-type", "kafka",
		"The transport type, 'kafka'")
	pflag.StringVar(&agentConfig.TransportConfig.TransportFormat, "transport-format", "cloudEvents",
		"The transport format, default is 'cloudEvents'.")
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
	pflag.StringVar(&agentConfig.KubeEventExporterConfigPath,
		"kubernetes-event-exporter-config", "",
		"The configuration file for the kubernetes event exporter")
	pflag.BoolVar(&agentConfig.EnableGlobalResource, "enable-global-resource", false,
		"Enable the global resource feature.")

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

	if agentConfig.TransportConfig.KafkaConfig.ProducerConfig.MessageSizeLimitKB > producer.MaxMessageSizeLimit {
		return fmt.Errorf("flag kafka-message-size-limit %d must not exceed %d",
			agentConfig.TransportConfig.KafkaConfig.ProducerConfig.MessageSizeLimitKB, producer.MaxMessageSizeLimit)
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
		MetricsBindAddress:      agentConfig.MetricsAddress,
		LeaderElection:          true,
		LeaderElectionConfig:    leaderElectionConfig,
		LeaderElectionID:        leaderElectionLockID,
		LeaderElectionNamespace: agentConfig.PodNameSpace,
		LeaseDuration:           &leaseDuration,
		RenewDeadline:           &renewDeadline,
		RetryPeriod:             &retryPeriod,
	}

	mgr, err := ctrl.NewManager(restConfig, options)
	if err != nil {
		return nil, fmt.Errorf("failed to create a new manager: %w", err)
	}

	// add scheme
	if err := agentscheme.AddToScheme(mgr.GetScheme()); err != nil {
		return nil, fmt.Errorf("failed to add schemes: %w", err)
	}

	// generate the client based on the config
	dynamicClient, err := dynamic.NewForConfig(restConfig)
	if err != nil {
		return nil, fmt.Errorf("failed to create dynamic client %v", err)
	}
	// check the acm is installed and then add the following controllers to mgr
	mchExists, err := ensureMulticlusterHub(ctx, dynamicClient)
	if err != nil {
		setupLog.Error(err, "You need install ACM before using the agent")
	}

	var clusterManagerExists bool
	if !mchExists {
		// we are using ocm for e2e test
		clusterManagerExists, err = ensureClusterManager(ctx, dynamicClient)
		if err != nil {
			setupLog.Error(err, "You need install OCM before using the agent")
		}
	}

	if mchExists || clusterManagerExists {
		// add spec controllers
		if agentConfig.EnableGlobalResource {
			if err := specController.AddToManager(mgr, agentConfig); err != nil {
				return nil, fmt.Errorf("failed to add spec syncer: %w", err)
			}
			setupLog.Info("add spec controllers to manager")
		}

		if err := statusController.AddControllers(ctx, mgr, agentConfig); err != nil {
			return nil, fmt.Errorf("failed to add status syncer: %w", err)
		}

		if err := lease.AddHoHLeaseUpdater(mgr, agentConfig.PodNameSpace,
			"multicluster-global-hub-controller"); err != nil {
			return nil, fmt.Errorf("failed to add lease updater: %w", err)
		}
	}

	// Need this controller to update the value of clusterclaim hub.open-cluster-management.io
	// we use the value to decide whether install the ACM or not
	if err := controllers.AddClusterClaimController(mgr); err != nil {
		return nil, fmt.Errorf("failed to add controllers: %w", err)
	}

	if err := event.AddEventExporter(mgr, agentConfig.KubeEventExporterConfigPath,
		agentConfig.LeafHubName); err != nil {
		return nil, fmt.Errorf("failed to add event exporter: %w", err)
	}

	return mgr, nil
}

func ensureMulticlusterHub(ctx context.Context, dynamicClient dynamic.Interface) (bool, error) {
	mch, err := dynamicClient.Resource(mchv1.GroupVersion.WithResource("multiclusterhubs")).
		Namespace("").
		List(ctx, metav1.ListOptions{})
	if err != nil {
		return false, err
	}
	return len(mch.Items) > 0, nil
}

func ensureClusterManager(ctx context.Context, dynamicClient dynamic.Interface) (
	bool, error,
) {
	clusterManager, err := dynamicClient.Resource(
		operatorv1.GroupVersion.WithResource("clustermanagers")).
		List(ctx, metav1.ListOptions{})
	if err != nil {
		return false, err
	}
	return len(clusterManager.Items) > 0, nil
}
