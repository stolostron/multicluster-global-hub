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
	"runtime"
	"strings"
	"time"

	// Import all Kubernetes client auth plugins (e.g. Azure, GCP, OIDC, etc.)
	"github.com/go-logr/logr"
	"github.com/operator-framework/operator-sdk/pkg/log/zap"
	"github.com/spf13/pflag"
	_ "k8s.io/client-go/plugin/pkg/client/auth"
	"k8s.io/client-go/rest"
	ctrl "sigs.k8s.io/controller-runtime"
	"sigs.k8s.io/controller-runtime/pkg/cache"
	"sigs.k8s.io/controller-runtime/pkg/webhook"

	managerconfig "github.com/stolostron/multicluster-global-hub/manager/pkg/config"
	"github.com/stolostron/multicluster-global-hub/manager/pkg/grafana"
	"github.com/stolostron/multicluster-global-hub/manager/pkg/nonk8sapi"
	"github.com/stolostron/multicluster-global-hub/manager/pkg/scheme"
	"github.com/stolostron/multicluster-global-hub/manager/pkg/specsyncer/db2transport/db/postgresql"
	specsyncer "github.com/stolostron/multicluster-global-hub/manager/pkg/specsyncer/db2transport/syncer"
	"github.com/stolostron/multicluster-global-hub/manager/pkg/specsyncer/spec2db"
	statussyncer "github.com/stolostron/multicluster-global-hub/manager/pkg/statussyncer/transport2db/syncer"
	mgrwebhook "github.com/stolostron/multicluster-global-hub/manager/pkg/webhook"
	commonobjects "github.com/stolostron/multicluster-global-hub/pkg/objects"
	"github.com/stolostron/multicluster-global-hub/pkg/statistics"
	"github.com/stolostron/multicluster-global-hub/pkg/transport"
	"github.com/stolostron/multicluster-global-hub/pkg/transport/producer"
	"github.com/stolostron/multicluster-global-hub/pkg/transport/protocol"
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
)

func parseFlags() (*managerconfig.ManagerConfig, error) {
	managerConfig := &managerconfig.ManagerConfig{
		SyncerConfig:   &managerconfig.SyncerConfig{},
		DatabaseConfig: &managerconfig.DatabaseConfig{},
		TransportConfig: &transport.TransportConfig{
			KafkaConfig: &protocol.KafkaConfig{
				EnableTSL:      true,
				ProducerConfig: &protocol.KafkaProducerConfig{},
				ConsumerConfig: &protocol.KafkaConsumerConfig{},
			},
		},
		StatisticsConfig:      &statistics.StatisticsConfig{},
		NonK8sAPIServerConfig: &nonk8sapi.NonK8sAPIServerConfig{},
		ElectionConfig:        &commonobjects.LeaderElectionConfig{},
	}

	pflag.StringVar(&managerConfig.ManagerNamespace, "manager-namespace", "open-cluster-management",
		"The manager running namespace, also used as leader election namespace.")
	pflag.StringVar(&managerConfig.WatchNamespace, "watch-namespace", "",
		"The watching namespace of the controllers, multiple namespace must be splited by comma.")
	pflag.DurationVar(&managerConfig.SyncerConfig.SpecSyncInterval, "spec-sync-interval", 5*time.Second,
		"The synchronization interval of resources in spec.")
	pflag.DurationVar(&managerConfig.SyncerConfig.StatusSyncInterval, "status-sync-interval", 5*time.Second,
		"The synchronization interval of resources in status.")
	pflag.DurationVar(&managerConfig.SyncerConfig.DeletedLabelsTrimmingInterval, "deleted-labels-trimming-interval",
		5*time.Second, "The trimming interval of deleted labels.")
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
	pflag.StringVar(&managerConfig.TransportConfig.KafkaConfig.CertPath, "kafka-ca-path", "",
		"The certificate path of CA certificate for kafka bootstrap server.")
	pflag.StringVar(&managerConfig.TransportConfig.KafkaConfig.ProducerConfig.ProducerID, "kakfa-producer-id",
		"multicluster-global-hub", "ID for the kafka producer.")
	pflag.StringVar(&managerConfig.TransportConfig.KafkaConfig.ProducerConfig.ProducerTopic, "kakfa-producer-topic",
		"spec", "Topic for the kafka producer.")
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
	// add flags for logger
	pflag.CommandLine.AddFlagSet(zap.FlagSet())
	pflag.CommandLine.AddGoFlagSet(flag.CommandLine)
	pflag.Parse()

	if managerConfig.DatabaseConfig.ProcessDatabaseURL == "" {
		return nil, fmt.Errorf("database url for process user: %w", errFlagParameterEmpty)
	}

	if managerConfig.DatabaseConfig.TransportBridgeDatabaseURL == "" {
		return nil, fmt.Errorf("database url for transport-bridge user: %w", errFlagParameterEmpty)
	}

	if managerConfig.TransportConfig.KafkaConfig.ProducerConfig.MessageSizeLimitKB > producer.MaxMessageSizeLimit {
		return nil, fmt.Errorf("%w - size must not exceed %d : %s", errFlagParameterIllegalValue,
			managerConfig.TransportConfig.KafkaConfig.ProducerConfig.MessageSizeLimitKB, "kafka-message-size-limit")
	}

	return managerConfig, nil
}

func initializeLogger() logr.Logger {
	ctrl.SetLogger(zap.Logger())
	log := ctrl.Log.WithName("cmd")

	return log
}

func printVersion(log logr.Logger) {
	log.Info(fmt.Sprintf("Go Version: %s", runtime.Version()))
	log.Info(fmt.Sprintf("Go OS/Arch: %s/%s", runtime.GOOS, runtime.GOARCH))
}

func createManager(restConfig *rest.Config, managerConfig *managerconfig.ManagerConfig,
	processPostgreSQL, transportBridgePostgreSQL *postgresql.PostgreSQL,
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

	if err := grafana.AddGrafanaServer(mgr, managerConfig); err != nil {
		return nil, fmt.Errorf("failed to add grafana: %w", err)
	}

	if err := spec2db.AddSpec2DBControllers(mgr, processPostgreSQL); err != nil {
		return nil, fmt.Errorf("failed to add spec-to-db controllers: %w", err)
	}

	if err := specsyncer.AddDB2TransportSyncers(mgr, transportBridgePostgreSQL, managerConfig); err != nil {
		return nil, fmt.Errorf("failed to add db-to-transport syncers: %w", err)
	}

	if err := specsyncer.AddStatusDBWatchers(mgr, transportBridgePostgreSQL, transportBridgePostgreSQL,
		managerConfig.SyncerConfig.DeletedLabelsTrimmingInterval); err != nil {
		return nil, fmt.Errorf("failed to add status db watchers: %w", err)
	}

	if _, err := statussyncer.AddTransport2DBSyncers(mgr, managerConfig); err != nil {
		return nil, fmt.Errorf("failed to add transport-to-db syncers: %w", err)
	}

	return mgr, nil
}

// function to handle defers with exit, see https://stackoverflow.com/a/27629493/553720.
func doMain(ctx context.Context, restConfig *rest.Config) int {
	log := initializeLogger()
	printVersion(log)
	// create hoh manager configuration from command parameters
	managerConfig, err := parseFlags()
	if err != nil {
		log.Error(err, "flags parse error")
		return 1
	}

	// db layer initialization for process user
	processPostgreSQL, err := postgresql.NewPostgreSQL(managerConfig.DatabaseConfig.ProcessDatabaseURL)
	if err != nil {
		log.Error(err, "failed to initialize process PostgreSQL")
		return 1
	}
	defer processPostgreSQL.Stop()

	// db layer initialization for transport-bridge user
	transportBridgePostgreSQL, err := postgresql.NewPostgreSQL(
		managerConfig.DatabaseConfig.TransportBridgeDatabaseURL)
	if err != nil {
		log.Error(err, "failed to initialize transport-bridge PostgreSQL")
		return 1
	}
	defer transportBridgePostgreSQL.Stop()

	mgr, err := createManager(restConfig, managerConfig, processPostgreSQL, transportBridgePostgreSQL)
	if err != nil {
		log.Error(err, "failed to create manager")
		return 1
	}

	hookServer := mgr.GetWebhookServer()
	log.Info("registering webhooks to the webhook server")
	hookServer.Register("/mutating", &webhook.Admission{
		Handler: &mgrwebhook.AdmissionHandler{Client: mgr.GetClient()},
	})

	log.Info("Starting the Cmd.")

	if err := mgr.Start(ctx); err != nil {
		log.Error(err, "manager exited non-zero")
		return 1
	}

	return 0
}

func main() {
	os.Exit(doMain(ctrl.SetupSignalHandler(), ctrl.GetConfigOrDie()))
}
