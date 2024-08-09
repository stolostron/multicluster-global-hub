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

	"github.com/go-logr/logr"
	"github.com/jackc/pgx/v4"
	"k8s.io/apimachinery/pkg/api/errors"
	ctrl "sigs.k8s.io/controller-runtime"

	"github.com/stolostron/multicluster-global-hub/operator/apis/v1alpha4"
	"github.com/stolostron/multicluster-global-hub/operator/pkg/config"
	"github.com/stolostron/multicluster-global-hub/pkg/constants"
	"github.com/stolostron/multicluster-global-hub/pkg/database"
	"github.com/stolostron/multicluster-global-hub/pkg/utils"
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

func NewStorageReconciler(mgr ctrl.Manager, enableGlobalResource bool) *StorageReconciler {
	return &StorageReconciler{
		log:                    ctrl.Log.WithName("global-hub-storage"),
		Manager:                mgr,
		upgrade:                false,
		databaseReconcileCount: 0,
		enableGlobalResource:   enableGlobalResource,
	}
}

func (r *StorageReconciler) Reconcile(ctx context.Context, mgh *v1alpha4.MulticlusterGlobalHub) error {
	storageConn, err := r.ReconcileStorage(ctx, mgh)
	if err != nil {
		return fmt.Errorf("storage not ready, Error: %v", err)
	}
	_ = config.SetStorageConnection(storageConn)

	err = r.reconcileDatabase(ctx, mgh)
	if err != nil {
		return fmt.Errorf("database not ready, Error: %v", err)
	}
	config.SetDatabaseReady(true)
	return nil
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

func (r *StorageReconciler) reconcileDatabase(ctx context.Context, mgh *v1alpha4.MulticlusterGlobalHub) error {
	log := r.log.WithName("database")
	storageConn := config.GetStorageConnection()
	if storageConn == nil {
		return fmt.Errorf("storage connection is nil")
	}

	if config.ContainConditionStatus(mgh, config.CONDITION_TYPE_DATABASE_INIT, config.CONDITION_STATUS_TRUE) {
		log.V(7).Info("database has been initialized, checking the reconcile counter")
		// if the operator is restarted, reconcile the database again
		if r.databaseReconcileCount > 0 {
			return nil
		}
	}

	conn, err := database.PostgresConnection(ctx, storageConn.SuperuserDatabaseURI, storageConn.CACert)
	if err != nil {
		return fmt.Errorf("failed to connect to database: %v", err)
	}
	defer func() {
		if err := conn.Close(ctx); err != nil {
			log.Error(err, "failed to close connection to database")
		}
	}()

	// Check if backup is enabled
	backupEnabled, err := utils.IsBackupEnabled(ctx, r.GetClient())
	if err != nil {
		log.Error(err, "failed to get backup status")
		return err
	}

	if backupEnabled || !r.upgrade {
		lockSql := fmt.Sprintf("select pg_advisory_lock(%s)", constants.LockId)
		unLockSql := fmt.Sprintf("select pg_advisory_unlock(%s)", constants.LockId)
		defer func() {
			_, err = conn.Exec(ctx, unLockSql)
			if err != nil {
				log.Error(err, "failed to unlock db")
			}
		}()
		_, err = conn.Exec(ctx, lockSql)
		if err != nil {
			log.Error(err, "failed to parse database_uri_with_readonlyuser")
			return err
		}
	}

	objURI, err := url.Parse(storageConn.ReadonlyUserDatabaseURI)
	if err != nil {
		log.Error(err, "failed to parse database_uri_with_readonlyuser")
	}
	readonlyUsername := objURI.User.Username()

	if err := applySQL(ctx, conn, databaseFS, "database", readonlyUsername); err != nil {
		return err
	}

	if r.enableGlobalResource {
		if err := applySQL(ctx, conn, databaseOldFS, "database.old", readonlyUsername); err != nil {
			return err
		}
	}

	if !r.upgrade {
		err := applySQL(ctx, conn, upgradeFS, "upgrade", readonlyUsername)
		if err != nil {
			log.Error(err, "failed to exec the upgrade sql files")
			return err
		}
		r.upgrade = true
	}

	log.V(7).Info("database initialized")
	r.databaseReconcileCount++
	err = config.SetConditionDatabaseInit(ctx, r.GetClient(), mgh, config.CONDITION_STATUS_TRUE)
	if err != nil {
		return config.FailToSetConditionError(config.CONDITION_STATUS_TRUE, err)
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
