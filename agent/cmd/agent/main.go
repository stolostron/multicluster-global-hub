package main

import (
	"context"
	"fmt"
	"os"
	"runtime"
	"strconv"
	"time"

	"github.com/go-logr/logr"
	"github.com/operator-framework/operator-sdk/pkg/log/zap"
	mchv1 "github.com/stolostron/multiclusterhub-operator/api/v1"
	v1 "k8s.io/api/core/v1"
	apiextensionsv1 "k8s.io/apiextensions-apiserver/pkg/apis/apiextensions/v1"
	apiErrors "k8s.io/apimachinery/pkg/api/errors"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	apiRuntime "k8s.io/apimachinery/pkg/runtime"
	_ "k8s.io/client-go/plugin/pkg/client/auth"
	"k8s.io/client-go/rest"
	clusterv1 "open-cluster-management.io/api/cluster/v1"
	clusterv1alpha1 "open-cluster-management.io/api/cluster/v1alpha1"
	clusterv1beta1 "open-cluster-management.io/api/cluster/v1beta1"
	policyv1 "open-cluster-management.io/governance-policy-propagator/api/v1"
	placementrulev1 "open-cluster-management.io/multicloud-operators-subscription/pkg/apis/apps/placementrule/v1"
	appsubv1 "open-cluster-management.io/multicloud-operators-subscription/pkg/apis/apps/v1"
	appsubv1alpha1 "open-cluster-management.io/multicloud-operators-subscription/pkg/apis/apps/v1alpha1"
	ctrl "sigs.k8s.io/controller-runtime"
	"sigs.k8s.io/controller-runtime/pkg/client"
	runtimescheme "sigs.k8s.io/controller-runtime/pkg/scheme"

	"github.com/stolostron/multicluster-global-hub/agent/pkg/controllers"
	"github.com/stolostron/multicluster-global-hub/agent/pkg/helper"
	"github.com/stolostron/multicluster-global-hub/agent/pkg/jobs"
	"github.com/stolostron/multicluster-global-hub/agent/pkg/lease"
	"github.com/stolostron/multicluster-global-hub/agent/pkg/spec/bundle"
	specController "github.com/stolostron/multicluster-global-hub/agent/pkg/spec/controller"
	statusController "github.com/stolostron/multicluster-global-hub/agent/pkg/status/controller"
	consumer "github.com/stolostron/multicluster-global-hub/agent/pkg/transport/consumer"
	producer "github.com/stolostron/multicluster-global-hub/agent/pkg/transport/producer"
	"github.com/stolostron/multicluster-global-hub/pkg/compressor"
)

const (
	METRICS_HOST               = "0.0.0.0"
	METRICS_PORT               = 9435
	TRANSPORT_TYPE_KAFKA       = "kafka"
	TRANSPORT_TYPE_SYNC_SVC    = "sync-service"
	LEADER_ELECTION_ID         = "multicluster-global-hub-agent-lock"
	HOH_LOCAL_NAMESPACE        = "open-cluster-management-global-hub-local"
	INCARNATION_CONFIG_MAP_KEY = "incarnation"
	BASE10                     = 10
	UINT64_SIZE                = 64
)

func main() {
	os.Exit(doMain())
}

// function to handle defers with exit, see https://stackoverflow.com/a/27629493/553720.
func doMain() int {
	log := initLog()
	printVersion(log)
	agentConfig, err := helper.NewConfigManager()
	if err != nil {
		log.Error(err, "failed to load environment variable")
		return 1
	}

	mgr, err := getControllerManager(ctrl.GetConfigOrDie(), agentConfig)
	if err != nil {
		log.Error(err, "failed the create controller manager")
		return 1
	}

	if agentConfig.Terminating {
		return jobs.NewPruneJob(mgr.GetClient()).Run()
	}

	// transport layer initialization
	genericBundleChan := make(chan *bundle.GenericBundle)
	defer close(genericBundleChan)

	consumer, err := getConsumer(agentConfig, genericBundleChan)
	if err != nil {
		log.Error(err, "transport consumer initialization error")
		return 1
	}
	producer, err := getProducer(agentConfig)
	if err != nil {
		log.Error(err, "transport producer initialization error")
	}

	consumer.Start()
	producer.Start()
	defer consumer.Stop()
	defer producer.Stop()

	if err = addControllerToManager(mgr, consumer, producer, agentConfig); err != nil {
		log.Error(err, "failed to create manager")
		return 1
	}

	log.Info("starting the Cmd")
	if err := mgr.Start(ctrl.SetupSignalHandler()); err != nil {
		log.Error(err, "manager exited non-zero")
		return 1
	}

	return 0
}

func initLog() logr.Logger {
	ctrl.SetLogger(zap.Logger())
	log := ctrl.Log.WithName("cmd")
	return log
}

func printVersion(log logr.Logger) {
	log.Info(fmt.Sprintf("Go Version: %s", runtime.Version()))
	log.Info(fmt.Sprintf("Go OS/Arch: %s/%s", runtime.GOOS, runtime.GOARCH))
}

// function to choose transport type based on env var.
func getConsumer(environmentManager *helper.ConfigManager,
	genericBundleChan chan *bundle.GenericBundle,
) (consumer.Consumer, error) {
	switch environmentManager.TransportType {
	case TRANSPORT_TYPE_KAFKA:
		kafkaConsumer, err := consumer.NewKafkaConsumer(ctrl.Log.WithName("kafka-consumer"),
			environmentManager, genericBundleChan)
		if err != nil {
			return nil, fmt.Errorf("failed to create kafka-consumer: %w", err)
		}
		return kafkaConsumer, nil
	case TRANSPORT_TYPE_SYNC_SVC:
		syncService, err := consumer.NewSyncService(ctrl.Log.WithName("sync-service"),
			environmentManager, genericBundleChan)
		if err != nil {
			return nil, fmt.Errorf("failed to create sync-service: %w", err)
		}
		return syncService, nil
	default:
		return nil, fmt.Errorf("environment variable %q - %q is not a valid option",
			"TRANSPORT_TYPE", environmentManager.TransportType)
	}
}

func getProducer(environmentManager *helper.ConfigManager) (producer.Producer, error) {
	messageCompressor, err := compressor.NewCompressor(
		compressor.CompressionType(environmentManager.TransportCompressionType))
	if err != nil {
		return nil, fmt.Errorf("failed to create transport producer message-compressor: %w", err)
	}

	switch environmentManager.TransportType {
	case TRANSPORT_TYPE_KAFKA:
		kafkaProducer, err := producer.NewKafkaProducer(messageCompressor,
			ctrl.Log.WithName("kafka-producer"), environmentManager)
		if err != nil {
			return nil, fmt.Errorf("failed to create kafka-producer: %w", err)
		}
		return kafkaProducer, nil
	case TRANSPORT_TYPE_SYNC_SVC:
		syncServiceProducer, err := producer.NewSyncServiceProducer(messageCompressor,
			ctrl.Log.WithName("syncservice-producer"), environmentManager)
		if err != nil {
			return nil, fmt.Errorf("failed to create sync-service producer: %w", err)
		}
		return syncServiceProducer, nil
	default:
		return nil, fmt.Errorf("environment variable %q - %q is not a valid option",
			"TRANSPORT_TYPE", environmentManager.TransportType)
	}
}

func getControllerManager(restConfig *rest.Config, agentConfig *helper.ConfigManager) (ctrl.Manager, error) {
	leaseDuration := time.Duration(agentConfig.ElectionConfig.LeaseDuration) * time.Second
	renewDeadline := time.Duration(agentConfig.ElectionConfig.RenewDeadline) * time.Second
	retryPeriod := time.Duration(agentConfig.ElectionConfig.RetryPeriod) * time.Second
	options := ctrl.Options{
		MetricsBindAddress:      fmt.Sprintf("%s:%d", METRICS_HOST, METRICS_PORT),
		LeaderElection:          true,
		LeaderElectionID:        LEADER_ELECTION_ID,
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
	if err := addToScheme(mgr.GetScheme()); err != nil {
		return nil, fmt.Errorf("failed to add a schemes: %w", err)
	}
	return mgr, nil
}

func addControllerToManager(mgr ctrl.Manager, consumer consumer.Consumer, producer producer.Producer,
	agentConfig *helper.ConfigManager,
) error {
	// incarnation version
	incarnation, err := getIncarnation(mgr)
	if err != nil {
		return fmt.Errorf("failed to get incarnation version: %w", err)
	}
	fmt.Printf("Starting the Cmd incarnation: %d", incarnation)

	if err := specController.AddSyncersToManager(mgr, consumer, *agentConfig); err != nil {
		return fmt.Errorf("failed to add spec syncer: %w", err)
	}

	if err := statusController.AddControllers(mgr, producer, *agentConfig, incarnation); err != nil {
		return fmt.Errorf("failed to add status syncer: %w", err)
	}

	if err := controllers.AddToManager(mgr); err != nil {
		return fmt.Errorf("failed to add controllers: %w", err)
	}

	if err := lease.AddHoHLeaseUpdater(mgr, agentConfig.PodNameSpace, "multicluster-global-hub-controller"); err != nil {
		return fmt.Errorf("failed to add lease updater: %w", err)
	}

	return nil
}

func addToScheme(runtimeScheme *apiRuntime.Scheme) error {
	// add cluster scheme
	if err := clusterv1.Install(runtimeScheme); err != nil {
		return fmt.Errorf("failed to add clusterv1 scheme: %w", err)
	}

	if err := clusterv1beta1.Install(runtimeScheme); err != nil {
		return fmt.Errorf("failed to add clusterv1beta1 scheme: %w", err)
	}

	if err := clusterv1alpha1.Install(runtimeScheme); err != nil {
		return fmt.Errorf("failed to add clustersv1alpha1 scheme: %w", err)
	}

	if err := apiextensionsv1.AddToScheme(runtimeScheme); err != nil {
		return fmt.Errorf("failed to add apiextensionsv1 scheme: %w", err)
	}

	schemeBuilders := []*runtimescheme.Builder{
		policyv1.SchemeBuilder, placementrulev1.SchemeBuilder, appsubv1alpha1.SchemeBuilder,
		mchv1.SchemeBuilder,
		appsubv1.SchemeBuilder,
	} // add schemes

	for _, schemeBuilder := range schemeBuilders {
		if err := schemeBuilder.AddToScheme(runtimeScheme); err != nil {
			return fmt.Errorf("failed to add scheme: %w", err)
		}
	}

	return nil
}

// Incarnation is a part of the version of all the messages this process will transport.
// The motivation behind this logic is allowing the message receivers/consumers to infer that messages transmitted
// from this instance are more recent than all other existing ones, regardless of their instance-specific generations.
func getIncarnation(mgr ctrl.Manager) (uint64, error) {
	k8sClient, err := client.New(mgr.GetConfig(), client.Options{Scheme: mgr.GetScheme()})
	if err != nil {
		return 0, fmt.Errorf("failed to start k8s client - %w", err)
	}

	ctx := context.Background()
	configMap := &v1.ConfigMap{}

	// create hoh-local ns if missing
	if err := helper.CreateNamespaceIfNotExist(ctx, k8sClient, HOH_LOCAL_NAMESPACE); err != nil {
		return 0, fmt.Errorf("failed to create ns - %w", err)
	}

	// try to get ConfigMap
	objKey := client.ObjectKey{
		Namespace: HOH_LOCAL_NAMESPACE,
		Name:      INCARNATION_CONFIG_MAP_KEY,
	}
	if err := k8sClient.Get(ctx, objKey, configMap); err != nil {
		if !apiErrors.IsNotFound(err) {
			return 0, fmt.Errorf("failed to get incarnation config-map - %w", err)
		}

		// incarnation ConfigMap does not exist, create it with incarnation = 0
		configMap = createIncarnationConfigMap(0)
		if err := k8sClient.Create(ctx, configMap); err != nil {
			return 0, fmt.Errorf("failed to create incarnation config-map obj - %w", err)
		}

		return 0, nil
	}

	// incarnation configMap exists, get incarnation, increment it and update object
	incarnationString, exists := configMap.Data[INCARNATION_CONFIG_MAP_KEY]
	if !exists {
		return 0, fmt.Errorf("configmap %s does not contain (%s)",
			INCARNATION_CONFIG_MAP_KEY, INCARNATION_CONFIG_MAP_KEY)
	}

	lastIncarnation, err := strconv.ParseUint(incarnationString, BASE10, UINT64_SIZE)
	if err != nil {
		return 0, fmt.Errorf("failed to parse value of key %s in configmap %s - %w", INCARNATION_CONFIG_MAP_KEY,
			INCARNATION_CONFIG_MAP_KEY, err)
	}

	newConfigMap := createIncarnationConfigMap(lastIncarnation + 1)
	if err := k8sClient.Patch(ctx, newConfigMap, client.MergeFrom(configMap)); err != nil {
		return 0, fmt.Errorf("failed to update incarnation version - %w", err)
	}
	return lastIncarnation + 1, nil
}

func createIncarnationConfigMap(incarnation uint64) *v1.ConfigMap {
	return &v1.ConfigMap{
		ObjectMeta: metav1.ObjectMeta{
			Namespace: HOH_LOCAL_NAMESPACE,
			Name:      INCARNATION_CONFIG_MAP_KEY,
		},
		Data: map[string]string{INCARNATION_CONFIG_MAP_KEY: strconv.FormatUint(incarnation, BASE10)},
	}
}
