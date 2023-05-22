package enhancers

import (
	"context"
	"path/filepath"
	"testing"
	"time"

	. "github.com/onsi/ginkgo/v2"
	. "github.com/onsi/gomega"
	"k8s.io/client-go/kubernetes/scheme"
	"k8s.io/client-go/rest"
	clusterv1 "open-cluster-management.io/api/cluster/v1"
	policyv1 "open-cluster-management.io/governance-policy-propagator/api/v1"
	"sigs.k8s.io/controller-runtime/pkg/client"
	"sigs.k8s.io/controller-runtime/pkg/envtest"
	logf "sigs.k8s.io/controller-runtime/pkg/log"
	"sigs.k8s.io/controller-runtime/pkg/log/zap"
)

func TestControllers(t *testing.T) {
	RegisterFailHandler(Fail)
	RunSpecs(t, "Enhancers Suite")
}

var (
	testenv       *envtest.Environment
	cfg           *rest.Config
	ctx           context.Context
	cancel        context.CancelFunc
	runtimeClient client.Client
)

var _ = BeforeSuite(func() {
	logf.SetLogger(zap.New(zap.WriteTo(GinkgoWriter), zap.UseDevMode(true)))

	testenv = &envtest.Environment{
		CRDDirectoryPaths: []string{
			filepath.Join("..", "..", "..", "..", "pkg", "testdata", "crds"),
		},
	}
	ctx, cancel = context.WithCancel(context.Background())

	var err error
	cfg, err = testenv.Start()
	Expect(err).NotTo(HaveOccurred())

	// add scheme
	Expect(clusterv1.AddToScheme(scheme.Scheme)).NotTo(HaveOccurred())
	Expect(policyv1.AddToScheme(scheme.Scheme)).NotTo(HaveOccurred())

	// create runtime client
	runtimeClient, err = client.New(cfg, client.Options{Scheme: scheme.Scheme})
	Expect(err).NotTo(HaveOccurred())
})

var _ = AfterSuite(func() {
	defer cancel()
	// https://github.com/kubernetes-sigs/controller-runtime/issues/1571
	if err := testenv.Stop(); err != nil {
		time.Sleep(4 * time.Second)
		Expect(testenv.Stop()).To(Succeed())
	}
})
