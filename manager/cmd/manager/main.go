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
	_ "k8s.io/client-go/plugin/pkg/client/auth"
	"k8s.io/client-go/rest"
	ctrl "sigs.k8s.io/controller-runtime"
	"sigs.k8s.io/controller-runtime/pkg/cache"
	"sigs.k8s.io/controller-runtime/pkg/log/zap"
	"sigs.k8s.io/controller-runtime/pkg/webhook"

	managerconfig "github.com/stolostron/multicluster-global-hub/manager/pkg/config"
	"github.com/stolostron/multicluster-global-hub/manager/pkg/cronjob"
	"github.com/stolostron/multicluster-global-hub/manager/pkg/eventcollector"
	"github.com/stolostron/multicluster-global-hub/manager/pkg/nonk8sapi"
	"github.com/stolostron/multicluster-global-hub/manager/pkg/scheme"
	"github.com/stolostron/multicluster-global-hub/manager/pkg/specsyncer"
	"github.com/stolostron/multicluster-global-hub/manager/pkg/specsyncer/db2transport/db/postgresql"
	statussyncer "github.com/stolostron/multicluster-global-hub/manager/pkg/statussyncer"
	mgrwebhook "github.com/stolostron/multicluster-global-hub/manager/pkg/webhook"
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
	leaderElectionLockID       = "multicluster-global-hub-lock"
)

var (
	errFlagParameterEmpty        = errors.New("flag parameter empty")
	errFlagParameterIllegalValue = errors.New("flag parameter illegal value")
	enableSimulation             = false
	setupLog                     = ctrl.Log.WithName("setup")
)

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
	}

	// add zap flags
	opts := utils.CtrlZapOptions()
	defaultFlags := flag.CommandLine
	opts.BindFlags(defaultFlags)
	pflag.CommandLine.AddGoFlagSet(defaultFlags)

	pflag.StringVar(&managerConfig.ManagerNamespace, "manager-namespace", "open-cluster-management",
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
		"kafka-brokers-cluster-kafka-bootstrap.kafka.svc:9092", "The bootstrap server for kafka.")
	pflag.StringVar(&managerConfig.TransportConfig.KafkaConfig.CaCertPath, "kafka-ca-cert-path", "",
		"The path of CA certificate for kafka bootstrap server.")
	pflag.StringVar(&managerConfig.TransportConfig.KafkaConfig.ClientCertPath, "kafka-client-cert-path", "",
		"The path of client certificate for kafka bootstrap server.")
	pflag.StringVar(&managerConfig.TransportConfig.KafkaConfig.ClientKeyPath, "kafka-client-key-path", "",
		"The path of client key for kafka bootstrap server.")
	pflag.StringVar(&managerConfig.DatabaseConfig.CACertPath, "postgres-ca-path", "/postgres-ca/ca.crt",
		"The path of CA certificate for kafka bootstrap server.")
	pflag.StringVar(&managerConfig.TransportConfig.KafkaConfig.ProducerConfig.ProducerID, "kakfa-producer-id",
		"multicluster-global-hub", "ID for the kafka producer.")
	pflag.StringVar(&managerConfig.TransportConfig.KafkaConfig.ProducerConfig.ProducerTopic, "kakfa-producer-topic",
		"spec", "Topic for the kafka producer.")
	pflag.StringVar(&managerConfig.EventExporterTopic, "event-exporter-topic", "event", "Topic for the event exporter.")
	pflag.IntVar(&managerConfig.TransportConfig.KafkaConfig.ProducerConfig.MessageSizeLimitKB,
		"kafka-message-size-limit", 940, "The limit for kafka message size in KB.")
	pflag.StringVar(&managerConfig.TransportConfig.KafkaConfig.ConsumerConfig.ConsumerID,
		"kakfa-consumer-id", "multicluster-global-hub", "ID for the kafka consumer.")
	pflag.StringVar(&managerConfig.TransportConfig.KafkaConfig.ConsumerConfig.ConsumerTopic,
		"kakfa-consumer-topic", "status", "Topic for the kafka consumer.")
	pflag.DurationVar(&managerConfig.StatisticsConfig.LogInterval, "statistics-log-interval", 0*time.Second,
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
	pflag.IntVar(&managerConfig.DatabaseConfig.DataExpiration, "data-expiration", 18,
		"data retention duration(months) indicates how long the data will be kept in the database")

	pflag.Parse()
	// set zap logger
	ctrl.SetLogger(zap.New(zap.UseFlagOptions(&opts)))

	pflag.Visit(func(f *pflag.Flag) {
		// set enableSimulation to be true when manually set 'scheduler-interval' flag
		if f.Name == "scheduler-interval" && f.Changed {
			enableSimulation = true
		}
	})
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
		MetricsBindAddress:      fmt.Sprintf("%s:%d", metricsHost, metricsPort),
		LeaderElection:          true,
		LeaderElectionNamespace: managerConfig.ManagerNamespace,
		LeaderElectionID:        leaderElectionLockID,
		LeaseDuration:           &leaseDuration,
		RenewDeadline:           &renewDeadline,
		RetryPeriod:             &retryPeriod,
		Port:                    webhookPort,
		CertDir:                 webhookCertDir,
		TLSOpts: []func(*tls.Config){
			func(config *tls.Config) {
				config.MinVersion = tls.VersionTLS12
			},
		},
	}

	// Add support for MultiNamespace set in WATCH_NAMESPACE (e.g ns1,ns2)
	// Note that this is not intended to be used for excluding namespaces, this is better done via a Predicate
	// Also note that you may face performance issues when using this with a high number of namespaces.
	// More Info: https://godoc.org/github.com/kubernetes-sigs/controller-runtime/pkg/cache#MultiNamespacedCacheBuilder
	if strings.Contains(managerConfig.WatchNamespace, ",") {
		options.Namespace = ""
		options.NewCache = cache.MultiNamespacedCacheBuilder(
			strings.Split(managerConfig.WatchNamespace, ","))
	}

	mgr, err := ctrl.NewManager(restConfig, options)
	if err != nil {
		return nil, fmt.Errorf("failed to create a new manager: %w", err)
	}

	if err := scheme.AddToScheme(mgr.GetScheme()); err != nil {
		return nil, fmt.Errorf("failed to add schemes: %w", err)
	}

	if err := nonk8sapi.AddNonK8sApiServer(mgr, processPostgreSQL,
		managerConfig.NonK8sAPIServerConfig); err != nil {
		return nil, fmt.Errorf("failed to add non-k8s-api-server: %w", err)
	}

	if err := specsyncer.AddSpecSyncers(mgr, managerConfig, processPostgreSQL); err != nil {
		return nil, fmt.Errorf("failed to add spec syncers: %w", err)
	}

	if _, err := statussyncer.AddStatusSyncers(mgr, managerConfig); err != nil {
		return nil, fmt.Errorf("failed to add transport-to-db syncers: %w", err)
	}

	if err := cronjob.AddSchedulerToManager(ctx, mgr, processPostgreSQL.GetConn(),
		managerConfig.SchedulerInterval, enableSimulation); err != nil {
		return nil, fmt.Errorf("failed to add scheduler to manager: %w", err)
	}

	eventKafkaConfig := deepcopy.Copy(managerConfig.TransportConfig.KafkaConfig).(*transport.KafkaConfig)
	eventKafkaConfig.ConsumerConfig.ConsumerTopic = managerConfig.EventExporterTopic
	if err := eventcollector.AddEventCollector(ctx, mgr, eventKafkaConfig); err != nil {
		return nil, fmt.Errorf("failed to add event collector: %w", err)
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

	hookServer := mgr.GetWebhookServer()
	setupLog.Info("registering webhooks to the webhook server")
	hookServer.Register("/mutating", &webhook.Admission{
		Handler: &mgrwebhook.AdmissionHandler{Client: mgr.GetClient()},
	})

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
