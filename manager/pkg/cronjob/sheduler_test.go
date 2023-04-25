package cronjob_test

// add test for the scheduler
import (
	"context"
	"os"
	"testing"

	"github.com/jackc/pgx/v4/pgxpool"

	"github.com/stolostron/multicluster-global-hub/manager/pkg/cronjob"
	"github.com/stolostron/multicluster-global-hub/pkg/database"
	"github.com/stolostron/multicluster-global-hub/test/pkg/testpostgres"
)

var (
	testPostgres *testpostgres.TestPostgres
	pool         *pgxpool.Pool
	ctx          context.Context
	cancel       context.CancelFunc
)

func TestMain(m *testing.M) {
	ctx, cancel = context.WithCancel(context.Background())

	var err error
	// init test postgres
	testPostgres, err = testpostgres.NewTestPostgres()
	if err != nil {
		panic(err)
	}

	pool, err = database.PostgresConnPool(ctx, testPostgres.URI, "ca-cert-path")
	if err != nil {
		panic(err)
	}

	// run testings
	code := m.Run()

	cancel()
	if err := testPostgres.Stop(); err != nil {
		panic(err)
	}
	pool.Close()

	os.Exit(code)
}

func TestScheduler(t *testing.T) {
	s, err := cronjob.StartJobScheduler(ctx, pool)
	if err != nil {
		t.Fatal(err)
	}
	defer s.Clear()
}
