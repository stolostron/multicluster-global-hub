package hubofhubs

import (
	"context"
	"embed"
	"fmt"
	iofs "io/fs"
	"strings"
	"time"

	"github.com/jackc/pgx/v4"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"

	operatorv1alpha2 "github.com/stolostron/multicluster-global-hub/operator/apis/v1alpha2"
	"github.com/stolostron/multicluster-global-hub/operator/pkg/condition"
	"github.com/stolostron/multicluster-global-hub/operator/pkg/config"
	"github.com/stolostron/multicluster-global-hub/pkg/database"
)

var (
	CheckDatabaseInterval = time.Duration(10) * time.Second
	ExpectedSchemaTables  = map[string]int{
		"spec":         12,
		"status":       9,
		"event":        1,
		"local_spec":   2,
		"local_status": 1,
		"history":      14,
	}
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

	if condition.ContainConditionStatus(mgh, condition.CONDITION_TYPE_DATABASE_INIT, condition.CONDITION_STATUS_TRUE) {
		log.Info("database has been initialized, checking table counts")
		err := r.checkSchemaTable(ctx, conn, mgh)
		if err == nil {
			return nil
		}
		log.Error(err, "failed to check schema table count, re-initializing database")
	}

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

func (r *MulticlusterGlobalHubReconciler) checkSchemaTable(ctx context.Context, pool *pgx.Conn,
	mgh *operatorv1alpha2.MulticlusterGlobalHub,
) error {
	schemaCountTemplate := `
	SELECT
		table_schema,
		count(*) AS table_count
	FROM
		information_schema.tables
	WHERE
		table_schema IN(%s)
	GROUP BY
		table_schema;`
	// Build the dynamic IN condition for the schemas
	schemas := make([]string, 0, len(ExpectedSchemaTables))
	for schema := range ExpectedSchemaTables {
		schemas = append(schemas, schema)
	}
	schemaCountStatement := fmt.Sprintf(schemaCountTemplate,
		fmt.Sprintf("'%s'", strings.Join(schemas, "', '")))
	rows, err := pool.Query(ctx, schemaCountStatement)
	if err != nil {
		return fmt.Errorf("failed to query schema table count: %w", err)
	}
	defer rows.Close()
	queryResult := make(map[string]int)
	for rows.Next() {
		var schema string
		var count int
		if err := rows.Scan(&schema, &count); err != nil {
			return fmt.Errorf("failed to scan schema table count: %w", err)
		}
		queryResult[schema] = count
	}
	if err := rows.Err(); err != nil {
		return fmt.Errorf("failed to iterate over rows: %w", err)
	}
	for schema, count := range ExpectedSchemaTables {
		if queryResult[schema] < count {
			return fmt.Errorf("schema %s table count is %d, expected at least %d", schema, queryResult[schema], count)
		}
	}
	return nil
}
