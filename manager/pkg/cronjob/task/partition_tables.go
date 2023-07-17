package task

import (
	"context"
	"fmt"
	"time"

	"github.com/go-co-op/gocron"
	"github.com/jackc/pgx/v4/pgxpool"
	ctrl "sigs.k8s.io/controller-runtime"
)

var (
	// some data tables will generate records over time, so it is necessary to split them into small tables to
	// achieve better scanning and writing performance.
	partitionTableTaskName = "partition-tables-job"
	// create a partition table for days in the future, it must be created before using.
	partitionInterval = 5
	// delete the partition table for days in the past. Deleting data older than 18 months means 18*30=540 days.
	deletionInterval    = 540
	partitionDateFormat = "2006_01_02"
	partitionTables     = []string{
		"event.local_policies",
		"event.local_root_policies",
		"status.managed_clusters",
		"status.leaf_hubs",
		"local_spec.policies",
	}
)

func PartitionTableJob(ctx context.Context, pool *pgxpool.Pool, enableSimulation bool, job gocron.Job) {
	partitionStartTime := time.Now()
	log = ctrl.Log.WithName(localComplianceTaskName).WithValues("startTime",
		partitionStartTime.Format(partitionDateFormat))

	for i := 0; i < partitionInterval; i++ {
		startDate := partitionStartTime.AddDate(0, 0, i)
		endDate := startDate.AddDate(0, 0, i+1)
		for _, tableName := range partitionTables {
			if err := createPartitionTable(ctx, tableName, startDate, endDate, pool); err != nil {
				log.Error(err, "failed to create partition table", "table", tableName)
			}
		}
	}

	// earliest partition table to retrieve, lets take 5 days before the deletionInterval as an example.
	earliestInterval := deletionInterval + 5
	for i := deletionInterval; i < earliestInterval; i++ {
		for _, tableName := range partitionTables {
			if err := deletePartitionTable(ctx, tableName, partitionStartTime.AddDate(0, 0, -i), pool); err != nil {
				log.Error(err, "failed to delete partition table", "table", tableName)
			}
		}
	}

	endTime := time.Now()
	log.Info("finish running", "endTime", endTime.Format(timeFormat), "nextRun", job.NextRun().Format(timeFormat))
}

func createPartitionTable(ctx context.Context, tableName string, startTime, endTime time.Time,
	pool *pgxpool.Pool,
) error {
	startTimeStr := startTime.Format(partitionDateFormat)
	endTimeStr := endTime.Format(partitionDateFormat)
	partitionTableName := fmt.Sprintf("%s_%s", tableName, startTimeStr)
	createTableSQL := fmt.Sprintf("CREATE TABLE IF NOT EXISTS %s PARTITION OF %s FOR VALUES FROM ('%s') TO ('%s')",
		partitionTableName, tableName, startTimeStr, endTimeStr)
	if _, err := pool.Exec(ctx, createTableSQL); err != nil {
		return fmt.Errorf("failed to create partition table %s: %w", tableName, err)
	}
	return nil
}

func deletePartitionTable(ctx context.Context, tableName string, dateTime time.Time, pool *pgxpool.Pool) error {
	partitionTable := fmt.Sprintf("%s_%s", tableName, dateTime.Format(partitionDateFormat))
	deleteTableSQL := fmt.Sprintf("DROP TABLE IF EXISTS %s", partitionTable)
	if _, err := pool.Exec(ctx, deleteTableSQL); err != nil {
		return fmt.Errorf("failed to delete partition table %s: %w", tableName, err)
	}
	return nil
}
