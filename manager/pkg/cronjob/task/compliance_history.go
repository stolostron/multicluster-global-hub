package task

import (
	"context"
	"fmt"

	ctrl "sigs.k8s.io/controller-runtime"

	"github.com/go-co-op/gocron"
	"github.com/jackc/pgx/v4/pgxpool"
)

var batchSize = 100

func SyncLocalCompliance(ctx context.Context, pool *pgxpool.Pool, job gocron.Job) error {
	log := ctrl.Log.WithName("local-compliance-job")
	log.Info("this job is start running", "LastRun", job.LastRun().Format("2006-01-02 15:04:05"),
		"NextRun", job.NextRun().Format("2006-01-02 15:04:05"))

	conn, err := pool.Acquire(ctx)
	if err != nil {
		return err
	}
	defer conn.Release()

	// clean the recent data  HOURS/MINUTES/SECONDS
	result, err := conn.Exec(ctx, `
	DELETE FROM local_status.compliance_history AS "compliance_history" 
	WHERE "compliance_history"."updated_at" BETWEEN NOW() - INTERVAL '1 HOURS' AND NOW()`)
	if err != nil {
		return err
	}
	log.Info("clean rows", "count", result.RowsAffected())

	// batch insert
	var count int64
	err = conn.QueryRow(ctx, fmt.Sprintf("SELECT COUNT(*) FROM %s", "local_status.compliance")).Scan(&count)
	if err != nil {
		return err
	}
	log.Info("total rows", "count", count)

	for offset := 0; offset < int(count); offset += batchSize {
		result, err = conn.Exec(ctx, `
		INSERT INTO local_status.compliance_history (id, cluster_name, leaf_hub_name, compliance) 
		SELECT id,cluster_name,leaf_hub_name,compliance 
		FROM local_status.compliance LIMIT $1 OFFSET $2`, batchSize, offset)
		if err != nil {
			return err
		}
		log.Info("insert rows", "batchSize", batchSize, "offset", offset, "insertCount", result.RowsAffected())
	}
	return nil
}
