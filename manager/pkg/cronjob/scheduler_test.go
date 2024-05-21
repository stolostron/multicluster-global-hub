package cronjob

import (
	"context"
	"path/filepath"
	"testing"
	"time"

	"github.com/go-co-op/gocron"
	"github.com/stretchr/testify/assert"
	"k8s.io/client-go/kubernetes/scheme"
	"k8s.io/client-go/rest"
	ctrl "sigs.k8s.io/controller-runtime"
	"sigs.k8s.io/controller-runtime/pkg/envtest"
	metricsserver "sigs.k8s.io/controller-runtime/pkg/metrics/server"

	"github.com/stolostron/multicluster-global-hub/manager/pkg/config"
	"github.com/stolostron/multicluster-global-hub/manager/pkg/cronjob/task"
)

var (
	testenv *envtest.Environment
	cfg     *rest.Config
	ctx     context.Context
	cancel  context.CancelFunc
	mgr     ctrl.Manager
)

func TestScheduler(t *testing.T) {
	ctx, cancel = context.WithCancel(context.Background())
	var err error
	testenv = &envtest.Environment{
		CRDDirectoryPaths: []string{
			filepath.Join("..", "..", "..", "pkg", "testdata", "crds"),
		},
		ErrorIfCRDPathMissing: true,
	}
	cfg, err = testenv.Start()
	assert.Nil(t, err)
	assert.NotNil(t, cfg)

	mgr, err = ctrl.NewManager(cfg, ctrl.Options{
		Metrics: metricsserver.Options{
			BindAddress: "0", // disable the metrics serving
		}, Scheme: scheme.Scheme,
	})
	assert.Nil(t, err)
	managerConfig := &config.ManagerConfig{
		DatabaseConfig: &config.DatabaseConfig{
			DataRetention: 18,
		},
	}
	managerConfig.SchedulerInterval = "month"
	assert.Nil(t, AddSchedulerToManager(ctx, mgr, managerConfig, false))
	managerConfig.SchedulerInterval = "week"
	assert.Nil(t, AddSchedulerToManager(ctx, mgr, managerConfig, false))
	managerConfig.SchedulerInterval = "day"
	assert.Nil(t, AddSchedulerToManager(ctx, mgr, managerConfig, false))
	managerConfig.SchedulerInterval = "hour"
	assert.Nil(t, AddSchedulerToManager(ctx, mgr, managerConfig, false))
	managerConfig.SchedulerInterval = "minute"
	assert.Nil(t, AddSchedulerToManager(ctx, mgr, managerConfig, false))
	managerConfig.SchedulerInterval = "second"
	assert.Nil(t, AddSchedulerToManager(ctx, mgr, managerConfig, false))

	scheduler := gocron.NewScheduler(time.Local)
	_, err = scheduler.Every(1).Day().At("00:00").Tag(task.LocalComplianceTaskName).DoWithJobDetails(
		task.SyncLocalCompliance, ctx, false)
	assert.Nil(t, err)

	_, err = scheduler.Every(1).Month(1, 15, 28).At("00:00").Tag(task.RetentionTaskName).
		DoWithJobDetails(task.DataRetention, ctx, managerConfig.DatabaseConfig.DataRetention)
	assert.Nil(t, err)

	globalScheduler := &GlobalHubJobScheduler{
		log:        ctrl.Log.WithName("cronjob-scheduler"),
		scheduler:  scheduler,
		launchJobs: []string{task.RetentionTaskName, task.LocalComplianceTaskName, "unexpected_name"},
	}

	err = globalScheduler.execJobs()
	assert.Nil(t, err)

	cancel()
	err = testenv.Stop()
	// https://github.com/kubernetes-sigs/controller-runtime/issues/1571
	// Set 4 with random
	if err != nil {
		time.Sleep(4 * time.Second)
		assert.Nil(t, testenv.Stop())
	}
}
