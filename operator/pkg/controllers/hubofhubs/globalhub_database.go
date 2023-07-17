package hubofhubs

import (
	"context"
	"embed"
	"fmt"
	iofs "io/fs"

	corev1 "k8s.io/api/core/v1"
	"sigs.k8s.io/controller-runtime/pkg/client"

	operatorv1alpha3 "github.com/stolostron/multicluster-global-hub/operator/apis/v1alpha3"
	"github.com/stolostron/multicluster-global-hub/operator/pkg/condition"
	"github.com/stolostron/multicluster-global-hub/operator/pkg/config"
	operatorconstants "github.com/stolostron/multicluster-global-hub/operator/pkg/constants"
	"github.com/stolostron/multicluster-global-hub/pkg/database"
)

var DatabaseReconcileCounter = 0

//go:embed database
var databaseFS embed.FS

func (r *MulticlusterGlobalHubReconciler) reconcileDatabase(ctx context.Context,
	mgh *operatorv1alpha3.MulticlusterGlobalHub,
) error {
	log := r.Log.WithName("database")

	if config.SkipDBInit(mgh) {
		log.Info("database initialization is skipped")
		return nil
	}

	if condition.ContainConditionStatus(mgh, condition.CONDITION_TYPE_DATABASE_INIT, condition.CONDITION_STATUS_TRUE) {
		log.Info("database has been initialized, checking the reconcile counter")
		// if the operator is restarted, reconcile the database again
		if DatabaseReconcileCounter > 0 {
			return nil
		}
	}

	// since cache the namespace secret, so we can retrieve it from controller runtime client
	log.Info("database initializing with storage secret", "namespace", mgh.Namespace, "name",
		operatorconstants.GHStorageSecretName)
	postgresSecret := &corev1.Secret{}
	if err := r.Client.Get(ctx, client.ObjectKey{
		Namespace: mgh.Namespace,
		Name:      operatorconstants.GHStorageSecretName,
	}, postgresSecret); err != nil {
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
	DatabaseReconcileCounter++
	err = condition.SetConditionDatabaseInit(ctx, r.Client, mgh, condition.CONDITION_STATUS_TRUE)
	if err != nil {
		return condition.FailToSetConditionError(condition.CONDITION_STATUS_TRUE, err)
	}
	return nil
}
