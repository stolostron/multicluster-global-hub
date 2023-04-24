package cronjob

import (
	"context"
	"time"

	ctrl "sigs.k8s.io/controller-runtime"

	"github.com/go-co-op/gocron"
	"github.com/jackc/pgx/v4/pgxpool"
	"github.com/stolostron/multicluster-global-hub/manager/pkg/cronjob/task"
)

func StartJobScheduler(ctx context.Context, pool *pgxpool.Pool) (*gocron.Scheduler, error) {
	log := ctrl.Log.WithName("cronjob-scheduler")
	s := gocron.NewScheduler(time.UTC)

	complianceJob, err := s.Every(1).Day().At("01:00").Tag("LocalCompliance").DoWithJobDetails(
		task.SyncLocalCompliance, ctx, pool)
	if err != nil {
		return nil, err
	}
	log.Info("set compliance job", "tags", complianceJob.Tags, "scheduleAt",
		complianceJob.ScheduledTime().Format("2006-01-02 15:04:05"))

	s.StartAsync()
	return s, nil
}
