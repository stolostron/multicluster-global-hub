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

	assert.Nil(t, AddSchedulerToManager(ctx, mgr, pool, "month", false))
	assert.Nil(t, AddSchedulerToManager(ctx, mgr, pool, "week", false))
	assert.Nil(t, AddSchedulerToManager(ctx, mgr, pool, "day", false))
	assert.Nil(t, AddSchedulerToManager(ctx, mgr, pool, "hour", false))
	assert.Nil(t, AddSchedulerToManager(ctx, mgr, pool, "minute", false))
	assert.Nil(t, AddSchedulerToManager(ctx, mgr, pool, "second", false))

	cancel()
	err = testenv.Stop()
	// https://github.com/kubernetes-sigs/controller-runtime/issues/1571
	// Set 4 with random
	if err != nil {
		time.Sleep(4 * time.Second)
		assert.Nil(t, testenv.Stop())
	}
}
