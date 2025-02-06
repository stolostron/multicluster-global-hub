package storage

import (
	"context"
	"crypto/rand"
	"embed"
	"encoding/json"
	"fmt"
	iofs "io/fs"
	"math/big"
	"net/url"
	"strings"
	"time"

	"github.com/jackc/pgx/v5"
	promv1 "github.com/prometheus-operator/prometheus-operator/pkg/apis/monitoring/v1"
	appsv1 "k8s.io/api/apps/v1"
	corev1 "k8s.io/api/core/v1"
	"k8s.io/apimachinery/pkg/api/errors"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/types"
	"k8s.io/apimachinery/pkg/util/sets"
	ctrl "sigs.k8s.io/controller-runtime"
	"sigs.k8s.io/controller-runtime/pkg/builder"
	"sigs.k8s.io/controller-runtime/pkg/client"
	"sigs.k8s.io/controller-runtime/pkg/controller/controllerutil"
	"sigs.k8s.io/controller-runtime/pkg/event"
	"sigs.k8s.io/controller-runtime/pkg/handler"
	"sigs.k8s.io/controller-runtime/pkg/predicate"

	"github.com/stolostron/multicluster-global-hub/operator/api/operator/v1alpha4"
	"github.com/stolostron/multicluster-global-hub/operator/pkg/config"
	"github.com/stolostron/multicluster-global-hub/operator/pkg/utils"
	"github.com/stolostron/multicluster-global-hub/pkg/constants"
	"github.com/stolostron/multicluster-global-hub/pkg/database"
	"github.com/stolostron/multicluster-global-hub/pkg/logger"
	commonutils "github.com/stolostron/multicluster-global-hub/pkg/utils"
)

// +kubebuilder:rbac:groups=operator.open-cluster-management.io,resources=multiclusterglobalhubs,verbs=get;list;watch;
// +kubebuilder:rbac:groups="",resources=services,verbs=get;list;watch;create;update;delete
// +kubebuilder:rbac:groups="",resources=secrets,verbs=get;list;watch;create;update;delete;patch
// +kubebuilder:rbac:groups="",resources=configmaps,verbs=get;list;watch;create;update;delete
// +kubebuilder:rbac:groups="apps",resources=deployments,verbs=get;list;watch;create;update;delete
// +kubebuilder:rbac:groups=monitoring.coreos.com,resources=servicemonitors;prometheusrules,verbs=get;create;delete;update;list;watch
// +kubebuilder:rbac:groups="apps",resources=statefulsets,verbs=get;list;watch;create;update;delete
// +kubebuilder:rbac:groups="",resources=persistentvolumeclaims,verbs=get;list;watch;create;update;delete;patch
// +kubebuilder:rbac:groups=postgres-operator.crunchydata.com,resources=postgresclusters,verbs=get;create;list;watch

//go:embed database
var databaseFS embed.FS

//go:embed database.old
var databaseOldFS embed.FS

//go:embed upgrade
var upgradeFS embed.FS

//go:embed manifests.sts
var stsPostgresFS embed.FS

var log = logger.DefaultZapLogger()

type StorageReconciler struct {
	ctrl.Manager
	upgrade                bool
	databaseReconcileCount int
	enableGlobalResource   bool
	enableMetrics          bool
}

var WatchedSecret = sets.NewString(
	constants.GHStorageSecretName,
	config.PostgresCertName,
	BuiltinPostgresName,
)

var WatchedConfigMap = sets.NewString(
	BuiltinPostgresCAName,
	BuiltinPostgresCustomizedConfigName,
	BuiltinPostgresCustomizedUsersName,
)

var (
	storageReconciler        *StorageReconciler
	updateConnection         bool
	appliedConfigMapUsers    map[string]string
	postgresUserNameTemplate = "postgresql-user-%s"
)

func (r *StorageReconciler) IsResourceRemoved() bool {
	return true
}

func StartController(initOption config.ControllerOption) (config.ControllerInterface, error) {
	if storageReconciler != nil {
		return storageReconciler, nil
	}
	log.Info("start storage controller")

	storageReconciler = NewStorageReconciler(initOption.Manager,
		initOption.OperatorConfig.GlobalResourceEnabled, initOption.MulticlusterGlobalHub.Spec.EnableMetrics)
	err := storageReconciler.SetupWithManager(initOption.Manager)
	if err != nil {
		storageReconciler = nil
		return nil, err
	}
	log.Infof("inited storage controller")
	return storageReconciler, nil
}

func NewStorageReconciler(mgr ctrl.Manager, enableGlobalResource, enableMetrics bool) *StorageReconciler {
	return &StorageReconciler{
		Manager:                mgr,
		upgrade:                false,
		databaseReconcileCount: 0,
		enableGlobalResource:   enableGlobalResource,
		enableMetrics:          enableMetrics,
	}
}

func (r *StorageReconciler) SetupWithManager(mgr ctrl.Manager) error {
	return ctrl.NewControllerManagedBy(mgr).Named("storageController").
		For(&v1alpha4.MulticlusterGlobalHub{},
			builder.WithPredicates(config.MGHPred)).
		Watches(&corev1.Secret{},
			&handler.EnqueueRequestForObject{}, builder.WithPredicates(secretPred)).
		Watches(&corev1.ConfigMap{},
			&handler.EnqueueRequestForObject{}, builder.WithPredicates(configMapPredicate)).
		Watches(&appsv1.StatefulSet{},
			&handler.EnqueueRequestForObject{}, builder.WithPredicates(statefulSetPred)).
		Watches(&corev1.ServiceAccount{},
			&handler.EnqueueRequestForObject{}, builder.WithPredicates(config.GeneralPredicate)).
		Watches(&promv1.PrometheusRule{},
			&handler.EnqueueRequestForObject{}, builder.WithPredicates(config.GeneralPredicate)).
		Watches(&promv1.ServiceMonitor{},
			&handler.EnqueueRequestForObject{}, builder.WithPredicates(config.GeneralPredicate)).
		Complete(r)
}

var configMapPredicate = predicate.Funcs{
	CreateFunc: func(e event.CreateEvent) bool {
		return WatchedConfigMap.Has(e.Object.GetName())
	},
	UpdateFunc: func(e event.UpdateEvent) bool {
		return WatchedConfigMap.Has(e.ObjectNew.GetName()) ||
			e.ObjectNew.GetLabels()[constants.GlobalHubOwnerLabelKey] == constants.GHOperatorOwnerLabelVal
	},
	DeleteFunc: func(e event.DeleteEvent) bool {
		return WatchedConfigMap.Has(e.Object.GetName()) ||
			e.Object.GetLabels()[constants.GlobalHubOwnerLabelKey] == constants.GHOperatorOwnerLabelVal
	},
}

var statefulSetPred = predicate.Funcs{
	CreateFunc: func(e event.CreateEvent) bool {
		return e.Object.GetNamespace() == commonutils.GetDefaultNamespace() &&
			e.Object.GetName() == BuiltinPostgresName
	},
	UpdateFunc: func(e event.UpdateEvent) bool {
		return e.ObjectNew.GetNamespace() == commonutils.GetDefaultNamespace() &&
			e.ObjectNew.GetName() == BuiltinPostgresName
	},
	DeleteFunc: func(e event.DeleteEvent) bool {
		return e.Object.GetNamespace() == commonutils.GetDefaultNamespace() &&
			e.Object.GetName() == BuiltinPostgresName
	},
}

var secretPred = predicate.Funcs{
	CreateFunc: func(e event.CreateEvent) bool {
		return WatchedSecret.Has(e.Object.GetName())
	},
	UpdateFunc: func(e event.UpdateEvent) bool {
		if e.ObjectNew.GetLabels()[constants.GlobalHubOwnerLabelKey] ==
			constants.GHOperatorOwnerLabelVal {
			return true
		}
		return WatchedSecret.Has(e.ObjectNew.GetName())
	},
	DeleteFunc: func(e event.DeleteEvent) bool {
		if e.Object.GetLabels()[constants.GlobalHubOwnerLabelKey] ==
			constants.GHOperatorOwnerLabelVal {
			return true
		}
		return WatchedSecret.Has(e.Object.GetName())
	},
}

func (r *StorageReconciler) Reconcile(ctx context.Context, req ctrl.Request) (ctrl.Result, error) {
	log.Debug("reconcile storage controller")
	mgh, err := config.GetMulticlusterGlobalHub(ctx, r.GetClient())
	if err != nil {
		return ctrl.Result{}, err
	}

	if mgh == nil || config.IsPaused(mgh) {
		return ctrl.Result{}, nil
	}
	if mgh.DeletionTimestamp != nil {
		appliedConfigMapUsers = nil
		updateConnection = false
		_ = config.SetStorageConnection(nil)
	}
	if !config.IsBYOPostgres() && !mgh.Spec.EnableMetrics {
		err = utils.PruneMetricsResources(ctx, r.GetClient(),
			map[string]string{
				constants.GlobalHubMetricsLabel: "postgres",
			})
		if err != nil {
			return ctrl.Result{}, err
		}
	}

	var reconcileErr error
	defer func() {
		err = config.UpdateMGHComponent(ctx, r.GetClient(),
			getDatabaseComponentStatus(ctx, r.GetClient(), mgh.Namespace, config.COMPONENTS_POSTGRES_NAME, reconcileErr),
			updateConnection,
		)
		if err != nil {
			log.Errorf("failed to update mgh status, err:%v", err)
		}
	}()
	storageConn, err := r.ReconcileStorage(ctx, mgh)
	if err != nil {
		reconcileErr = fmt.Errorf("storage not ready, Error: %v", err)
		return ctrl.Result{}, reconcileErr
	}
	updateConnection = config.SetStorageConnection(storageConn)

	needRequeue, err := r.ReconcileDatabase(ctx, mgh)
	if err != nil {
		reconcileErr = fmt.Errorf("database not ready, Error: %v", err)
		return ctrl.Result{}, reconcileErr
	}
	if needRequeue {
		return ctrl.Result{RequeueAfter: 5 * time.Second}, nil
	}
	config.SetDatabaseReady(true)

	// Update retention condition
	reconcileErr = config.UpdateCondition(ctx, r.GetClient(), types.NamespacedName{
		Namespace: mgh.Namespace,
		Name:      mgh.Name,
	}, getRetentionConditions(mgh), "")

	return ctrl.Result{}, reconcileErr
}

func getRetentionConditions(mgh *v1alpha4.MulticlusterGlobalHub) metav1.Condition {
	months, err := commonutils.ParseRetentionMonth(mgh.Spec.DataLayerSpec.Postgres.Retention)
	if err != nil {
		err = fmt.Errorf("failed to parse the retention month, err:%v", err)
		return metav1.Condition{
			Type:    config.CONDITION_TYPE_DATABASE,
			Status:  config.CONDITION_STATUS_FALSE,
			Reason:  config.CONDITION_REASON_RETENTION_PARSED_FAILED,
			Message: err.Error(),
		}
	}

	if months < 1 {
		months = 1
	}
	msg := fmt.Sprintf("The data will be kept in the database for %d months.", months)
	return metav1.Condition{
		Type:    config.CONDITION_TYPE_DATABASE,
		Status:  config.CONDITION_STATUS_TRUE,
		Reason:  config.CONDITION_REASON_RETENTION_PARSED,
		Message: msg,
	}
}

func (r *StorageReconciler) ReconcileStorage(ctx context.Context,
	mgh *v1alpha4.MulticlusterGlobalHub,
) (*config.PostgresConnection, error) {
	// support BYO postgres
	pgConnection, err := config.GetPGConnectionFromGHStorageSecret(ctx, r.GetClient())
	if err == nil {
		return pgConnection, nil
	} else if !errors.IsNotFound(err) {
		return nil, err
	}

	// then the storage secret is not found
	// if not-provided postgres secret, create crunchy postgres operator by subscription
	if config.GetInstallCrunchyOperator(mgh) {
		err := EnsureCrunchyPostgresSub(ctx, r.GetClient(), mgh)
		if err != nil {
			return nil, err
		}
		pgConnection, err = EnsureCrunchyPostgres(ctx, r.GetClient())
		if err != nil {
			return nil, err
		}
	} else {
		// create the statefulset postgres and initialize the r.MiddlewareConfig.PgConnection
		pgConnection, err = InitPostgresByStatefulset(ctx, mgh, r.Manager)
		if err != nil {
			return nil, err
		}
	}
	return pgConnection, nil
}

func (r *StorageReconciler) ReconcileDatabase(ctx context.Context, mgh *v1alpha4.MulticlusterGlobalHub) (bool, error) {
	var conn *pgx.Conn

	pgUsers, err := r.getPostgresUsers(ctx, mgh)
	if err != nil {
		return false, err
	}

	// Don't reconcile, or create the connection, when
	// 1. postgres users isn't updated
	// 2. database has been initialized
	if pgUsers == nil && r.databaseReconcileCount > 0 {
		return false, nil
	}

	defer func() {
		if conn != nil {
			if err := conn.Close(ctx); err != nil {
				log.Error(err, "failed to close connection to database")
			}
		}
	}()

	storageConn := config.GetStorageConnection()
	if storageConn == nil {
		return false, fmt.Errorf("storage connection is nil")
	}
	conn, err = database.PostgresConnection(ctx, storageConn.SuperuserDatabaseURI, storageConn.CACert)
	if err != nil {
		log.Infof("wait database ready, failed to connect database: %v", err)
		return true, nil
	}

	// apply the init users
	if !config.IsBYOPostgres() && pgUsers != nil {
		if err = r.applyPostgresUsers(ctx, conn, pgUsers.Data, mgh); err != nil {
			return false, err
		}
		log.Infof("applied the postgresql users from ConfigMap(%s) successfully!", pgUsers.Name)
		appliedConfigMapUsers = pgUsers.Data
	}

	// apply the global hub init SQL when the operator restarted
	if r.databaseReconcileCount == 0 {
		err = r.applyGlobalHubInitSQL(ctx, conn, storageConn.ReadonlyUserDatabaseURI)
		if err != nil {
			return false, err
		}
		log.Debug("global hub database initialized")
		r.databaseReconcileCount++
	}

	return false, nil
}

func (r *StorageReconciler) getPostgresUsers(ctx context.Context, mgh *v1alpha4.MulticlusterGlobalHub) (
	*corev1.ConfigMap, error,
) {
	currentCM := &corev1.ConfigMap{}
	err := r.GetClient().Get(ctx, client.ObjectKey{
		Name:      BuiltinPostgresCustomizedUsersName,
		Namespace: mgh.Namespace,
	}, currentCM)
	if err != nil && !errors.IsNotFound(err) {
		return nil, fmt.Errorf("unable to fetch ConfigMap: %w", err)
	}

	// not found the ConfigMap
	if errors.IsNotFound(err) {
		return nil, nil
	}

	if !configMapDataEqual(currentCM.Data, appliedConfigMapUsers) {
		return currentCM, nil
	}
	return nil, nil // No change detected
}

func configMapDataEqual(data1, data2 map[string]string) bool {
	if len(data1) != len(data2) {
		return false
	}
	for key, value := range data1 {
		if data2[key] != value {
			return false
		}
	}
	return true
}

func (r *StorageReconciler) applyPostgresUsers(ctx context.Context, conn *pgx.Conn, pgUsers map[string]string,
	mgh *v1alpha4.MulticlusterGlobalHub,
) error {
	for userName, dbStr := range pgUsers {

		// parse the databases
		var dbs []string
		err := json.Unmarshal([]byte(dbStr), &dbs)
		if err != nil {
			return fmt.Errorf("failed to parse ConfigMap value: %w", err)
		}

		// create postgres user
		pwd, err := r.createPostgresUser(ctx, conn, userName)
		if err != nil {
			return fmt.Errorf("error creating postgres user %s: %v", userName, err)
		}
		// create database and add permission for the user
		for _, db := range dbs {
			err = r.createDatabaseIfNotExists(ctx, conn, db)
			if err != nil {
				return fmt.Errorf("error creating database %s: %v", db, err)
			}
			err = r.grantPermissions(ctx, conn, userName, db)
			if err != nil {
				return fmt.Errorf("failed to grant permissions to user %s on database %s: %v", userName, db, err)
			}
		}
		// create the secret for the postgres user and databases
		if err = r.createPostgresUserSecret(ctx, userName, pwd, dbStr, mgh); err != nil {
			return fmt.Errorf("error creating postgres user secret %v", err)
		}
	}
	return nil
}

func (r *StorageReconciler) createDatabaseIfNotExists(ctx context.Context, conn *pgx.Conn, dbName string) error {
	var exists bool
	err := conn.QueryRow(ctx, `SELECT EXISTS(SELECT 1 FROM pg_database WHERE datname = $1);`, dbName).Scan(&exists)
	if err != nil {
		return fmt.Errorf("error checking if database %s exists: %v", dbName, err)
	}

	if !exists {
		createDBQuery := fmt.Sprintf("CREATE DATABASE \"%s\";", dbName)
		_, err := conn.Exec(ctx, createDBQuery)
		if err != nil {
			return fmt.Errorf("error creating database %s: %v", dbName, err)
		}
		log.Infof("database %s created.", dbName)
	} else {
		log.Infof("database %s already exists.", dbName)
	}

	return nil
}

// createPostgresUser return the password of the created user, if the password is empty if the user is already existing
func (r *StorageReconciler) createPostgresUser(ctx context.Context, conn *pgx.Conn, userName string,
) (string, error) {
	var roleExists bool
	err := conn.QueryRow(ctx, "SELECT EXISTS(SELECT 1 FROM pg_catalog.pg_roles WHERE rolname = $1)",
		userName).Scan(&roleExists)
	if err != nil {
		return "", fmt.Errorf("error checking if role exists: %v", err)
	}

	password := ""
	if roleExists {
		// updatePasswordQuery := fmt.Sprintf("ALTER ROLE \"%s\" WITH PASSWORD '%s';", user.Name, password)
		// _, err = conn.Exec(ctx, updatePasswordQuery)
		// if err != nil {
		// 	return fmt.Errorf("error updating password for role %s: %v", user.Name, err)
		// }
		log.Infof("postgres user already exist: %s", userName)
	} else {
		password = generatePassword(16)
		createRoleQuery := fmt.Sprintf("CREATE ROLE \"%s\" LOGIN PASSWORD '%s';", userName, password)
		_, err = conn.Exec(ctx, createRoleQuery)
		if err != nil {
			return "", fmt.Errorf("error creating role %s: %v", userName, err)
		}
		log.Infof("create postgres user: %s", userName)
	}

	return password, nil
}

func (r *StorageReconciler) createPostgresUserSecret(ctx context.Context, userName string, password string, dbs string,
	mgh *v1alpha4.MulticlusterGlobalHub,
) error {
	// convert the userName to a valid secret name
	userSecretName := strings.ReplaceAll(userName, "_", "-")

	userSecret := &corev1.Secret{
		ObjectMeta: metav1.ObjectMeta{
			Name:      fmt.Sprintf(postgresUserNameTemplate, userSecretName),
			Namespace: mgh.Namespace,
		},
	}
	err := r.GetClient().Get(ctx, client.ObjectKeyFromObject(userSecret), userSecret)
	if err != nil && !errors.IsNotFound(err) {
		return err
	}

	// update the databases if exists
	if err == nil {
		log.Infof("the postgresql user secret already exists: %s", userSecret.Name)
		if string(userSecret.Data[PostgresCustomizedUserSecretDatabasesKey]) != dbs ||
			string(userSecret.Data[PostgresCustomizedUserSecretUserKey]) != userName {
			userSecret.Data[PostgresCustomizedUserSecretDatabasesKey] = []byte(dbs)
			userSecret.Data[PostgresCustomizedUserSecretUserKey] = []byte(userName)
			err = r.GetClient().Update(ctx, userSecret)
			if err != nil {
				return fmt.Errorf("failed to updating postgres user secret %s, err %v", userName, err)
			}
			log.Infof("update the postgres user secret: %s", userSecret.Name)
		}
		return nil
	}

	// create secret
	storageConn := config.GetStorageConnection()
	pgConfig, err := pgx.ParseConfig(storageConn.SuperuserDatabaseURI)
	if err != nil {
		return fmt.Errorf("failed the parse the supper user database URI")
	}
	userSecret.Data = map[string][]byte{
		PostgresCustomizedUserSecretHostKey:      []byte(pgConfig.Host),
		PostgresCustomizedUserSecretPortKey:      []byte(fmt.Sprintf("%d", pgConfig.Port)),
		PostgresCustomizedUserSecretUserKey:      []byte(userName),
		PostgresCustomizedUserSecretDatabasesKey: []byte(dbs),
		PostgresCustomizedUserSecretCACertKey:    storageConn.CACert,
	}
	if password != "" {
		userSecret.Data[PostgresCustomizedUserSecretPasswordKey] = []byte(password)
	}
	err = controllerutil.SetControllerReference(mgh, userSecret, r.Manager.GetScheme())
	if err != nil {
		return fmt.Errorf("failed to add the owner reference to the user secret: %s", userSecret.Name)
	}
	err = r.GetClient().Create(ctx, userSecret)
	log.Infof("create the postgresql user secret: %s", userSecret.Name)
	if err != nil {
		return fmt.Errorf("failed to create the postgresql user secret: %s", userSecret.Name)
	}

	return nil
}

func (r *StorageReconciler) grantPermissions(ctx context.Context, conn *pgx.Conn, user, dbName string) error {
	grantQuery := fmt.Sprintf("GRANT ALL PRIVILEGES ON DATABASE \"%s\" TO \"%s\";", dbName, user)
	_, err := conn.Exec(ctx, grantQuery)
	if err != nil {
		return fmt.Errorf("error granting permissions to user %s on database %s: %v", user, dbName, err)
	}
	log.Infof("granted all privileges to user %s on database %s.\n", user, dbName)
	return nil
}

func (r *StorageReconciler) applyGlobalHubInitSQL(ctx context.Context, conn *pgx.Conn, readonlyUserURI string) error {
	// Check if backup is enabled
	var backupEnabled bool
	backupEnabled, err := commonutils.IsBackupEnabled(ctx, r.GetClient())
	if err != nil {
		return fmt.Errorf("failed to get the backup status: %v", err)
	}

	if backupEnabled || !r.upgrade {
		lockSql := fmt.Sprintf("select pg_advisory_lock(%s)", constants.LockId)
		unLockSql := fmt.Sprintf("select pg_advisory_unlock(%s)", constants.LockId)
		defer func() {
			_, err = conn.Exec(ctx, unLockSql)
			if err != nil {
				log.Errorf("failed to unlock db: %v", err)
			}
		}()
		_, err = conn.Exec(ctx, lockSql)
		if err != nil {
			return fmt.Errorf("failed to parse database_uri_with_readonlyuser: %v", err)
		}
	}

	objURI, err := url.Parse(readonlyUserURI)
	if err != nil {
		log.Error(err, "failed to parse database_uri_with_readonlyuser")
	}
	readonlyUsername := objURI.User.Username()

	if err = applySQL(ctx, conn, databaseFS, "database", readonlyUsername); err != nil {
		return fmt.Errorf("failed to apply the database sql: %v", err)
	}

	if r.enableGlobalResource {
		if err = applySQL(ctx, conn, databaseOldFS, "database.old", readonlyUsername); err != nil {
			return fmt.Errorf("failed to apply the database.old sql: %v", err)
		}
	}

	if !r.upgrade {
		err = applySQL(ctx, conn, upgradeFS, "upgrade", readonlyUsername)
		if err != nil {
			return fmt.Errorf("failed to apply the upgrade sql: %v", err)
		}
		r.upgrade = true
	}

	return nil
}

func applySQL(ctx context.Context, conn *pgx.Conn, databaseFS embed.FS, rootDir, username string) error {
	err := iofs.WalkDir(databaseFS, rootDir, func(file string, d iofs.DirEntry, beforeError error) error {
		if beforeError != nil {
			return beforeError
		}
		if d.IsDir() {
			return nil
		}
		sqlBytes, err := databaseFS.ReadFile(file)
		if err != nil {
			return fmt.Errorf("failed to read %s: %w", file, err)
		}
		if file == rootDir+"/5.privileges.sql" {
			if username != "" {
				_, err = conn.Exec(ctx, strings.ReplaceAll(string(sqlBytes), "$1", username))
			}
		} else {
			_, err = conn.Exec(ctx, string(sqlBytes))
		}
		if err != nil {
			return fmt.Errorf("failed to create %s: %w", file, err)
		}
		return nil
	})
	if err != nil {
		return fmt.Errorf("failed to exec database sql: %w", err)
	}
	return nil
}

func generatePassword(length int) string {
	chars := "ABCDEFGHIJKLMNOPQRSTUVWXYZ" +
		"abcdefghijklmnopqrstuvwxyz" +
		"0123456789"

	buf := make([]byte, length)
	for i := 0; i < length; i++ {
		nBig, _ := rand.Int(rand.Reader, big.NewInt(int64(len(chars))))
		buf[i] = chars[nBig.Int64()]
	}
	return string(buf)
}

func getDatabaseComponentStatus(ctx context.Context, c client.Client,
	namespace string, name string, reconcileErr error,
) v1alpha4.StatusCondition {
	availableType := config.COMPONENTS_AVAILABLE
	if reconcileErr != nil {
		return v1alpha4.StatusCondition{
			Kind:    "DatabaseConnection",
			Name:    name,
			Type:    availableType,
			Status:  config.CONDITION_STATUS_FALSE,
			Reason:  config.RECONCILE_ERROR,
			Message: reconcileErr.Error(),
		}
	}
	if config.GetStorageConnection() == nil {
		return v1alpha4.StatusCondition{
			Kind:    "DatabaseConnection",
			Name:    name,
			Type:    availableType,
			Status:  config.CONDITION_STATUS_FALSE,
			Reason:  "DatabaseConnectionNotSet",
			Message: "Database connection is null",
		}
	}

	if config.IsBYOPostgres() {
		return v1alpha4.StatusCondition{
			Kind:    "DatabaseConnection",
			Name:    name,
			Type:    availableType,
			Status:  config.CONDITION_STATUS_TRUE,
			Reason:  "DatabaseConnectionSet",
			Message: "Use customized database, connection has set using provided secret",
		}
	}
	return config.GetStatefulSetComponentStatus(ctx, c, namespace, name)
}

type AnnotationPGUser struct {
	Name      string   `json:"name"`
	Databases []string `json:"databases"`
}
