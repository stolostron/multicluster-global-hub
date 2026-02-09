// Copyright (c) 2024 Red Hat, Inc.
// Copyright Contributors to the Open Cluster Management project

package controller

import (
	"context"
	"path/filepath"
	"testing"
	"time"

	. "github.com/onsi/ginkgo/v2"
	. "github.com/onsi/gomega"
	corev1 "k8s.io/api/core/v1"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/client-go/kubernetes"
	ctrl "sigs.k8s.io/controller-runtime"
	"sigs.k8s.io/controller-runtime/pkg/envtest"
	logf "sigs.k8s.io/controller-runtime/pkg/log"
	"sigs.k8s.io/controller-runtime/pkg/log/zap"
	metricsserver "sigs.k8s.io/controller-runtime/pkg/metrics/server"

	"github.com/stolostron/multicluster-global-hub/agent/pkg/configs"
	"github.com/stolostron/multicluster-global-hub/agent/pkg/controllers"
	"github.com/stolostron/multicluster-global-hub/pkg/constants"
)

func TestIntegration(t *testing.T) {
	RegisterFailHandler(Fail)
	RunSpecs(t, "Controller Integration Suite")
}

var (
	ctx     context.Context
	cancel  context.CancelFunc
	testenv *envtest.Environment
	mgr     ctrl.Manager
)

var _ = BeforeSuite(func() {
	logf.SetLogger(zap.New(zap.WriteTo(GinkgoWriter), zap.UseDevMode(true)))
	ctx, cancel = context.WithCancel(context.Background())

	testenv = &envtest.Environment{
		CRDInstallOptions: envtest.CRDInstallOptions{
			Paths: []string{
				filepath.Join("..", "..", "..", "manifest", "crd"),
			},
			MaxTime: 1 * time.Minute,
		},
	}

	restConfig, err := testenv.Start()
	Expect(err).NotTo(HaveOccurred())

	kubeClient, err := kubernetes.NewForConfig(restConfig)
	Expect(err).NotTo(HaveOccurred())

	mghSystemNamespace := &corev1.Namespace{ObjectMeta: metav1.ObjectMeta{Name: constants.GHAgentNamespace}}
	_, err = kubeClient.CoreV1().Namespaces().Create(ctx, mghSystemNamespace, metav1.CreateOptions{})
	Expect(err).Should(Succeed())

	mgr, err = ctrl.NewManager(restConfig, ctrl.Options{
		Metrics: metricsserver.Options{
			BindAddress: "0", // disable the metrics serving
		},
		Scheme: configs.GetRuntimeScheme(),
	})
	Expect(err).NotTo(HaveOccurred())

	Expect(controllers.AddHubClusterClaimController(mgr)).NotTo(HaveOccurred())
	Expect(controllers.AddVersionClusterClaimController(mgr)).NotTo(HaveOccurred())

	go func() {
		Expect(mgr.Start(ctx)).To(Succeed())
	}()

	Expect(mgr.GetCache().WaitForCacheSync(ctx)).To(BeTrue())
})

var _ = AfterSuite(func() {
	if cancel != nil {
		defer cancel()
	}
	if testenv != nil {
		err := testenv.Stop()
		// https://github.com/kubernetes-sigs/controller-runtime/issues/1571
		// Set 4 with random
		if err != nil {
			time.Sleep(4 * time.Second)
			_ = testenv.Stop()
		}
	}
})
