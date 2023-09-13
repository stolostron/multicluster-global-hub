package cronjob

import (
	"context"
	"path/filepath"
	"testing"
	"time"

	"github.com/jackc/pgx/v4/pgxpool"
	"github.com/stretchr/testify/assert"
	"k8s.io/client-go/kubernetes/scheme"
	"k8s.io/client-go/rest"
	ctrl "sigs.k8s.io/controller-runtime"
	"sigs.k8s.io/controller-runtime/pkg/envtest"

	"github.com/stolostron/multicluster-global-hub/manager/pkg/config"
)

var (
	testenv *envtest.Environment
	cfg     *rest.Config
	ctx     context.Context
	cancel  context.CancelFunc
	mgr     ctrl.Manager
	pool    *pgxpool.Pool
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
		MetricsBindAddress: "0",
		Scheme:             scheme.Scheme,
	})
	assert.Nil(t, err)
	managerConfig := &config.ManagerConfig{
		DatabaseConfig: &config.DatabaseConfig{
			DataRetention: 18,
		},
	}
	managerConfig.SchedulerInterval = "month"
	assert.Nil(t, AddSchedulerToManager(ctx, mgr, pool, managerConfig, false))
	managerConfig.SchedulerInterval = "week"
	assert.Nil(t, AddSchedulerToManager(ctx, mgr, pool, managerConfig, false))
	managerConfig.SchedulerInterval = "day"
	assert.Nil(t, AddSchedulerToManager(ctx, mgr, pool, managerConfig, false))
	managerConfig.SchedulerInterval = "hour"
	assert.Nil(t, AddSchedulerToManager(ctx, mgr, pool, managerConfig, false))
	managerConfig.SchedulerInterval = "minute"
	assert.Nil(t, AddSchedulerToManager(ctx, mgr, pool, managerConfig, false))
	managerConfig.SchedulerInterval = "second"
	assert.Nil(t, AddSchedulerToManager(ctx, mgr, pool, managerConfig, false))

	cancel()
	err = testenv.Stop()
	// https://github.com/kubernetes-sigs/controller-runtime/issues/1571
	// Set 4 with random
	if err != nil {
		time.Sleep(4 * time.Second)
		assert.Nil(t, testenv.Stop())
	}
}
