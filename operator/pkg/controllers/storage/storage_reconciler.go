package storage

import (
	"context"
	"crypto/rand"
	"embed"
	"fmt"
	iofs "io/fs"
	"math/big"
	"net/url"
	"strings"
	"time"

	"github.com/go-logr/logr"
	"github.com/jackc/pgx/v4"
	appsv1 "k8s.io/api/apps/v1"
	corev1 "k8s.io/api/core/v1"
	"k8s.io/apimachinery/pkg/api/errors"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/types"
	"k8s.io/apimachinery/pkg/util/sets"
	"k8s.io/klog"
	ctrl "sigs.k8s.io/controller-runtime"
	"sigs.k8s.io/controller-runtime/pkg/builder"
	"sigs.k8s.io/controller-runtime/pkg/client"
	"sigs.k8s.io/controller-runtime/pkg/event"
	"sigs.k8s.io/controller-runtime/pkg/handler"
	"sigs.k8s.io/controller-runtime/pkg/predicate"

	"github.com/stolostron/multicluster-global-hub/operator/api/operator/v1alpha4"
	"github.com/stolostron/multicluster-global-hub/operator/pkg/config"
	"github.com/stolostron/multicluster-global-hub/pkg/constants"
	"github.com/stolostron/multicluster-global-hub/pkg/database"
	"github.com/stolostron/multicluster-global-hub/pkg/utils"
	commonutils "github.com/stolostron/multicluster-global-hub/pkg/utils"
)

//go:embed database
var databaseFS embed.FS

//go:embed database.old
var databaseOldFS embed.FS

//go:embed upgrade
var upgradeFS embed.FS

//go:embed manifests.sts
var stsPostgresFS embed.FS

type StorageReconciler struct {
	log logr.Logger
	ctrl.Manager
	upgrade                bool
	databaseReconcileCount int
	enableGlobalResource   bool
}

var WatchedSecret = sets.NewString(
	constants.GHStorageSecretName,
	constants.GHBuiltInStorageSecretName,
	config.PostgresCertName,
)

var WatchedConfigMap = sets.NewString(
	constants.PostgresCAConfigMap,
)
var started bool

func StartController(initOption config.ControllerOption) error {
	if started {
		return nil
	}
	err := NewStorageReconciler(initOption.Manager,
		initOption.OperatorConfig.GlobalResourceEnabled).SetupWithManager(initOption.Manager)
	if err != nil {
		return err
	}
	started = true
	klog.Infof("inited storage controller")
	return nil
}

func NewStorageReconciler(mgr ctrl.Manager, enableGlobalResource bool) *StorageReconciler {
	return &StorageReconciler{
		log:                    ctrl.Log.WithName("storage"),
		Manager:                mgr,
		upgrade:                false,
		databaseReconcileCount: 0,
		enableGlobalResource:   enableGlobalResource,
	}
}

func (r *StorageReconciler) SetupWithManager(mgr ctrl.Manager) error {
	return ctrl.NewControllerManagedBy(mgr).Named("storageController").
		For(&v1alpha4.MulticlusterGlobalHub{},
			builder.WithPredicates(config.MGHPred)).
		Watches(&corev1.Secret{},
			&handler.EnqueueRequestForObject{}, builder.WithPredicates(secretPred)).
		Watches(&corev1.ConfigMap{},
			&handler.EnqueueRequestForObject{}, builder.WithPredicates(configmapPred)).
		Watches(&appsv1.StatefulSet{},
			&handler.EnqueueRequestForObject{}, builder.WithPredicates(statefulSetPred)).
		Complete(r)
}

var statefulSetPred = predicate.Funcs{
	CreateFunc: func(e event.CreateEvent) bool {
		return e.Object.GetNamespace() == commonutils.GetDefaultNamespace() &&
			e.Object.GetName() == config.COMPONENTS_POSTGRES_NAME
	},
	UpdateFunc: func(e event.UpdateEvent) bool {
		return e.ObjectNew.GetNamespace() == commonutils.GetDefaultNamespace() &&
			e.ObjectNew.GetName() == config.COMPONENTS_POSTGRES_NAME
	},
	DeleteFunc: func(e event.DeleteEvent) bool {
		return e.Object.GetNamespace() == commonutils.GetDefaultNamespace() &&
			e.Object.GetName() == config.COMPONENTS_POSTGRES_NAME
	},
}

var configmapPred = predicate.Funcs{
	CreateFunc: func(e event.CreateEvent) bool {
		return WatchedConfigMap.Has(e.Object.GetName())
	},
	UpdateFunc: func(e event.UpdateEvent) bool {
		return WatchedConfigMap.Has(e.ObjectNew.GetName())
	},
	DeleteFunc: func(e event.DeleteEvent) bool {
		return WatchedConfigMap.Has(e.Object.GetName())
	},
}

var secretPred = predicate.Funcs{
	CreateFunc: func(e event.CreateEvent) bool {
		return WatchedSecret.Has(e.Object.GetName())
	},
	UpdateFunc: func(e event.UpdateEvent) bool {
		return WatchedSecret.Has(e.ObjectNew.GetName())
	},
	DeleteFunc: func(e event.DeleteEvent) bool {
		return WatchedSecret.Has(e.Object.GetName())
	},
}

// +kubebuilder:rbac:groups="apps",resources=statefulsets,verbs=get;list;watch;create;update;delete
// +kubebuilder:rbac:groups="",resources=persistentvolumeclaims,verbs=get;list;watch;create;update;delete;patch
func (r *StorageReconciler) Reconcile(ctx context.Context, req ctrl.Request) (ctrl.Result, error) {
	mgh, err := config.GetMulticlusterGlobalHub(ctx, r.GetClient())
	if err != nil {
		return ctrl.Result{}, err
	}
	if mgh == nil || config.IsPaused(mgh) || mgh.DeletionTimestamp != nil {
		return ctrl.Result{}, nil
	}
	var reconcileErr error
	defer func() {
		err = config.UpdateMGHComponent(ctx, r.GetClient(),
			getDatabaseComponentStatus(ctx, r.GetClient(), mgh.Namespace, reconcileErr),
		)
		if err != nil {
			klog.Errorf("failed to update mgh status, err:%v", err)
		}
	}()
	storageConn, err := r.ReconcileStorage(ctx, mgh)
	if err != nil {
		reconcileErr = fmt.Errorf("storage not ready, Error: %v", err)
		return ctrl.Result{}, reconcileErr
	}
	_ = config.SetStorageConnection(storageConn)

	needRequeue, err := r.reconcileDatabase(ctx, mgh)
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
		pgConnection, err = EnsureCrunchyPostgres(ctx, r.GetClient(), r.log)
		if err != nil {
			return nil, err
		}
	} else {
		// create the statefulset postgres and initialize the r.MiddlewareConfig.PgConnection
		pgConnection, err = InitPostgresByStatefulset(ctx, mgh, r.Manager, r.log)
		if err != nil {
			return nil, err
		}
	}
	return pgConnection, nil
}

func (r *StorageReconciler) reconcileDatabase(ctx context.Context, mgh *v1alpha4.MulticlusterGlobalHub) (bool, error) {
	log := r.log.WithName("database")

	var reconcileErr error
	storageConn := config.GetStorageConnection()
	if storageConn == nil {
		reconcileErr = fmt.Errorf("storage connection is nil")
		return true, reconcileErr
	}
	// if the operator is restarted, reconcile the database again
	if r.databaseReconcileCount > 0 {
		return false, nil
	}

	conn, err := database.PostgresConnection(ctx, storageConn.SuperuserDatabaseURI, storageConn.CACert)
	if err != nil {
		reconcileErr = fmt.Errorf("failed to connect to database: %v", err)
		klog.Infof("wait database ready")
		return true, nil
	}
	defer func() {
		if err := conn.Close(ctx); err != nil {
			log.Error(err, "failed to close connection to database")
		}
	}()

	// Check if backup is enabled
	var backupEnabled bool
	backupEnabled, reconcileErr = utils.IsBackupEnabled(ctx, r.GetClient())
	if reconcileErr != nil {
		log.Error(reconcileErr, "failed to get backup status")
		return true, reconcileErr
	}

	if backupEnabled || !r.upgrade {
		lockSql := fmt.Sprintf("select pg_advisory_lock(%s)", constants.LockId)
		unLockSql := fmt.Sprintf("select pg_advisory_unlock(%s)", constants.LockId)
		defer func() {
			_, reconcileErr = conn.Exec(ctx, unLockSql)
			if reconcileErr != nil {
				log.Error(reconcileErr, "failed to unlock db")
			}
		}()
		_, reconcileErr = conn.Exec(ctx, lockSql)
		if reconcileErr != nil {
			log.Error(reconcileErr, "failed to parse database_uri_with_readonlyuser")
			return true, reconcileErr
		}
	}

	objURI, err := url.Parse(storageConn.ReadonlyUserDatabaseURI)
	if err != nil {
		log.Error(err, "failed to parse database_uri_with_readonlyuser")
	}
	readonlyUsername := objURI.User.Username()

	if reconcileErr = applySQL(ctx, conn, databaseFS, "database", readonlyUsername); reconcileErr != nil {
		return true, reconcileErr
	}

	if r.enableGlobalResource {
		if reconcileErr = applySQL(ctx, conn, databaseOldFS, "database.old", readonlyUsername); reconcileErr != nil {
			return true, reconcileErr
		}
	}

	if !r.upgrade {
		reconcileErr = applySQL(ctx, conn, upgradeFS, "upgrade", readonlyUsername)
		if reconcileErr != nil {
			log.Error(reconcileErr, "failed to exec the upgrade sql files")
			return true, reconcileErr
		}
		r.upgrade = true
	}

	log.V(7).Info("database initialized")
	r.databaseReconcileCount++

	return false, nil
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
	namespace string, reconcileErr error,
) v1alpha4.StatusCondition {
	name := config.COMPONENTS_POSTGRES_NAME
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
