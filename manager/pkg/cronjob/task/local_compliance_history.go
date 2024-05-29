package task

import (
	"context"
	"fmt"
	"time"

	"github.com/go-co-op/gocron"
	"github.com/go-logr/logr"
	"k8s.io/apimachinery/pkg/util/wait"
	ctrl "sigs.k8s.io/controller-runtime"

	"github.com/stolostron/multicluster-global-hub/manager/pkg/monitoring"
	"github.com/stolostron/multicluster-global-hub/pkg/database"
	"github.com/stolostron/multicluster-global-hub/pkg/database/models"
)

var (
	LocalComplianceTaskName = "local-compliance-history"
	startTime               time.Time
	log                     logr.Logger
	timeFormat              = "2006-01-02 15:04:05"
	dateFormat              = "2006-01-02"
	batchSize               = int64(1000)
	// batchSize = 1000 for now
	// The suitable batchSize for selecting and inserting a lot of records from a table in PostgreSQL depends on
	// several factors such as the size of the table, available memory, network bandwidth, and hardware specifications.
	// However, as a general rule of thumb, a batch size of around 1000 to 5000 records is a good starting point. This
	// size provides a balance between minimizing the number of queries sent to the server while still being efficient
	// and not overloading the system. To determine the optimal batchSize, it may be helpful to test different batch
	// sizes and measure the performance of the queries.
)

func LocalComplianceHistory(ctx context.Context, job gocron.Job) {
	startTime = time.Now()
	log = ctrl.Log.WithName(LocalComplianceTaskName).WithValues("date", startTime.Format(dateFormat))
	log.V(2).Info("start running", "currentRun", job.LastRun().Format(timeFormat))

	var err error
	defer func() {
		if err != nil {
			monitoring.GlobalHubCronJobGaugeVec.WithLabelValues(LocalComplianceTaskName).Set(1)
		} else {
			monitoring.GlobalHubCronJobGaugeVec.WithLabelValues(LocalComplianceTaskName).Set(0)
		}
	}()

	err = snapshotLocalComplianceToHistory(ctx)
	if err != nil {
		log.Error(err, "sync from local_status.compliance to history.local_compliance failed")
		return
	}

	log.V(2).Info("finish running", "nextRun", job.NextRun().Format(timeFormat))
}

func snapshotLocalComplianceToHistory(ctx context.Context) (err error) {
	db := database.GetGorm()
	var totalCount int64
	err = db.Model(&models.LocalStatusCompliance{}).Count(&totalCount).Error
	if err != nil {
		return err
	}
	log.V(2).Info("The number of compliance need to be synchronized", "count", totalCount)

	insertedCount := int64(0)
	for offset := int64(0); offset < totalCount; offset += batchSize {
		batchInsertedCount, err := batchSync(ctx, totalCount, offset)
		if err != nil {
			return err
		}
		insertedCount += batchInsertedCount
	}
	log.V(2).Info("The number of compliance has been synchronized", "insertedCount", insertedCount)
	return nil
}

func batchSync(ctx context.Context, totalCount, offset int64) (int64, error) {
	batchInsert := int64(0)
	var err error
	defer func() {
		e := traceComplianceHistoryLog(LocalComplianceTaskName, totalCount, offset, offset+batchInsert, startTime, err)
		if e != nil {
			log.Info("trace local compliance job failed, retrying", "error", e)
		}
	}()
	err = wait.PollUntilContextTimeout(ctx, 5*time.Second, 10*time.Minute, true,
		func(ctx context.Context) (done bool, err error) {
			batchSyncSQLTemplate := `
				INSERT INTO history.local_compliance (
					policy_id, 
					cluster_id, 
					leaf_hub_name, 
					compliance, 
					compliance_date
				) 
				(
					SELECT 
						policy_id, 
						cluster_id, 
						leaf_hub_name, 
						compliance, 
						(CURRENT_DATE - INTERVAL '0 day') 
					FROM 
							local_status.compliance
					ORDER BY policy_id, cluster_id
					LIMIT %d 
					OFFSET %d
				)
				ON CONFLICT (
						leaf_hub_name, 
						policy_id, 
						cluster_id, 
						compliance_date
				) DO NOTHING;
			`

			db := database.GetGorm()
			ret := db.Exec(fmt.Sprintf(batchSyncSQLTemplate, batchSize, offset))
			if ret.Error != nil {
				log.Info("exec failed, retrying", "error", ret.Error)
				return false, nil
			}
			batchInsert = ret.RowsAffected
			log.V(2).Info("sync compliance to history", "batch", batchSize, "batchInsert", batchInsert, "offset", offset)
			return true, nil
		})
	return batchInsert, err
}

func traceComplianceHistoryLog(name string, total, offset, inserted int64, start time.Time, err error) error {
	db := database.GetGorm()
	localComplianceJobLog := &models.LocalComplianceJobLog{
		Name:     name,
		StartAt:  start,
		EndAt:    time.Now(),
		Error:    "none",
		Total:    total,
		Offsets:  offset,
		Inserted: inserted,
	}
	if err != nil {
		localComplianceJobLog.Error = err.Error()
	}
	return db.Create(localComplianceJobLog).Error
}
