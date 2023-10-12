// Copyright (c) 2020 Red Hat, Inc.
// Copyright Contributors to the Open Cluster Management project

package main

import (
	"context"
	"crypto/tls"
	"errors"
	"flag"
	"fmt"
	"os"
	"strings"
	"time"

	"github.com/mohae/deepcopy"
	"github.com/spf13/pflag"
	corev1 "k8s.io/api/core/v1"
	"k8s.io/apimachinery/pkg/fields"
	"k8s.io/apimachinery/pkg/runtime"
	_ "k8s.io/client-go/plugin/pkg/client/auth"
	"k8s.io/client-go/rest"
	clusterv1 "open-cluster-management.io/api/cluster/v1"
	clusterv1beta1 "open-cluster-management.io/api/cluster/v1beta1"
	clusterv1beta2 "open-cluster-management.io/api/cluster/v1beta2"
	policyv1 "open-cluster-management.io/governance-policy-propagator/api/v1"
	channelv1 "open-cluster-management.io/multicloud-operators-channel/pkg/apis/apps/v1"
	placementrulev1 "open-cluster-management.io/multicloud-operators-subscription/pkg/apis/apps/placementrule/v1"
	subscriptionv1 "open-cluster-management.io/multicloud-operators-subscription/pkg/apis/apps/v1"
	applicationv1beta1 "sigs.k8s.io/application/api/v1beta1"
	ctrl "sigs.k8s.io/controller-runtime"
	"sigs.k8s.io/controller-runtime/pkg/cache"
	"sigs.k8s.io/controller-runtime/pkg/client"
	"sigs.k8s.io/controller-runtime/pkg/log/zap"
	"sigs.k8s.io/controller-runtime/pkg/webhook"

	managerconfig "github.com/stolostron/multicluster-global-hub/manager/pkg/config"
	"github.com/stolostron/multicluster-global-hub/manager/pkg/cronjob"
	"github.com/stolostron/multicluster-global-hub/manager/pkg/eventcollector"
	globalhubmetrics "github.com/stolostron/multicluster-global-hub/manager/pkg/metrics"
	"github.com/stolostron/multicluster-global-hub/manager/pkg/nonk8sapi"
	managerscheme "github.com/stolostron/multicluster-global-hub/manager/pkg/scheme"
	"github.com/stolostron/multicluster-global-hub/manager/pkg/specsyncer"
	"github.com/stolostron/multicluster-global-hub/manager/pkg/specsyncer/db2transport/db/postgresql"
	statussyncer "github.com/stolostron/multicluster-global-hub/manager/pkg/statussyncer"
	mgrwebhook "github.com/stolostron/multicluster-global-hub/manager/pkg/webhook"
	"github.com/stolostron/multicluster-global-hub/pkg/constants"
	"github.com/stolostron/multicluster-global-hub/pkg/database"
	commonobjects "github.com/stolostron/multicluster-global-hub/pkg/objects"
	"github.com/stolostron/multicluster-global-hub/pkg/statistics"
	"github.com/stolostron/multicluster-global-hub/pkg/transport"
	"github.com/stolostron/multicluster-global-hub/pkg/transport/producer"
	"github.com/stolostron/multicluster-global-hub/pkg/utils"
)

const (
	metricsHost                = "0.0.0.0"
	metricsPort          int32 = 8384
	webhookPort                = 9443
	webhookCertDir             = "/webhook-certs"
	kafkaTransportType         = "kafka"
	leaderElectionLockID       = "multicluster-global-hub-manager-lock"
	launchJobNamesEnv          = "LAUNCH_JOB_NAMES"
)

var (
	setupLog                     = ctrl.Log.WithName("setup")
	managerNamespace             = constants.GHDefaultNamespace
	scheme                       = runtime.NewScheme()
	enableSimulation             = false
	errFlagParameterEmpty        = errors.New("flag parameter empty")
	errFlagParameterIllegalValue = errors.New("flag parameter illegal value")
)

func init() {
	managerscheme.AddToScheme(scheme)
}

func parseFlags() *managerconfig.ManagerConfig {
	managerConfig := &managerconfig.ManagerConfig{
		SyncerConfig:   &managerconfig.SyncerConfig{},
		DatabaseConfig: &managerconfig.DatabaseConfig{},
		TransportConfig: &transport.TransportConfig{
			KafkaConfig: &transport.KafkaConfig{
				EnableTLS:      true,
				ProducerConfig: &transport.KafkaProducerConfig{},
				ConsumerConfig: &transport.KafkaConsumerConfig{},
			},
		},
		StatisticsConfig:      &statistics.StatisticsConfig{},
		NonK8sAPIServerConfig: &nonk8sapi.NonK8sAPIServerConfig{},
		ElectionConfig:        &commonobjects.LeaderElectionConfig{},
		LaunchJobNames:        "",
	}

	// add zap flags
	opts := utils.CtrlZapOptions()
	defaultFlags := flag.CommandLine
	opts.BindFlags(defaultFlags)
	pflag.CommandLine.AddGoFlagSet(defaultFlags)

	pflag.StringVar(&managerConfig.ManagerNamespace, "manager-namespace", constants.GHDefaultNamespace,
		"The manager running namespace, also used as leader election namespace.")
	pflag.StringVar(&managerConfig.WatchNamespace, "watch-namespace", "",
		"The watching namespace of the controllers, multiple namespace must be splited by comma.")
	pflag.StringVar(&managerConfig.SchedulerInterval, "scheduler-interval", "day",
		"The job scheduler interval for moving policy compliance history, "+
			"can be 'month', 'week', 'day', 'hour', 'minute' or 'second', default value is 'day'.")
	pflag.DurationVar(&managerConfig.SyncerConfig.SpecSyncInterval, "spec-sync-interval", 5*time.Second,
		"The synchronization interval of resources in spec.")
	pflag.DurationVar(&managerConfig.SyncerConfig.StatusSyncInterval, "status-sync-interval", 5*time.Second,
		"The synchronization interval of resources in status.")
	pflag.DurationVar(&managerConfig.SyncerConfig.DeletedLabelsTrimmingInterval, "deleted-labels-trimming-interval",
		5*time.Second, "The trimming interval of deleted labels.")
	pflag.IntVar(&managerConfig.DatabaseConfig.MaxOpenConns, "database-pool-size", 10,
		"The size of database connection pool for the process user.")
	pflag.StringVar(&managerConfig.DatabaseConfig.ProcessDatabaseURL, "process-database-url", "",
		"The URL of database server for the process user.")
	pflag.StringVar(&managerConfig.DatabaseConfig.TransportBridgeDatabaseURL,
		"transport-bridge-database-url", "", "The URL of database server for the transport-bridge user.")
	pflag.StringVar(&managerConfig.TransportConfig.TransportType, "transport-type", "kafka",
		"The transport type, 'kafka'.")
	pflag.StringVar(&managerConfig.TransportConfig.TransportFormat, "transport-format", "cloudEvents",
		"The transport format, default is 'cloudEvents'.")
	pflag.StringVar(&managerConfig.TransportConfig.MessageCompressionType, "transport-message-compression-type",
		"gzip", "The message compression type for transport layer, 'gzip' or 'no-op'.")
	pflag.DurationVar(&managerConfig.TransportConfig.CommitterInterval, "transport-committer-interval",
		40*time.Second, "The committer interval for transport layer.")
	pflag.StringVar(&managerConfig.TransportConfig.KafkaConfig.BootstrapServer, "kafka-bootstrap-server",
		"kafka-kafka-bootstrap.kafka.svc:9092", "The bootstrap server for kafka.")
	pflag.StringVar(&managerConfig.TransportConfig.KafkaConfig.CaCertPath, "kafka-ca-cert-path", "",
		"The path of CA certificate for kafka bootstrap server.")
	pflag.StringVar(&managerConfig.TransportConfig.KafkaConfig.ClientCertPath, "kafka-client-cert-path", "",
		"The path of client certificate for kafka bootstrap server.")
	pflag.StringVar(&managerConfig.TransportConfig.KafkaConfig.ClientKeyPath, "kafka-client-key-path", "",
		"The path of client key for kafka bootstrap server.")
	pflag.StringVar(&managerConfig.DatabaseConfig.CACertPath, "postgres-ca-path", "/postgres-ca/ca.crt",
		"The path of CA certificate for kafka bootstrap server.")
	pflag.StringVar(&managerConfig.TransportConfig.KafkaConfig.ProducerConfig.ProducerID, "kafka-producer-id",
		"multicluster-global-hub-manager", "ID for the kafka producer.")
	pflag.StringVar(&managerConfig.TransportConfig.KafkaConfig.ProducerConfig.ProducerTopic, "kafka-producer-topic",
		"spec", "Topic for the kafka producer.")
	pflag.StringVar(&managerConfig.EventExporterTopic, "event-exporter-topic", "event", "Topic for the event exporter.")
	pflag.IntVar(&managerConfig.TransportConfig.KafkaConfig.ProducerConfig.MessageSizeLimitKB,
		"kafka-message-size-limit", 940, "The limit for kafka message size in KB.")
	pflag.StringVar(&managerConfig.TransportConfig.KafkaConfig.ConsumerConfig.ConsumerID,
		"kafka-consumer-id", "multicluster-global-hub-manager", "ID for the kafka consumer.")
	pflag.StringVar(&managerConfig.TransportConfig.KafkaConfig.ConsumerConfig.ConsumerTopic,
		"kafka-consumer-topic", "status", "Topic for the kafka consumer.")
	pflag.StringVar(&managerConfig.StatisticsConfig.LogInterval, "statistics-log-interval", "1m",
		"The log interval for statistics.")
	pflag.StringVar(&managerConfig.NonK8sAPIServerConfig.ClusterAPIURL, "cluster-api-url",
		"https://kubernetes.default.svc:443", "The cluster API URL for nonK8s API server.")
	pflag.StringVar(&managerConfig.NonK8sAPIServerConfig.ClusterAPICABundlePath, "cluster-api-cabundle-path",
		"/var/run/secrets/kubernetes.io/serviceaccount/ca.crt", "The CA bundle path for cluster API.")
	pflag.StringVar(&managerConfig.NonK8sAPIServerConfig.ServerBasePath, "server-base-path",
		"/global-hub-api/v1", "The base path for nonK8s API server.")
	pflag.IntVar(&managerConfig.ElectionConfig.LeaseDuration, "lease-duration", 137, "controller leader lease duration")
	pflag.IntVar(&managerConfig.ElectionConfig.RenewDeadline, "renew-deadline", 107, "controller leader renew deadline")
	pflag.IntVar(&managerConfig.ElectionConfig.RetryPeriod, "retry-period", 26, "controller leader retry period")
	pflag.IntVar(&managerConfig.DatabaseConfig.DataRetention, "data-retention", 18,
		"data retention indicates how many months the expired data will kept in the database")
	pflag.BoolVar(&managerConfig.EnableGlobalResource, "enable-global-resource", false,
		"enable the global resource feature.")

	pflag.Parse()
	// set zap logger
	ctrl.SetLogger(zap.New(zap.UseFlagOptions(&opts)))

	pflag.Visit(func(f *pflag.Flag) {
		// set enableSimulation to be true when manually set 'scheduler-interval' flag
		if f.Name == "scheduler-interval" && f.Changed {
			enableSimulation = true
		}
	})
	managerNamespace = managerConfig.ManagerNamespace
	return managerConfig
}

func completeConfig(managerConfig *managerconfig.ManagerConfig) error {
	if managerConfig.DatabaseConfig.ProcessDatabaseURL == "" {
		return fmt.Errorf("database url for process user: %w", errFlagParameterEmpty)
	}
	if managerConfig.TransportConfig.KafkaConfig.ProducerConfig.MessageSizeLimitKB > producer.MaxMessageSizeLimit {
		return fmt.Errorf("%w - size must not exceed %d : %s", errFlagParameterIllegalValue,
			managerConfig.TransportConfig.KafkaConfig.ProducerConfig.MessageSizeLimitKB, "kafka-message-size-limit")
	}
	// the specified jobs(concatenate multiple jobs with ',') runs when the container starts
	val, ok := os.LookupEnv(launchJobNamesEnv)
	if ok && val != "" {
		managerConfig.LaunchJobNames = val
	}
	return nil
}

func createManager(ctx context.Context, restConfig *rest.Config, managerConfig *managerconfig.ManagerConfig,
	processPostgreSQL *postgresql.PostgreSQL,
) (ctrl.Manager, error) {
	leaseDuration := time.Duration(managerConfig.ElectionConfig.LeaseDuration) * time.Second
	renewDeadline := time.Duration(managerConfig.ElectionConfig.RenewDeadline) * time.Second
	retryPeriod := time.Duration(managerConfig.ElectionConfig.RetryPeriod) * time.Second
	options := ctrl.Options{
		Namespace:               managerConfig.WatchNamespace,
		Scheme:                  scheme,
		MetricsBindAddress:      fmt.Sprintf("%s:%d", metricsHost, metricsPort),
		LeaderElection:          true,
		LeaderElectionNamespace: managerConfig.ManagerNamespace,
		LeaderElectionID:        leaderElectionLockID,
		LeaseDuration:           &leaseDuration,
		RenewDeadline:           &renewDeadline,
		RetryPeriod:             &retryPeriod,
		NewCache:                initCache,
	}

	if managerConfig.EnableGlobalResource {
		options.WebhookServer = &webhook.DefaultServer{
			Options: webhook.Options{
				Port:    webhookPort,
				CertDir: webhookCertDir,
				TLSOpts: []func(*tls.Config){
					func(config *tls.Config) {
						config.MinVersion = tls.VersionTLS12
					},
				},
			},
		}
	}

	// Add support for MultiNamespace set in WATCH_NAMESPACE (e.g ns1,ns2)
	// Note that this is not intended to be used for excluding namespaces, this is better done via a Predicate
	// Also note that you may face performance issues when using this with a high number of namespaces.
	// More Info: https://godoc.org/github.com/kubernetes-sigs/controller-runtime/pkg/cache#MultiNamespacedCacheBuilder
	if strings.Contains(managerConfig.WatchNamespace, ",") {
		options.Cache.Namespaces = strings.Split(managerConfig.WatchNamespace, ",")
	}

	mgr, err := ctrl.NewManager(restConfig, options)
	if err != nil {
		return nil, fmt.Errorf("failed to create a new manager: %w", err)
	}

	if err := nonk8sapi.AddNonK8sApiServer(mgr, processPostgreSQL,
		managerConfig.NonK8sAPIServerConfig); err != nil {
		return nil, fmt.Errorf("failed to add non-k8s-api-server: %w", err)
	}

	if managerConfig.EnableGlobalResource {
		if err := specsyncer.AddGlobalResourceSpecSyncers(mgr, managerConfig, processPostgreSQL); err != nil {
			return nil, fmt.Errorf("failed to add global resource spec syncers: %w", err)
		}
	}

	if err := specsyncer.AddBasicSpecSyncers(mgr); err != nil {
		return nil, fmt.Errorf("failed to add basic spec syncers: %w", err)
	}

	if _, err := statussyncer.AddStatusSyncers(mgr, managerConfig); err != nil {
		return nil, fmt.Errorf("failed to add transport-to-db syncers: %w", err)
	}

	if err := cronjob.AddSchedulerToManager(ctx, mgr, processPostgreSQL.GetConn(),
		managerConfig, enableSimulation); err != nil {
		return nil, fmt.Errorf("failed to add scheduler to manager: %w", err)
	}

	eventKafkaConfig := deepcopy.Copy(managerConfig.TransportConfig.KafkaConfig).(*transport.KafkaConfig)
	eventKafkaConfig.ConsumerConfig.ConsumerTopic = managerConfig.EventExporterTopic
	if err := eventcollector.AddEventCollector(ctx, mgr, eventKafkaConfig); err != nil {
		return nil, fmt.Errorf("failed to add event collector: %w", err)
	}

	if err := mgr.Add(globalhubmetrics.NewGlobalHubMetrics()); err != nil {
		return nil, fmt.Errorf("failed to add metrics to manager: %w", err)
	}

	return mgr, nil
}

// function to handle defers with exit, see https://stackoverflow.com/a/27629493/553720.
func doMain(ctx context.Context, restConfig *rest.Config) int {
	managerConfig := parseFlags()
	if err := completeConfig(managerConfig); err != nil {
		setupLog.Error(err, "failed to complete configuration")
		return 1
	}
	utils.PrintVersion(setupLog)

	processPostgreSQL, err := postgresql.NewSpecPostgreSQL(ctx, managerConfig.DatabaseConfig)
	if err != nil {
		setupLog.Error(err, "failed to initialize process PostgreSQL")
		return 1
	}
	defer processPostgreSQL.Stop()

	err = database.InitGormInstance(&database.DatabaseConfig{
		URL:        managerConfig.DatabaseConfig.ProcessDatabaseURL,
		Dialect:    database.PostgresDialect,
		CaCertPath: managerConfig.DatabaseConfig.CACertPath,
		PoolSize:   managerConfig.DatabaseConfig.MaxOpenConns,
	})
	if err != nil {
		setupLog.Error(err, "failed to initialize GORM instance")
		return 1
	}
	defer database.CloseGorm()

	mgr, err := createManager(ctx, restConfig, managerConfig, processPostgreSQL)
	if err != nil {
		setupLog.Error(err, "failed to create manager")
		return 1
	}

	if managerConfig.EnableGlobalResource {
		hookServer := mgr.GetWebhookServer()
		setupLog.Info("registering webhooks to the webhook server")
		hookServer.Register("/mutating", &webhook.Admission{
			Handler: mgrwebhook.NewAdmissionHandler(mgr.GetClient(), mgr.GetScheme()),
		})
	}

	setupLog.Info("Starting the Manager")
	if err := mgr.Start(ctx); err != nil {
		setupLog.Error(err, "manager exited non-zero")
		return 1
	}

	return 0
}

func main() {
	os.Exit(doMain(ctrl.SetupSignalHandler(), ctrl.GetConfigOrDie()))
}

func initCache(config *rest.Config, cacheOpts cache.Options) (cache.Cache, error) {
	cacheOpts.ByObject = map[client.Object]cache.ByObject{
		&corev1.Secret{}: {
			Field: fields.OneTermEqualSelector("metadata.namespace", managerNamespace),
		},
		&corev1.ConfigMap{}: {
			Field: fields.OneTermEqualSelector("metadata.namespace", managerNamespace),
		},
		&applicationv1beta1.Application{}:          {},
		&channelv1.Channel{}:                       {},
		&clusterv1beta2.ManagedClusterSet{}:        {},
		&clusterv1beta2.ManagedClusterSetBinding{}: {},
		&clusterv1.ManagedCluster{}:                {},
		&clusterv1beta1.Placement{}:                {},
		&policyv1.PlacementBinding{}:               {},
		&placementrulev1.PlacementRule{}:           {},
		&policyv1.Policy{}:                         {},
		&subscriptionv1.Subscription{}:             {},
	}
	return cache.New(config, cacheOpts)
}
