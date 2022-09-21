package controller_test

import (
	"context"
	"os"
	"testing"
	"time"

	_ "github.com/lib/pq"
	. "github.com/onsi/ginkgo/v2"
	. "github.com/onsi/gomega"
	"k8s.io/client-go/rest"
	"sigs.k8s.io/controller-runtime/pkg/envtest"
	logf "sigs.k8s.io/controller-runtime/pkg/log"
	"sigs.k8s.io/controller-runtime/pkg/log/zap"

	"github.com/stolostron/multicluster-global-hub/test/pkg/testpostgres"
)

var (
	testenv      *envtest.Environment
	cfg          *rest.Config
	ctx          context.Context
	cancel       context.CancelFunc
	testPostgres *testpostgres.TestPostgres
)

func TestSpec2db(t *testing.T) {
	RegisterFailHandler(Fail)
	RunSpecs(t, "Spec2db Suite")
}

var _ = BeforeSuite(func() {
	logf.SetLogger(zap.New(zap.WriteTo(GinkgoWriter), zap.UseDevMode(true)))
	Expect(os.Setenv("POD_NAMESPACE", "default")).To(Succeed())

	ctx, cancel = context.WithCancel(context.Background())

	var err error
	testenv = &envtest.Environment{}
	cfg, err = testenv.Start()
	Expect(err).NotTo(HaveOccurred())
	Expect(cfg).NotTo(BeNil())

	testPostgres, err = testpostgres.NewTestPostgres()
	Expect(err).NotTo(HaveOccurred())
})

var _ = AfterSuite(func() {
	By("tearing down the test environment")
	err := testenv.Stop()
	// https://github.com/kubernetes-sigs/controller-runtime/issues/1571
	// Set 4 with random
	if err != nil {
		time.Sleep(4 * time.Second)
	}
	err = testenv.Stop()
	Expect(err).NotTo(HaveOccurred())
	err = testPostgres.Stop()
	Expect(err).NotTo(HaveOccurred())
})
