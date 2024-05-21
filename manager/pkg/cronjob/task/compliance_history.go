package task

import (
	"context"
	"fmt"
	"time"

	"github.com/go-co-op/gocron"
	"k8s.io/apimachinery/pkg/util/wait"
	ctrl "sigs.k8s.io/controller-runtime"

	"github.com/stolostron/multicluster-global-hub/manager/pkg/monitoring"
	"github.com/stolostron/multicluster-global-hub/pkg/database"
	"github.com/stolostron/multicluster-global-hub/pkg/database/models"
)

func SyncToComplianceHistory(ctx context.Context, job gocron.Job) {
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
