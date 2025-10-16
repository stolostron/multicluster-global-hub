// Copyright (c) 2020 Red Hat, Inc.
// Copyright Contributors to the Open Cluster Management project

package main

import (
	"context"
	"crypto/tls"
	"database/sql"
	"errors"
	"fmt"
	"os"
	"strings"
	"time"

	"github.com/spf13/pflag"
	corev1 "k8s.io/api/core/v1"
	"k8s.io/apimachinery/pkg/fields"
	_ "k8s.io/client-go/plugin/pkg/client/auth"
	"k8s.io/client-go/rest"
	ctrl "sigs.k8s.io/controller-runtime"
	"sigs.k8s.io/controller-runtime/pkg/cache"
	"sigs.k8s.io/controller-runtime/pkg/client"
	metricsserver "sigs.k8s.io/controller-runtime/pkg/metrics/server"
	"sigs.k8s.io/controller-runtime/pkg/webhook"

	"github.com/stolostron/multicluster-global-hub/manager/pkg/configs"
	"github.com/stolostron/multicluster-global-hub/manager/pkg/controllers"
	"github.com/stolostron/multicluster-global-hub/manager/pkg/migration"
	"github.com/stolostron/multicluster-global-hub/manager/pkg/processes/cronjob"
	"github.com/stolostron/multicluster-global-hub/manager/pkg/processes/hubmanagement"
	"github.com/stolostron/multicluster-global-hub/manager/pkg/restapis"
	specsyncer "github.com/stolostron/multicluster-global-hub/manager/pkg/spec"
	"github.com/stolostron/multicluster-global-hub/manager/pkg/status"
	mgrwebhook "github.com/stolostron/multicluster-global-hub/manager/pkg/webhook"
	"github.com/stolostron/multicluster-global-hub/pkg/constants"
	"github.com/stolostron/multicluster-global-hub/pkg/database"
	"github.com/stolostron/multicluster-global-hub/pkg/logger"
	commonobjects "github.com/stolostron/multicluster-global-hub/pkg/objects"
	"github.com/stolostron/multicluster-global-hub/pkg/statistics"
	"github.com/stolostron/multicluster-global-hub/pkg/transport"
	"github.com/stolostron/multicluster-global-hub/pkg/transport/controller"
	"github.com/stolostron/multicluster-global-hub/pkg/utils"
)

const (
	metricsHost                = "0.0.0.0"
	metricsPort          int32 = 8384
	webhookPort                = 9443
	webhookCertDir             = "/webhook-certs"
	leaderElectionLockID       = "multicluster-global-hub-manager-lock"
	launchJobNamesEnv          = "LAUNCH_JOB_NAMES"
	namespacePath              = "metadata.namespace"
)

var (
	managerNamespace      = constants.GHDefaultNamespace
	enableSimulation      = false
	errFlagParameterEmpty = errors.New("flag parameter empty")
	log                   = logger.DefaultZapLogger()
)

func parseFlags() *configs.ManagerConfig {
	managerConfig := &configs.ManagerConfig{
		SyncerConfig:   &configs.SyncerConfig{},
		DatabaseConfig: &configs.DatabaseConfig{},
		TransportConfig: &transport.TransportInternalConfig{
			EnableDatabaseOffset: true,
		},
		StatisticsConfig:    &statistics.StatisticsConfig{},
		RestAPIServerConfig: &restapis.RestApiServerConfig{},
		ElectionConfig:      &commonobjects.LeaderElectionConfig{},
		LaunchJobNames:      "",
	}

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
	pflag.DurationVar(&managerConfig.TransportConfig.CommitterInterval, "transport-committer-interval",
		40*time.Second, "The committer interval for transport layer.")
	pflag.StringVar(&managerConfig.DatabaseConfig.CACertPath, "postgres-ca-path", "/postgres-ca/ca.crt",
		"The path of CA certificate for kafka bootstrap server.")
	pflag.StringVar(&managerConfig.StatisticsConfig.LogInterval, "statistics-log-interval", "10m",
		"The log interval for statistics.")
	pflag.StringVar(&managerConfig.RestAPIServerConfig.ClusterAPIURL, "cluster-api-url",
		"https://kubernetes.default.svc:443", "The cluster API URL for nonK8s API server.")
	pflag.StringVar(&managerConfig.RestAPIServerConfig.ClusterAPICABundlePath, "cluster-api-cabundle-path",
		"/var/run/secrets/kubernetes.io/serviceaccount/ca.crt", "The CA bundle path for cluster API.")
	pflag.StringVar(&managerConfig.RestAPIServerConfig.ServerBasePath, "server-base-path",
		"/global-hub-api/v1", "The base path for nonK8s API server.")
	pflag.IntVar(&managerConfig.ElectionConfig.LeaseDuration, "lease-duration", 137, "controller leader lease duration")
	pflag.IntVar(&managerConfig.ElectionConfig.RenewDeadline, "renew-deadline", 107, "controller leader renew deadline")
	pflag.IntVar(&managerConfig.ElectionConfig.RetryPeriod, "retry-period", 26, "controller leader retry period")
	pflag.IntVar(&managerConfig.DatabaseConfig.DataRetention, "data-retention", 18,
		"data retention indicates how many months the expired data will kept in the database")
	pflag.BoolVar(&managerConfig.EnableGlobalResource, "enable-global-resource", false,
		"enable the global resource feature")
	pflag.BoolVar(&managerConfig.WithACM, "with-acm", false,
		"run on Red Hat Advanced Cluster Management")
	pflag.BoolVar(&managerConfig.EnableInventoryAPI, "enable-inventory-api", false,
		"enable the inventory api")
	pflag.BoolVar(&managerConfig.EnablePprof, "enable-pprof", false, "enable the pprof tool")
	pflag.IntVar(&managerConfig.TransportConfig.FailureThreshold, "transport-failure-threshold", 10,
		"Restart the pod if the transport error count exceeds the transport-failure-threshold within 5 minutes.")
	pflag.Parse()

	pflag.Visit(func(f *pflag.Flag) {
		// set enableSimulation to be true when manually set 'scheduler-interval' flag
		if f.Name == "scheduler-interval" && f.Changed {
			enableSimulation = true
		}
	})
	managerNamespace = managerConfig.ManagerNamespace
	return managerConfig
}

func completeConfig(managerConfig *configs.ManagerConfig) error {
	if managerConfig.DatabaseConfig.ProcessDatabaseURL == "" {
		return fmt.Errorf("database url for process user: %w", errFlagParameterEmpty)
	}
	// the specified jobs(concatenate multiple jobs with ',') runs when the container starts
	val, ok := os.LookupEnv(launchJobNamesEnv)
	if ok && val != "" {
		managerConfig.LaunchJobNames = val
	}
	return nil
}

func createManager(ctx context.Context,
	restConfig *rest.Config,
	managerConfig *configs.ManagerConfig,
	sqlConn *sql.Conn,
) (ctrl.Manager, error) {
	leaseDuration := time.Duration(managerConfig.ElectionConfig.LeaseDuration) * time.Second
	renewDeadline := time.Duration(managerConfig.ElectionConfig.RenewDeadline) * time.Second
	retryPeriod := time.Duration(managerConfig.ElectionConfig.RetryPeriod) * time.Second
	options := ctrl.Options{
		Scheme: configs.GetRuntimeScheme(),
		Metrics: metricsserver.Options{
			BindAddress: fmt.Sprintf("%s:%d", metricsHost, metricsPort),
		},
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
						config.MinVersion = tls.VersionTLS13
					},
				},
			},
		}
	}

	// Add support for MultiNamespace set in WATCH_NAMESPACE (e.g ns1,ns2)
	// Note that this is not intended to be used for excluding namespaces, this is better done via a Predicate
	// Also note that you may face performance issues when using this with a high number of namespaces.
	// More Info: https://godoc.org/github.com/kubernetes-sigs/controller-runtime/pkg/cache#MultiNamespacedCacheBuilder
	if managerConfig.WatchNamespace != "" {
		namespaces := map[string]cache.Config{}
		if strings.Contains(managerConfig.WatchNamespace, ",") {
			for _, ns := range strings.Split(managerConfig.WatchNamespace, ",") {
				namespaces[ns] = cache.Config{}
			}
		} else {
			namespaces[managerConfig.WatchNamespace] = cache.Config{}
		}
		options.Cache.DefaultNamespaces = namespaces
	}

	mgr, err := ctrl.NewManager(restConfig, options)
	if err != nil {
		return nil, fmt.Errorf("failed to create a new manager: %w", err)
	}

	// add the configmap: logLevel
	if err = logger.AddLogConfigController(ctx, mgr); err != nil {
		return nil, fmt.Errorf("failed to add configmap controller to manager: %w", err)
	}
	configs.SetEnableInventoryAPI(managerConfig.EnableInventoryAPI)
	err = controller.NewTransportCtrl(managerConfig.ManagerNamespace, constants.GHTransportConfigSecret,
		transportCallback(mgr, managerConfig),
		managerConfig.TransportConfig, true,
	).SetupWithManager(mgr)
	if err != nil {
		return nil, fmt.Errorf("failed to add the transport controller")
	}

	// the cronjob can start without producer and consumer
	if err := cronjob.AddSchedulerToManager(ctx, mgr, managerConfig, enableSimulation); err != nil {
		return nil, fmt.Errorf("failed to add scheduler to manager: %w", err)
	}
	if !managerConfig.WithACM {
		return mgr, nil
	}

	// need lock DB for backup
	backupPVC := controllers.NewBackupPVCReconciler(mgr, sqlConn)
	if err := backupPVC.SetupWithManager(mgr); err != nil {
		return nil, err
	}
	if managerConfig.EnableGlobalResource {
		if err := restapis.AddRestApiServer(mgr, managerConfig.RestAPIServerConfig); err != nil {
			return nil, fmt.Errorf("failed to add non-k8s-api-server: %w", err)
		}
	}
	return mgr, nil
}

func transportCallback(mgr ctrl.Manager, managerConfig *configs.ManagerConfig) controller.TransportCallback {
	return func(transportClient transport.TransportClient) error {
		if !managerConfig.WithACM {
			return nil
		}
		producer := transportClient.GetProducer()
		consumer := transportClient.GetConsumer()
		requester := transportClient.GetRequester()
		if managerConfig.EnableGlobalResource {
			if err := specsyncer.AddToManager(mgr, managerConfig, producer); err != nil {
				return fmt.Errorf("failed to add global resource spec syncers: %w", err)
			}
		}

		if consumer == nil {
			return fmt.Errorf("consumer is not initialized")
		}

		if err := status.AddStatusSyncers(mgr, consumer, requester, managerConfig); err != nil {
			return fmt.Errorf("failed to add transport-to-db syncers: %w", err)
		}

		// add hub management
		if err := hubmanagement.AddHubManagement(mgr, producer); err != nil {
			return fmt.Errorf("failed to add hubmanagement to manager - %w", err)
		}

		// add managedclustermigration controller
		if err := migration.AddMigrationToManager(mgr, producer, managerConfig); err != nil {
			return fmt.Errorf("failed to add migration controller to manager - %w", err)
		}
		return nil
	}
}

// function to handle defers with exit, see https://stackoverflow.com/a/27629493/553720.
func doMain(ctx context.Context, restConfig *rest.Config) error {
	managerConfig := parseFlags()
	if err := completeConfig(managerConfig); err != nil {
		return fmt.Errorf("failed to complete configuration %w", err)
	}

	if managerConfig.EnablePprof {
		go utils.StartDefaultPprofServer()
	}

	utils.PrintRuntimeInfo()
	databaseConfig := &database.DatabaseConfig{
		URL:        managerConfig.DatabaseConfig.ProcessDatabaseURL,
		Dialect:    database.PostgresDialect,
		CaCertPath: managerConfig.DatabaseConfig.CACertPath,
		PoolSize:   managerConfig.DatabaseConfig.MaxOpenConns,
	}
	// Init the default gorm instance, it's used to sync data to db
	err := database.InitGormInstance(databaseConfig)
	if err != nil {
		return fmt.Errorf("failed to initialize GORM instance %w", err)
	}
	defer database.CloseGorm(database.GetSqlDb())

	// Init the backup gorm instance, it's used to add lock when backup database
	_, sqlBackupConn, err := database.NewGormConn(databaseConfig)
	if err != nil {
		return fmt.Errorf("failed to initialize GORM conn instance %w", err)
	}
	defer database.CloseGorm(sqlBackupConn)

	sqlConn, err := sqlBackupConn.Conn(ctx)
	if err != nil {
		return fmt.Errorf("failed to sql conn instance %w", err)
	}
	mgr, err := createManager(ctx, restConfig, managerConfig, sqlConn)
	if err != nil {
		return fmt.Errorf("failed to create manager %w", err)
	}

	if managerConfig.EnableGlobalResource {
		hookServer := mgr.GetWebhookServer()
		log.Info("registering webhooks to the webhook server")
		hookServer.Register("/mutating", &webhook.Admission{
			Handler: mgrwebhook.NewAdmissionHandler(mgr.GetScheme()),
		})
	}

	if err := mgr.Start(ctx); err != nil {
		return fmt.Errorf("failed to start manager %w", err)
	}

	return nil
}

func main() {
	defer func() { _ = logger.CoreZapLogger().Sync() }()
	if err := doMain(ctrl.SetupSignalHandler(), ctrl.GetConfigOrDie()); err != nil {
		logger.DefaultZapLogger().Panicf("failed to run the main: %v", err)
	}
}

func initCache(config *rest.Config, cacheOpts cache.Options) (cache.Cache, error) {
	cacheOpts.ByObject = map[client.Object]cache.ByObject{
		&corev1.ConfigMap{}: {
			Field: fields.OneTermEqualSelector(namespacePath, managerNamespace),
		},
		&corev1.PersistentVolumeClaim{}: {
			Field: fields.OneTermEqualSelector(namespacePath, managerNamespace),
		},
	}
	return cache.New(config, cacheOpts)
}
