package hubofhubs

import (
	"context"
	"embed"
	"fmt"
	iofs "io/fs"

	"github.com/jackc/pgx/v4"
	corev1 "k8s.io/api/core/v1"
	"k8s.io/apimachinery/pkg/types"
	ctrllog "sigs.k8s.io/controller-runtime/pkg/log"

	operatorv1alpha1 "github.com/stolostron/multicluster-globalhub/operator/apis/operator/v1alpha1"
	"github.com/stolostron/multicluster-globalhub/operator/pkg/condition"
)

const (
	failedConditionMsg = "failed to set condition(%s): %w"
)

//go:embed database
var databaseFS embed.FS

func (reconciler *MultiClusterGlobalHubReconciler) reconcileDatabase(ctx context.Context, mgh *operatorv1alpha1.MultiClusterGlobalHub,
	namespacedName types.NamespacedName,
) error {
	log := ctrllog.FromContext(ctx)
	if condition.ContainConditionStatus(mgh, condition.CONDITION_TYPE_DATABASE_INIT, condition.CONDITION_STATUS_TRUE) {
		log.Info("Database has initialized")
		return nil
	}

	log.Info("Database initializing")
	postgreSecret := &corev1.Secret{}
	err := reconciler.Client.Get(ctx, namespacedName, postgreSecret)
	if err != nil {
		log.Error(err, "Can't get postgres secret")
		return err
	}
	databaseURI := string(postgreSecret.Data["database_uri"])
	conn, err := pgx.Connect(ctx, databaseURI)
	if err != nil {
		return fmt.Errorf("failed to connect to postgres: %w", err)
	}
	defer conn.Close(ctx)

	err = iofs.WalkDir(databaseFS, "database", func(file string, d iofs.DirEntry, beforeError error) error {
		if beforeError != nil {
			return beforeError
		}
		if d.IsDir() {
			return nil
		}
		log.Info("Database executing SQL file: " + file)
		sqlBytes, err := databaseFS.ReadFile(file)
		if err != nil {
			return fmt.Errorf("failed to read %s: %w", file, err)
		}
		_, err = conn.Exec(context.Background(), string(sqlBytes))
		if err != nil {
			return fmt.Errorf("failed to create %s: %w", file, err)
		}
		return nil
	})
	if err != nil {
		conditionError := condition.SetConditionDatabaseInit(ctx, reconciler.Client, mgh, condition.CONDITION_STATUS_FALSE)
		if conditionError != nil {
			return fmt.Errorf(failedConditionMsg, condition.CONDITION_STATUS_FALSE, conditionError)
		}
		return fmt.Errorf("failed to walk database directory: %w", err)
	}

	log.Info("Database initialized")
	err = condition.SetConditionDatabaseInit(ctx, reconciler.Client, mgh, condition.CONDITION_STATUS_TRUE)
	if err != nil {
		return fmt.Errorf(failedConditionMsg, condition.CONDITION_STATUS_TRUE, err)
	}
	return nil
}
