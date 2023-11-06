package hubofhubs

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

	globalhubv1alpha4 "github.com/stolostron/multicluster-global-hub/operator/apis/v1alpha4"
	"github.com/stolostron/multicluster-global-hub/operator/pkg/condition"
	"github.com/stolostron/multicluster-global-hub/pkg/database"
)

var DatabaseReconcileCounter = 0

//go:embed database
var databaseFS embed.FS

//go:embed database.old
var databaseOldFS embed.FS

//go:embed upgrade
var upgradeFS embed.FS

func (r *MulticlusterGlobalHubReconciler) ReconcileDatabase(ctx context.Context,
	mgh *globalhubv1alpha4.MulticlusterGlobalHub,
) error {
	log := r.Log.WithName("database")

	if condition.ContainConditionStatus(mgh, condition.CONDITION_TYPE_DATABASE_INIT, condition.CONDITION_STATUS_TRUE) {
		log.V(7).Info("database has been initialized, checking the reconcile counter")
		// if the operator is restarted, reconcile the database again
		if DatabaseReconcileCounter > 0 {
			return nil
		}
	}

	conn, err := database.PostgresConnection(ctx, r.MiddlewareConfig.PgConnection.SuperuserDatabaseURI,
		r.MiddlewareConfig.PgConnection.CACert)
	if err != nil {
		return fmt.Errorf("failed to connect to database: %w", err)
	}
	defer func() {
		if err := conn.Close(ctx); err != nil {
			log.Error(err, "failed to close connection to database")
		}
	}()

	objURI, err := url.Parse(r.MiddlewareConfig.PgConnection.ReadonlyUserDatabaseURI)
	if err != nil {
		log.Error(err, "failed to parse database_uri_with_readonlyuser")
	}
	readonlyUsername := objURI.User.Username()

	if err := applySQL(ctx, conn, databaseFS, "database", readonlyUsername); err != nil {
		return err
	}

	if r.EnableGlobalResource {
		if err := applySQL(ctx, conn, databaseOldFS, "database.old", readonlyUsername); err != nil {
			return err
		}
	}
	err = handleUpgrade(ctx, conn, log, upgradeFS, readonlyUsername)
	if err != nil {
		log.Error(err, "Failed to upgrade db schema")
		return err
	}
	log.V(7).Info("database initialized")
	DatabaseReconcileCounter++
	err = condition.SetConditionDatabaseInit(ctx, r.Client, mgh, condition.CONDITION_STATUS_TRUE)
	if err != nil {
		return condition.FailToSetConditionError(condition.CONDITION_STATUS_TRUE, err)
	}
	return nil
}

func handleUpgrade(ctx context.Context, conn *pgx.Conn, log logr.Logger, upgradeFS embed.FS, username string) error {
	log.V(2).Info("Handle upgrade db schema")
	rows, err := conn.Query(ctx,
		"SELECT column_name FROM information_schema.columns WHERE table_name = 'leaf_hubs' AND column_name = 'cluster_id';",
	)
	if err != nil {
		return err
	}
	hasNext := rows.Next()
	log.V(2).Info("Query rows :", "has value:", hasNext)
	defer rows.Close()
	if !hasNext {
		log.Info("Db shcema will be upgraded")
		if err := applySQL(ctx, conn, upgradeFS, "upgrade", username); err != nil {
			return err
		}
		log.Info("Db shcema upgrade success")
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
