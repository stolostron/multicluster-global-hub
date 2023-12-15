// Copyright (c) 2022 Red Hat, Inc.
// Copyright Contributors to the Open Cluster Management project

package controllers_test

import (
	"context"
	"path/filepath"
	"testing"
	"time"

	. "github.com/onsi/ginkgo/v2"
	. "github.com/onsi/gomega"
	mchv1 "github.com/stolostron/multiclusterhub-operator/api/v1"
	apiextensionsv1 "k8s.io/apiextensions-apiserver/pkg/apis/apiextensions/v1"
	"k8s.io/client-go/kubernetes/scheme"
	"k8s.io/client-go/rest"
	clustersv1alpha1 "open-cluster-management.io/api/cluster/v1alpha1"
	operatorv1 "open-cluster-management.io/api/operator/v1"
	ctrl "sigs.k8s.io/controller-runtime"
	"sigs.k8s.io/controller-runtime/pkg/envtest"
	logf "sigs.k8s.io/controller-runtime/pkg/log"
	"sigs.k8s.io/controller-runtime/pkg/log/zap"

	"github.com/stolostron/multicluster-global-hub/agent/pkg/controllers"
)

func TestControllers(t *testing.T) {
	RegisterFailHandler(Fail)
	RunSpecs(t, "Controller Integration Suite")
}

var (
	testenv *envtest.Environment
	cfg     *rest.Config
	ctx     context.Context
	cancel  context.CancelFunc
	mgr     ctrl.Manager
)

var _ = BeforeSuite(func() {
	logf.SetLogger(zap.New(zap.WriteTo(GinkgoWriter), zap.UseDevMode(true)))

	testenv = &envtest.Environment{
		CRDDirectoryPaths: []string{
			filepath.Join("..", "..", "..", "pkg", "testdata", "crds"),
		},
	}
	ctx, cancel = context.WithCancel(context.Background())

	var err error
	cfg, err = testenv.Start()
	Expect(err).NotTo(HaveOccurred())

	By("Creating the Manager")
	// add scheme
	Expect(mchv1.AddToScheme(scheme.Scheme)).NotTo(HaveOccurred())
	Expect(clustersv1alpha1.AddToScheme(scheme.Scheme)).NotTo(HaveOccurred())
	Expect(apiextensionsv1.AddToScheme(scheme.Scheme)).NotTo(HaveOccurred())
	Expect(operatorv1.AddToScheme(scheme.Scheme)).NotTo(HaveOccurred())

	By("Creating the Manager")
	mgr, err = ctrl.NewManager(cfg, ctrl.Options{
		MetricsBindAddress: "0", // disable the metrics serving
		Scheme:             scheme.Scheme,
	})
	Expect(err).NotTo(HaveOccurred())

	By("Adding the controllers to the manager")
	Expect(controllers.StartHubClusterClaimController(mgr)).NotTo(HaveOccurred())
	Expect(controllers.StartVersionClusterClaimController(mgr)).NotTo(HaveOccurred())

	go func() {
		Expect(mgr.Start(ctx)).NotTo(HaveOccurred())
	}()

	By("Waiting for the manager to be ready")
	Expect(mgr.GetCache().WaitForCacheSync(ctx)).To(BeTrue())
})

var _ = AfterSuite(func() {
	defer cancel()
	// stop testenv
	if err := testenv.Stop(); err != nil {
		time.Sleep(4 * time.Second)
		Expect(testenv.Stop()).To(Succeed())
	}
	// https://github.com/kubernetes-sigs/controller-runtime/issues/1571
})
