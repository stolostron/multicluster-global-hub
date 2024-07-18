// Copyright (c) 2024 Red Hat, Inc.
// Copyright Contributors to the Open Cluster Management project

package hubofhubs

import (
	"context"
	"os"
	"path/filepath"
	"testing"

	. "github.com/onsi/gomega"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/client-go/kubernetes"
	"k8s.io/client-go/rest"
	ctrl "sigs.k8s.io/controller-runtime"
	"sigs.k8s.io/controller-runtime/pkg/client"
	"sigs.k8s.io/controller-runtime/pkg/controller"
	"sigs.k8s.io/controller-runtime/pkg/envtest"
	metricsserver "sigs.k8s.io/controller-runtime/pkg/metrics/server"

	"github.com/stolostron/multicluster-global-hub/operator/apis/v1alpha4"
	"github.com/stolostron/multicluster-global-hub/operator/pkg/config"
	operatorconstants "github.com/stolostron/multicluster-global-hub/operator/pkg/constants"
	"github.com/stolostron/multicluster-global-hub/pkg/utils"
)

var (
	cfg           *rest.Config
	kubeClient    kubernetes.Interface
	runtimeClient client.Client
	namespace     = "default"
	reconciler    *GlobalHubReconciler
	ctx           context.Context
	cancel        context.CancelFunc
)

func TestMain(m *testing.M) {
	ctx, cancel = context.WithCancel(context.Background())
	err := os.Setenv("POD_NAMESPACE", namespace)
	if err != nil {
		panic(err)
	}

	testenv := &envtest.Environment{
		CRDDirectoryPaths: []string{
			filepath.Join("..", "..", "..", "config", "crd", "bases"),
			filepath.Join("..", "..", "..", "..", "test", "manifest", "crd"),
		},
		ErrorIfCRDPathMissing: true,
	}
	config.SetKafkaResourceReady(true)

	cfg, err = testenv.Start()
	if err != nil {
		panic(err)
	}

	kubeClient, err = kubernetes.NewForConfig(cfg)
	if err != nil {
		panic(err)
	}

	runtimeClient, err = client.New(cfg, client.Options{Scheme: config.GetRuntimeScheme()})
	if err != nil {
		panic(err)
	}

	runtimeMgr, err := ctrl.NewManager(cfg, ctrl.Options{
		Metrics: metricsserver.Options{
			BindAddress: "0", // disable the metrics serving
		}, Scheme: config.GetRuntimeScheme(),
		LeaderElection:          true,
		LeaderElectionNamespace: namespace,
		LeaderElectionID:        "549a8919.open-cluster-management.io",
	})
	if err != nil {
		panic(err)
	}

	_, err = controller.New("global-hub-controller", runtimeMgr, controller.Options{
		Reconciler: reconciler,
	})
	if err != nil {
		panic(err)
	}

	// run testings
	code := m.Run()

	cancel()

	// stop testenv
	if err := testenv.Stop(); err != nil {
		panic(err)
	}
	os.Exit(code)
}

func TestController(t *testing.T) {
	RegisterTestingT(t)
	// By("By creating a new MGH instance with reference to nonexisting image override configmap")
	mgh := &v1alpha4.MulticlusterGlobalHub{
		ObjectMeta: metav1.ObjectMeta{
			Name:      "test-mgh",
			Namespace: utils.GetDefaultNamespace(),
			Annotations: map[string]string{
				operatorconstants.AnnotationImageOverridesCM: "noexisting-cm",
			},
		},
		Spec: v1alpha4.MulticlusterGlobalHubSpec{
			DataLayer: v1alpha4.DataLayerConfig{
				Postgres: v1alpha4.PostgresConfig{
					Retention: "2y",
				},
			},
		},
	}
	Expect(runtimeClient.Create(ctx, mgh)).To(Succeed())

	err := runtimeClient.Get(ctx, client.ObjectKeyFromObject(mgh), mgh)
	Expect(err).To(Succeed())

	count := 0
	for _, cond := range mgh.Status.Conditions {
		if cond.Type == config.CONDITION_REASON_RETENTION_PARSED {
			count++
			Expect(cond.Status).To(Equal(metav1.ConditionTrue))
			Expect(cond.Message).To(Equal("The data will be kept in the database for 24 months."))
		}
		if cond.Type == config.CONDITION_TYPE_GLOBALHUB_READY {
			count++
			Expect(cond.Status).To(Equal(metav1.ConditionFalse))
			Expect(cond.Message).To(ContainSubstring("middleware not ready"))
		}
	}
	Expect(count).To(Equal(2))
}
