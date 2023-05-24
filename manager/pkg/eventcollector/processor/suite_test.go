package processor

import (
	"context"
	"fmt"
	"testing"

	"github.com/jackc/pgx/v4/pgxpool"
	. "github.com/onsi/ginkgo/v2"
	. "github.com/onsi/gomega"
	logf "sigs.k8s.io/controller-runtime/pkg/log"
	"sigs.k8s.io/controller-runtime/pkg/log/zap"

	"github.com/stolostron/multicluster-global-hub/pkg/database"
	"github.com/stolostron/multicluster-global-hub/test/pkg/testpostgres"
)

var (
	ctx          context.Context
	cancel       context.CancelFunc
	testPostgres *testpostgres.TestPostgres
	pool         *pgxpool.Pool
)

func TestEventProcessor(t *testing.T) {
	RegisterFailHandler(Fail)
	RunSpecs(t, "Processors Suite")
}

var _ = BeforeSuite(func() {
	logf.SetLogger(zap.New(zap.WriteTo(GinkgoWriter), zap.UseDevMode(true)))
	ctx, cancel = context.WithCancel(context.Background())

	var err error
	testPostgres, err = testpostgres.NewTestPostgres()
	Expect(err).NotTo(HaveOccurred())

	pool, err = database.PostgresConnPool(ctx, testPostgres.URI, "test-ca-cert-path")
	Expect(err).NotTo(HaveOccurred())
	Expect(pool).NotTo(BeNil())
})

var _ = AfterSuite(func() {
	cancel()
	pool.Close()
	Expect(testPostgres.Stop()).NotTo(HaveOccurred())
})

type offsetManagerMock struct{}

func (o *offsetManagerMock) MarkOffset(topic string, partition int32, offset int64) {
	fmt.Printf("mark offset: topic=%s, partition=%d, offset=%d\n", topic, partition, offset)
}
