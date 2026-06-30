// Copyright (c) 2025 Red Hat, Inc.
// Copyright Contributors to the Open Cluster Management project

package hubha

import (
	"context"
	"path/filepath"
	"testing"
	"time"

	. "github.com/onsi/ginkgo/v2"
	. "github.com/onsi/gomega"
	corev1 "k8s.io/api/core/v1"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/client-go/rest"
	ctrl "sigs.k8s.io/controller-runtime"
	"sigs.k8s.io/controller-runtime/pkg/client"
	"sigs.k8s.io/controller-runtime/pkg/envtest"
	logf "sigs.k8s.io/controller-runtime/pkg/log"
	"sigs.k8s.io/controller-runtime/pkg/log/zap"
	metricsserver "sigs.k8s.io/controller-runtime/pkg/metrics/server"

	"github.com/stolostron/multicluster-global-hub/agent/pkg/configs"
	"github.com/stolostron/multicluster-global-hub/agent/pkg/status/generic"
	"github.com/stolostron/multicluster-global-hub/agent/pkg/status/syncers/configmap"
	hubhastatus "github.com/stolostron/multicluster-global-hub/agent/pkg/status/syncers/hubha"
	"github.com/stolostron/multicluster-global-hub/pkg/constants"
	"github.com/stolostron/multicluster-global-hub/pkg/transport"
)

func TestHubHA(t *testing.T) {
	RegisterFailHandler(Fail)
	RunSpecs(t, "Hub HA Integration Suite")
}

var (
	ctx             context.Context
	cancel          context.CancelFunc
	testenv         *envtest.Environment
	cfg             *rest.Config
	k8sClient       client.Client
	mgr             ctrl.Manager
	transportConfig *transport.TransportInternalConfig
	suiteProducer   *mockProducer
)

var _ = BeforeSuite(func() {
	logf.SetLogger(zap.New(zap.WriteTo(GinkgoWriter), zap.UseDevMode(true)))
	ctx, cancel = context.WithCancel(context.Background())

	transportConfig = &transport.TransportInternalConfig{
		TransportType: string(transport.Chan),
		KafkaCredential: &transport.KafkaConfig{
			SpecTopic:   "spec",
			StatusTopic: "status",
		},
	}

	By("Bootstrapping test environment")
	var err error
	testenv = &envtest.Environment{
		CRDInstallOptions: envtest.CRDInstallOptions{
			Paths: []string{
				// Policy CRDs
				filepath.Join("..", "..", "..", "manifest", "crd",
					"0000_00_policy.open-cluster-management.io_policies.crd.yaml"),
				filepath.Join("..", "..", "..", "manifest", "crd",
					"0000_00_policy.open-cluster-management.io_placementbindings.crd.yaml"),
				// Cluster CRDs
				filepath.Join("..", "..", "..", "manifest", "crd",
					"0000_00_cluster.open-cluster-management.io_placements.crd.yaml"),
				filepath.Join("..", "..", "..", "manifest", "crd",
					"0000_02_clusters.open-cluster-management.io_managedclusters.crd.yaml"),
			},
		},
		ErrorIfCRDPathMissing: false, // Some CRDs might not exist in test env
	}

	cfg, err = testenv.Start()
	Expect(err).NotTo(HaveOccurred())
	Expect(cfg).NotTo(BeNil())

	k8sClient, err = client.New(cfg, client.Options{Scheme: configs.GetRuntimeScheme()})
	Expect(err).NotTo(HaveOccurred())
	Expect(k8sClient).NotTo(BeNil())

	// Create test namespaces
	testNamespace := &corev1.Namespace{
		ObjectMeta: metav1.ObjectMeta{
			Name: "default",
		},
	}
	err = k8sClient.Create(ctx, testNamespace)
	if err != nil {
		Expect(client.IgnoreAlreadyExists(err)).NotTo(HaveOccurred())
	}

	agentNamespace := &corev1.Namespace{
		ObjectMeta: metav1.ObjectMeta{
			Name: constants.GHAgentNamespace,
		},
	}
	err = k8sClient.Create(ctx, agentNamespace)
	if err != nil {
		Expect(client.IgnoreAlreadyExists(err)).NotTo(HaveOccurred())
	}

	By("Setting up controller manager")
	mgr, err = ctrl.NewManager(cfg, ctrl.Options{
		Metrics: metricsserver.Options{
			BindAddress: "0", // disable metrics serving
		},
		Scheme: configs.GetRuntimeScheme(),
	})
	Expect(err).NotTo(HaveOccurred())

	By("Adding configmap controller")
	// Initialize agent config for the controller.
	// TransportConfig is included so that the Hub HA emitter created by
	// enableSyncer has a valid config for any PeriodicSyncer ticks that fire.
	agentConfig := &configs.AgentConfig{
		LeafHubName:     "hub1",
		PodNamespace:    constants.GHAgentNamespace,
		TransportConfig: transportConfig,
	}
	configs.SetAgentConfig(agentConfig)

	// Add the configmap controller that watches for hub role changes
	err = configmap.AddConfigMapController(mgr, agentConfig)
	Expect(err).NotTo(HaveOccurred())

	By("Setting up Hub HA lifecycle controller")
	// Create suite-level mock producer for lifecycle tests.
	suiteProducer = newMockProducer()
	// Register the Hub HA lifecycle controller (replaces the removed SetHubHASyncerManager).
	// Use WithNoOpStartResourceSyncerFn so the lifecycle controller does not attempt to
	// register the "hubha" GVK-watching controller — the syncer tests do that directly via
	// StartHubHAResourceSyncer, and controller names must be unique within the manager.
	suitePeriodicSyncer := &generic.PeriodicSyncer{}
	err = hubhastatus.AddHubHAController(mgr, suitePeriodicSyncer, suiteProducer, agentConfig,
		hubhastatus.WithNoOpStartResourceSyncerFn())
	Expect(err).NotTo(HaveOccurred(), "failed to register Hub HA lifecycle controller during integration suite setup")

	go func() {
		defer GinkgoRecover()
		err := mgr.Start(ctx)
		Expect(err).ToNot(HaveOccurred(), "failed to run manager")
	}()

	// Wait for cache sync
	Expect(mgr.GetCache().WaitForCacheSync(ctx)).To(BeTrue())
})

var _ = AfterSuite(func() {
	By("Tearing down the test environment")
	// Stop suite-level producer
	if suiteProducer != nil {
		suiteProducer.Stop()
	}
	if cancel != nil {
		cancel()
	}
	if testenv != nil {
		err := testenv.Stop()
		// https://github.com/kubernetes-sigs/controller-runtime/issues/1571
		if err != nil {
			time.Sleep(4 * time.Second)
			_ = testenv.Stop()
		}
	}
})
