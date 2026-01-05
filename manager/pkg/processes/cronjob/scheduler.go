package cronjob

import (
	"context"
	"strings"
	"time"

	"github.com/go-co-op/gocron"
	ctrl "sigs.k8s.io/controller-runtime"

	"github.com/stolostron/multicluster-global-hub/manager/pkg/configs"
	"github.com/stolostron/multicluster-global-hub/manager/pkg/processes/cronjob/task"
	"github.com/stolostron/multicluster-global-hub/pkg/logger"
)

const (
	EveryMonth  string = "month"
	EveryWeek   string = "week"
	EveryDay    string = "day"
	EveryHour   string = "hour"
	EveryMinute string = "minute"
	EverySecond string = "second"
)

var log = logger.DefaultZapLogger()

type GlobalHubJobScheduler struct {
	scheduler  *gocron.Scheduler
	launchJobs []string
}

func NewGlobalHubScheduler(scheduler *gocron.Scheduler, launchJobs []string) *GlobalHubJobScheduler {
	return &GlobalHubJobScheduler{
		scheduler:  scheduler,
		launchJobs: launchJobs,
	}
}

func AddSchedulerToManager(ctx context.Context, mgr ctrl.Manager,
	managerConfig *configs.ManagerConfig, enableSimulation bool,
) error {
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
	complianceHistoryJob, err := scheduler.
		Tag(task.LocalComplianceTaskName).
		DoWithJobDetails(task.LocalComplianceHistory, ctx)
	if err != nil {
		return err
	}
	log.Infow("set SyncLocalCompliance job", "scheduleAt", complianceHistoryJob.ScheduledAtTime())

	dataRetentionJob, err := scheduler.
		Every(1).Month(1, 15, 28).At("00:00").
		Tag(task.RetentionTaskName).
		DoWithJobDetails(task.DataRetention, ctx, managerConfig.DatabaseConfig.DataRetention)
	if err != nil {
		return err
	}
	log.Info("set DataRetention job", "scheduleAt", dataRetentionJob.ScheduledAtTime())

	// register the metrics before starting the jobs
	task.RegisterMetrics()

	return mgr.Add(NewGlobalHubScheduler(
		scheduler,
		strings.Split(managerConfig.LaunchJobNames, ",")))
}

func (s *GlobalHubJobScheduler) Start(ctx context.Context) error {
	log.Infow("start job scheduler")
	// Set the status of the job to 0 (success) when the job is started.
	task.GlobalHubCronJobGaugeVec.WithLabelValues(task.RetentionTaskName).Set(0)
	task.GlobalHubCronJobGaugeVec.WithLabelValues(task.LocalComplianceTaskName).Set(0)
	s.scheduler.StartAsync()

	// Always run data-retention job on startup to ensure partition tables exist
	// This is critical to handle operator restarts that occur near month boundaries
	log.Info("running data-retention job on startup to ensure partition tables exist")
	if err := s.scheduler.RunByTag(task.RetentionTaskName); err != nil {
		log.Errorw("failed to run data-retention job on startup", "error", err)
		// Don't return error here, allow scheduler to continue
	}

	// Run other configured launch jobs
	if err := s.ExecJobs(); err != nil {
		return err
	}
	<-ctx.Done()
	s.scheduler.Stop()
	return nil
}

func (s *GlobalHubJobScheduler) ExecJobs() error {
	for _, job := range s.launchJobs {
		switch job {
		case task.RetentionTaskName:
			log.Infow("launch the job", "name", job)
			if err := s.scheduler.RunByTag(job); err != nil {
				return err
			}
		default:
			log.Infow("failed to launch the unknow job immediately", "name", job)
		}
	}
	return nil
}
