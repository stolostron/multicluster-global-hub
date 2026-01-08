package controller

import (
	"time"

	"github.com/go-co-op/gocron"
	. "github.com/onsi/ginkgo/v2"
	. "github.com/onsi/gomega"

	"github.com/stolostron/multicluster-global-hub/manager/pkg/configs"
	"github.com/stolostron/multicluster-global-hub/manager/pkg/processes/cronjob"
	"github.com/stolostron/multicluster-global-hub/manager/pkg/processes/cronjob/task"
)

var _ = Describe("scheduler", func() {
	It("test the scheduler", func() {
		managerConfig := &configs.ManagerConfig{
			DatabaseConfig: &configs.DatabaseConfig{
				DataRetention: 18,
			},
		}

		managerConfig.SchedulerInterval = "second"
		Expect(cronjob.AddSchedulerToManager(ctx, mgr, managerConfig, false)).To(Succeed())

		scheduler := gocron.NewScheduler(time.Local)
		_, err := scheduler.Every(1).Day().At("00:00").Tag(task.LocalComplianceTaskName).DoWithJobDetails(
			task.LocalComplianceHistory, ctx)
		Expect(err).To(Succeed())

		_, err = scheduler.Every(1).Month(1, 15).At("00:00").Tag(task.RetentionTaskName).
			DoWithJobDetails(task.DataRetention, ctx, managerConfig.DatabaseConfig.DataRetention)
		Expect(err).To(Succeed())

		_, err = scheduler.Every(1).MonthLastDay().At("00:00").Tag(task.RetentionTaskName).
			DoWithJobDetails(task.DataRetention, ctx, managerConfig.DatabaseConfig.DataRetention)
		Expect(err).To(Succeed())

		// Test scheduler mechanism without executing data-retention job to avoid
		// conflict with data_retention_job_test which validates the job logs.
		// Only test with local-compliance-history job which doesn't interfere.
		globalScheduler := cronjob.NewGlobalHubScheduler(scheduler,
			[]string{task.LocalComplianceTaskName, "unexpected_name"})
		err = globalScheduler.ExecJobs()
		Expect(err).To(Succeed())
	})
})
