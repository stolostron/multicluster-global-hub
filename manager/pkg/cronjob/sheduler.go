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
	// Scheduler timezone:
	// The cluster may be in a different timezones, Here we choose to be consistent with the local GH timezone.
	scheduler := gocron.NewScheduler(time.Local)

	complianceJob, err := scheduler.Every(1).Day().At("00:00").Tag("LocalComplianceHistory").DoWithJobDetails(
		task.SyncLocalCompliance, ctx, pool)
	if err != nil {
		return err
	}
	log.Info("set local compliance job", "scheduleAt", complianceJob.ScheduledAtTime())

	return mgr.Add(&GlobalHubJobScheduler{
		log:       log,
		scheduler: scheduler,
	})
}

func (s *GlobalHubJobScheduler) Start(ctx context.Context) error {
	s.log.Info("start job scheduler")
	s.scheduler.StartAsync()
	<-ctx.Done()
	s.scheduler.Stop()
	return nil
}
