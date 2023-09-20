package cronjob

import (
	"context"
	"strings"
	"time"

	"github.com/go-co-op/gocron"
	"github.com/go-logr/logr"
	"github.com/jackc/pgx/v4/pgxpool"
	ctrl "sigs.k8s.io/controller-runtime"

	"github.com/stolostron/multicluster-global-hub/manager/pkg/config"
	"github.com/stolostron/multicluster-global-hub/manager/pkg/cronjob/task"
)

const (
	EveryMonth  string = "month"
	EveryWeek   string = "week"
	EveryDay    string = "day"
	EveryHour   string = "hour"
	EveryMinute string = "minute"
	EverySecond string = "second"
)

type GlobalHubJobScheduler struct {
	log       logr.Logger
	scheduler *gocron.Scheduler
}

func AddSchedulerToManager(ctx context.Context, mgr ctrl.Manager, pool *pgxpool.Pool,
	managerConfig *config.ManagerConfig, enableSimulation bool,
) error {
	log := ctrl.Log.WithName("cronjob-scheduler")
	// Scheduler timezone:
	// The cluster may be in a different timezones, Here we choose to be consistent with the local GH timezone.
	scheduler := gocron.NewScheduler(time.Local)

	switch managerConfig.SchedulerInterval {
	case EveryMonth:
		scheduler = scheduler.Every(1).Month(1)
	case EveryWeek:
		scheduler = scheduler.Every(1).Week()
	case EveryHour:
		scheduler = scheduler.Every(1).Hour()
	case EveryMinute:
		scheduler = scheduler.Every(1).Minute()
	case EverySecond:
		scheduler = scheduler.Every(1).Second()
	default:
		scheduler = scheduler.Every(1).Day().At("00:00")
	}
	complianceJob, err := scheduler.Tag(task.LocalComplianceTaskName).DoWithJobDetails(
		task.SyncLocalCompliance, ctx, pool, enableSimulation)
	if err != nil {
		return err
	}
	log.Info("set SyncLocalCompliance job", "scheduleAt", complianceJob.ScheduledAtTime())

	dataRetentionJob, err := scheduler.Every(1).Month(1, 15, 28).At("00:00").Tag(task.RetentionTaskName).
		DoWithJobDetails(task.DataRetention, ctx, pool, managerConfig.DatabaseConfig.DataRetention)
	if err != nil {
		return err
	}
	log.Info("set DataRetention job", "scheduleAt", dataRetentionJob.ScheduledAtTime())

	// get the jobs need to execute immediately
	launchJobs := strings.Split(managerConfig.LaunchJobImmediately, ",")
	for _, job := range launchJobs {
		switch job {
		case task.LocalComplianceTaskName, task.RetentionTaskName:
			if err = scheduler.RunByTag(job); err != nil {
				return err
			}
		default:
			log.Info("failed to launch the unknow job %s immediately", job)
		}
	}

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
