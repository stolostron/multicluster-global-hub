package hubofhubs

import (
	"context"
	"embed"
	"fmt"
	iofs "io/fs"

	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"

	operatorv1alpha2 "github.com/stolostron/multicluster-global-hub/operator/apis/v1alpha2"
	"github.com/stolostron/multicluster-global-hub/operator/pkg/condition"
	"github.com/stolostron/multicluster-global-hub/operator/pkg/config"
	"github.com/stolostron/multicluster-global-hub/pkg/database"
)

//go:embed database
var databaseFS embed.FS

func (r *MulticlusterGlobalHubReconciler) reconcileDatabase(ctx context.Context,
	mgh *operatorv1alpha2.MulticlusterGlobalHub,
) error {
	log := r.Log.WithName("database")

	if config.SkipDBInit(mgh) {
		log.Info("database initialization is skipped")
		return nil
	}

	storageNamespace := config.GetDefaultNamespace()
	storageName := mgh.Spec.DataLayer.LargeScale.Postgres.Name

	log.Info("database initializing with storage secret", "namespace", storageNamespace, "name", storageName)
	postgresSecret, err := r.KubeClient.CoreV1().Secrets(storageNamespace).Get(ctx, storageName,
		metav1.GetOptions{})
	if err != nil {
		log.Error(err, "failed to get storage secret", "namespace", storageNamespace, "name", storageName)
		return err
	}

	conn, err := database.PostgresConnection(ctx, string(postgresSecret.Data["database_uri"]),
		postgresSecret.Data["ca.crt"])
	if err != nil {
		return fmt.Errorf("failed to connect to database: %w", err)
	}
	defer func() {
		if err := conn.Close(ctx); err != nil {
			log.Error(err, "failed to close connection to database")
		}
	}()

	err = iofs.WalkDir(databaseFS, "database", func(file string, d iofs.DirEntry, beforeError error) error {
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
		_, err = conn.Exec(ctx, string(sqlBytes))
		if err != nil {
			return fmt.Errorf("failed to create %s: %w", file, err)
		}
		return nil
	})
	if err != nil {
		return fmt.Errorf("failed to exec database sql: %w", err)
	}

	log.Info("database initialized")
	err = condition.SetConditionDatabaseInit(ctx, r.Client, mgh, condition.CONDITION_STATUS_TRUE)
	if err != nil {
		return condition.FailToSetConditionError(condition.CONDITION_STATUS_TRUE, err)
	}
	return nil
}
