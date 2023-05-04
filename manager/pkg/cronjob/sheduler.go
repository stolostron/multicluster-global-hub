package cronjob

import (
	"context"
	"time"

	ctrl "sigs.k8s.io/controller-runtime"

	"github.com/go-co-op/gocron"
	"github.com/go-logr/logr"
	"github.com/jackc/pgx/v4/pgxpool"
	"github.com/stolostron/multicluster-global-hub/manager/pkg/cronjob/task"
)

type GlobalHubJobScheduler struct {
	log       logr.Logger
	scheduler *gocron.Scheduler
}

func AddSchedulerToManager(ctx context.Context, mgr ctrl.Manager, pool *pgxpool.Pool) error {
	log := ctrl.Log.WithName("cronjob-scheduler")
	scheduler := gocron.NewScheduler(time.Local) // or time.UTC

	complianceJob, err := scheduler.Every(1).Day().At("00:00").Tag("LocalComplianceHistory").DoWithJobDetails(
		task.SyncLocalCompliance, ctx, pool)
	if err != nil {
		return err
	}
	log.Info("set local compliance job", "scheduleAt", complianceJob.ScheduledAtTime())

	mgr.Add(&GlobalHubJobScheduler{
		log:       log,
		scheduler: scheduler,
	})
	return nil
}

func (s *GlobalHubJobScheduler) Start(ctx context.Context) error {
	s.log.Info("start job scheduler")
	s.scheduler.StartAsync()
	<-ctx.Done()
	s.scheduler.Stop()
	return nil
}
